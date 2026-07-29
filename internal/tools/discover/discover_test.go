package discover

import (
	"context"
	"encoding/binary"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

type recorder struct {
	mu       sync.Mutex
	devices  []Device
	progress []string
}

func (r *recorder) sink() Sink {
	return Sink{
		Emit: func(d Device) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.devices = append(r.devices, d)
		},
		Progress: func(_ int, message string) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.progress = append(r.progress, message)
		},
	}
}

// waitFor gives the read loop time to record what arrived. Devices are collected
// on the probing goroutine after the sender has already finished, so reading the
// slice straight away is a race, not a pass.
func (r *recorder) waitFor(t *testing.T, n int) []Device {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		got := append([]Device(nil), r.devices...)
		r.mu.Unlock()
		if len(got) >= n {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	r.mu.Lock()
	got := append([]Device(nil), r.devices...)
	r.mu.Unlock()
	t.Fatalf("wanted %d devices within 5 seconds, got %d: %+v", n, len(got), got)
	return nil
}

func TestNormalizeParams(t *testing.T) {
	tests := []struct {
		in      int
		wantMS  int
		wantErr bool
	}{
		{0, 4000, false},
		{999, 0, true},
		{1000, 1000, false},
		{30000, 30000, false},
		{30001, 0, true},
		{-1, 0, true},
	}
	for _, tt := range tests {
		got, err := Params{TimeoutMS: tt.in}.normalize()
		if tt.wantErr {
			if err == nil {
				t.Errorf("timeout %d was accepted", tt.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("timeout %d errored: %v", tt.in, err)
			continue
		}
		if got != time.Duration(tt.wantMS)*time.Millisecond {
			t.Errorf("timeout %d became %v, want %d ms", tt.in, got, tt.wantMS)
		}
	}
}

func TestMDNSQuery(t *testing.T) {
	got, err := mdnsQuery()
	if err != nil {
		t.Fatalf("mdnsQuery errored: %v", err)
	}
	if len(got) < headerBytes {
		t.Fatalf("query is %d bytes", len(got))
	}
	// A fixed zero transaction id: mDNS does not match replies by id, and a
	// constant packet is one a test can pin.
	if id := binary.BigEndian.Uint16(got[0:]); id != 0 {
		t.Errorf("transaction id = %d, want 0", id)
	}
	if qd := int(binary.BigEndian.Uint16(got[4:])); qd != len(ServiceTypes) {
		t.Errorf("question count = %d, want %d", qd, len(ServiceTypes))
	}
	for _, count := range []int{6, 8, 10} {
		if n := binary.BigEndian.Uint16(got[count:]); n != 0 {
			t.Errorf("a query has %d records at header offset %d, want 0", n, count)
		}
	}

	// Every question carries the QU bit, asking responders to answer the port
	// that asked. A live run showed real responders ignore it and answer the
	// group instead, which is why the tool also joins the group; the bit is kept
	// because it is harmless and some responders do honour it.
	msg, err := decodeMessage(got)
	if err != nil {
		t.Fatalf("the query does not parse as a DNS message: %v", err)
	}
	if len(msg.records) != 0 {
		t.Errorf("a query carries %d records, want none", len(msg.records))
	}

	off := headerBytes
	for i := range ServiceTypes {
		name, next, err := decodeName(got, off)
		if err != nil {
			t.Fatalf("question %d name: %v", i, err)
		}
		if want := strings.TrimSuffix(ServiceTypes[i], "."); name != want {
			t.Errorf("question %d is %q, want %q", i, name, want)
		}
		if rtype := binary.BigEndian.Uint16(got[next:]); rtype != typePTR {
			t.Errorf("question %d has type %d, want PTR (%d)", i, rtype, typePTR)
		}
		// The literal 0x8001 is written in rather than compared against
		// classINWithUnicast: reading the constant it is checking would let the
		// QU bit be dropped with this test still green, which is exactly what a
		// mutation proved.
		if class := binary.BigEndian.Uint16(got[next+2:]); class != 0x8001 {
			t.Errorf("question %d has class %#x, want 0x8001 (class IN, top bit set for a unicast response)",
				i, class)
		}
		off = next + 4
	}
	if off != len(got) {
		t.Errorf("query has %d trailing bytes", len(got)-off)
	}
}

func TestSSDPSearchBytes(t *testing.T) {
	// Asserted as a literal, including the quotes around ssdp:discover and the
	// blank line at the end. A responder ignores a request missing either.
	want := "M-SEARCH * HTTP/1.1\r\n" +
		"HOST: 239.255.255.250:1900\r\n" +
		"MAN: \"ssdp:discover\"\r\n" +
		"MX: 2\r\n" +
		"ST: ssdp:all\r\n" +
		"\r\n"
	if ssdpSearch != want {
		t.Errorf("M-SEARCH\n got %q\nwant %q", ssdpSearch, want)
	}
	if !strings.HasSuffix(ssdpSearch, "\r\n\r\n") {
		t.Error("the request does not end with a blank line")
	}
}

func TestDevicesFromMDNS(t *testing.T) {
	printer := mustHex(t, mdnsPrinterHex)

	t.Run("a full response", func(t *testing.T) {
		got := devicesFromMDNS(printer, "192.168.1.200", "eth0")
		if len(got) != 1 {
			t.Fatalf("got %d devices, want 1: %+v", len(got), got)
		}
		d := got[0]
		if d.Name != "Brother HL-L2350DW" {
			t.Errorf("name = %q", d.Name)
		}
		if d.Service != "_ipp._tcp" {
			t.Errorf("service = %q, want _ipp._tcp", d.Service)
		}
		if d.Port != 631 {
			t.Errorf("port = %d, want 631", d.Port)
		}
		if d.Host != "BRW001122334455.local" {
			t.Errorf("host = %q", d.Host)
		}
		// The A record wins over the packet's source address.
		if d.IP != "192.168.1.63" {
			t.Errorf("ip = %q, want 192.168.1.63 from the A record", d.IP)
		}
		if !strings.Contains(d.Details, "Brother HL-L2350DW") {
			t.Errorf("details = %q", d.Details)
		}
		if d.Adapter != "eth0" {
			t.Errorf("adapter = %q", d.Adapter)
		}
		if d.Key == "" {
			t.Error("key is blank, so the UI cannot upsert this row")
		}
	})

	t.Run("no A record falls back to the packet source", func(t *testing.T) {
		pointer := mustHex(t, mdnsPointerHex)
		got := devicesFromMDNS(pointer, "10.0.0.7", "wlan0")
		if len(got) != 1 {
			t.Fatalf("got %d devices, want 1: %+v", len(got), got)
		}
		if got[0].IP != "10.0.0.7" {
			t.Errorf("ip = %q, want the source address", got[0].IP)
		}
		if got[0].Name != "Living Room" {
			t.Errorf("name = %q", got[0].Name)
		}
		if got[0].Service != "_airplay._tcp" {
			t.Errorf("service = %q", got[0].Service)
		}
	})

	t.Run("nothing usable produces no device", func(t *testing.T) {
		if got := devicesFromMDNS([]byte{1, 2, 3}, "10.0.0.1", "eth0"); len(got) != 0 {
			t.Errorf("got %+v, want none", got)
		}
		if got := devicesFromMDNS(nil, "10.0.0.1", "eth0"); len(got) != 0 {
			t.Errorf("got %+v, want none", got)
		}
	})

	t.Run("the meta-query's service list is not a device", func(t *testing.T) {
		// A responder answering _services._dns-sd._udp.local names service
		// types, not devices. Treating them as devices would fill the table with
		// rows that are not things.
		msg := buildPTRResponse(t, "_services._dns-sd._udp.local", "_ipp._tcp.local")
		if got := devicesFromMDNS(msg, "10.0.0.1", "eth0"); len(got) != 0 {
			t.Errorf("got %+v, want none", got)
		}
	})

	t.Run("a meta-query answer shaped like an instance is still not a device", func(t *testing.T) {
		// The case above is refused by the ordinary "is this an instance of that
		// service" test, so on its own it does not exercise the meta-query guard
		// at all. This target really does look like an instance of the
		// meta-query name, which is the only input that reaches the guard.
		msg := buildPTRResponse(t,
			"_services._dns-sd._udp.local", "evil._services._dns-sd._udp.local")
		if got := devicesFromMDNS(msg, "10.0.0.1", "eth0"); len(got) != 0 {
			t.Errorf("got %+v, want none: the service enumeration list holds service types, not devices", got)
		}
	})
}

// buildPTRResponse writes a one-record response. Test-only, and deliberately not
// used to build the fixtures the parser is checked against: those come from
// python so the encoder and the decoder cannot share a mistake.
func buildPTRResponse(t *testing.T, owner, target string) []byte {
	t.Helper()
	ownerName, err := encodeName(owner)
	if err != nil {
		t.Fatalf("encodeName: %v", err)
	}
	targetName, err := encodeName(target)
	if err != nil {
		t.Fatalf("encodeName: %v", err)
	}
	out := make([]byte, headerBytes)
	binary.BigEndian.PutUint16(out[2:], 0x8400)
	binary.BigEndian.PutUint16(out[6:], 1)
	out = append(out, ownerName...)
	out = binary.BigEndian.AppendUint16(out, typePTR)
	out = binary.BigEndian.AppendUint16(out, 1)
	out = binary.BigEndian.AppendUint32(out, 120)
	out = binary.BigEndian.AppendUint16(out, uint16(len(targetName)))
	return append(out, targetName...)
}

func TestSplitInstance(t *testing.T) {
	tests := []struct {
		in          string
		wantName    string
		wantService string
	}{
		{"Brother HL-L2350DW._ipp._tcp.local", "Brother HL-L2350DW", "_ipp._tcp"},
		{"Living Room._airplay._tcp.local", "Living Room", "_airplay._tcp"},
		{"kitchen-nas._smb._tcp.local", "kitchen-nas", "_smb._tcp"},
		{"noservice.local", "noservice", ""},
		{"", "", ""},
	}
	for _, tt := range tests {
		name, service := splitInstance(tt.in)
		if name != tt.wantName || service != tt.wantService {
			t.Errorf("splitInstance(%q) = (%q, %q), want (%q, %q)",
				tt.in, name, service, tt.wantName, tt.wantService)
		}
	}
}

func TestDetailLine(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want string
	}{
		{"a model name", []string{"ty=Brother HL-L2350DW"}, "Brother HL-L2350DW"},
		{
			"several useful keys, in the order the list gives",
			[]string{"product=(Brother series)", "ty=Brother HL-L2350DW"},
			"Brother HL-L2350DW, (Brother series)",
		},
		{"nothing useful", []string{"txtvers=1", "priority=50", "adminurl=http://x"}, ""},
		{"an empty value is skipped", []string{"ty="}, ""},
		{"a value with no equals sign", []string{"standalone"}, ""},
		{"nothing at all", nil, ""},
		{"a key in a different case", []string{"TY=Canon"}, "Canon"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detailLine(tt.in); got != tt.want {
				t.Errorf("detailLine(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDetailLineIsNotADump(t *testing.T) {
	// A device can publish twenty keys of internal state. Only the listed ones
	// reach the screen, so the Details column stays readable.
	var many []string
	for i := 0; i < 20; i++ {
		many = append(many, "internal"+string(rune('a'+i))+"=value")
	}
	many = append(many, "ty=Real Name")
	got := detailLine(many)
	if got != "Real Name" {
		t.Errorf("detailLine = %q, want only the useful key", got)
	}
}

func TestDevicesFromSSDP(t *testing.T) {
	response := "HTTP/1.1 200 OK\r\n" +
		"CACHE-CONTROL: max-age=1800\r\n" +
		"LOCATION: http://192.168.1.5:49152/desc.xml\r\n" +
		"SERVER: Linux/4.4 UPnP/1.0 MiniDLNA/1.2\r\n" +
		"ST: upnp:rootdevice\r\n" +
		"USN: uuid:4d696e69-444c-164e-9d41-001122334455::upnp:rootdevice\r\n" +
		"\r\n"

	t.Run("a search response", func(t *testing.T) {
		got := devicesFromSSDP([]byte(response), "192.168.1.5", "eth0")
		if len(got) != 1 {
			t.Fatalf("got %d devices, want 1", len(got))
		}
		d := got[0]
		if d.Protocol != ProtocolSSDP {
			t.Errorf("protocol = %q", d.Protocol)
		}
		if d.Host != "192.168.1.5" || d.Port != 49152 {
			t.Errorf("host:port = %s:%d, want 192.168.1.5:49152", d.Host, d.Port)
		}
		if d.Service != "upnp:rootdevice" {
			t.Errorf("service = %q", d.Service)
		}
		if d.Details != "Linux/4.4 UPnP/1.0 MiniDLNA/1.2" {
			t.Errorf("details = %q", d.Details)
		}
		// A USN that is only a uuid carries no name, and inventing one would be
		// worse than an empty cell the UI labels.
		if d.Name != "" {
			t.Errorf("name = %q, want empty for a uuid-only USN", d.Name)
		}
	})

	t.Run("headers are matched whatever their case", func(t *testing.T) {
		lower := strings.ReplaceAll(response, "LOCATION:", "Location:")
		lower = strings.ReplaceAll(lower, "SERVER:", "server:")
		got := devicesFromSSDP([]byte(lower), "192.168.1.5", "eth0")
		if len(got) != 1 || got[0].Port != 49152 || got[0].Details == "" {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("a NOTIFY announcement", func(t *testing.T) {
		notify := "NOTIFY * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\n" +
			"LOCATION: http://10.0.0.9/root.xml\r\nNT: upnp:rootdevice\r\n\r\n"
		got := devicesFromSSDP([]byte(notify), "10.0.0.9", "eth0")
		if len(got) != 1 {
			t.Fatalf("got %d devices, want 1", len(got))
		}
		if got[0].Service != "upnp:rootdevice" {
			t.Errorf("service = %q, want the NT header", got[0].Service)
		}
		if got[0].Port != 80 {
			t.Errorf("port = %d, want 80 for a LOCATION with no port", got[0].Port)
		}
	})

	t.Run("no LOCATION still gives a device at the source address", func(t *testing.T) {
		bare := "HTTP/1.1 200 OK\r\nST: ssdp:all\r\n\r\n"
		got := devicesFromSSDP([]byte(bare), "10.0.0.3", "eth0")
		if len(got) != 1 {
			t.Fatalf("got %d devices, want 1", len(got))
		}
		if got[0].IP != "10.0.0.3" || got[0].Host != "" || got[0].Port != 0 {
			t.Errorf("got %+v", got[0])
		}
	})

	t.Run("garbage produces nothing", func(t *testing.T) {
		for _, bad := range []string{"", "hello", "GET / HTTP/1.1\r\n\r\n", "HTTP/1.1 404 Not Found\r\n\r\n"} {
			if got := devicesFromSSDP([]byte(bad), "10.0.0.1", "eth0"); len(got) != 0 {
				t.Errorf("%q produced %+v", bad, got)
			}
		}
	})
}

func TestHostAndPort(t *testing.T) {
	tests := []struct {
		in       string
		wantHost string
		wantPort int
	}{
		{"http://192.168.1.5:49152/desc.xml", "192.168.1.5", 49152},
		{"http://192.168.1.5/desc.xml", "192.168.1.5", 80},
		{"https://192.168.1.5/desc.xml", "192.168.1.5", 443},
		{"http://nas.local:8200/rootDesc.xml", "nas.local", 8200},
		{"not a url", "", 0},
		{"", "", 0},
	}
	for _, tt := range tests {
		host, port := hostAndPort(tt.in)
		if host != tt.wantHost || port != tt.wantPort {
			t.Errorf("hostAndPort(%q) = (%q, %d), want (%q, %d)",
				tt.in, host, port, tt.wantHost, tt.wantPort)
		}
	}
}

func TestNameFromUSN(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"uuid:4d696e69-444c::upnp:rootdevice", ""},
		{"UUID:4d696e69::upnp:rootdevice", ""},
		{"kitchen-nas::upnp:rootdevice", "kitchen-nas"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := nameFromUSN(tt.in); got != tt.want {
			t.Errorf("nameFromUSN(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDeviceKey(t *testing.T) {
	base := Device{Protocol: ProtocolMDNS, IP: "10.0.0.1", Service: "_ipp._tcp", Name: "Printer"}
	same := newDevice(base)
	again := newDevice(base)
	if same.Key != again.Key {
		t.Error("two emissions of the same device have different keys, so the UI will show it twice")
	}

	other := base
	other.Service = "_http._tcp"
	if newDevice(other).Key == same.Key {
		t.Error("a different service shares a key, so two real rows would collapse into one")
	}

	elsewhere := base
	elsewhere.IP = "10.0.0.2"
	if newDevice(elsewhere).Key == same.Key {
		t.Error("a different address shares a key")
	}

	// The adapter is deliberately not part of the key: the same device heard on
	// two adapters is one device, not two rows.
	onWifi := base
	onWifi.Adapter = "wlan0"
	if newDevice(onWifi).Key != same.Key {
		t.Error("the adapter is part of the key, so a multi-homed machine lists everything twice")
	}
}

func TestUsableInterfaces(t *testing.T) {
	addr := func(ip string) net.Addr {
		return &net.IPNet{IP: net.ParseIP(ip), Mask: net.CIDRMask(24, 32)}
	}
	addrs := map[string][]net.Addr{
		"eth0": {addr("192.168.1.42")},
		"wlan0": {
			&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
			addr("10.0.0.5"),
		},
		"lo":     {addr("127.0.0.1")},
		"down0":  {addr("10.0.0.9")},
		"nomult": {addr("10.0.0.10")},
		"v6only": {&net.IPNet{IP: net.ParseIP("fe80::2"), Mask: net.CIDRMask(64, 128)}},
	}
	lookup := func(ifi net.Interface) ([]net.Addr, error) { return addrs[ifi.Name], nil }

	up := net.FlagUp | net.FlagMulticast
	got := UsableInterfaces([]net.Interface{
		{Name: "eth0", Flags: up},
		{Name: "wlan0", Flags: up},
		{Name: "lo", Flags: up | net.FlagLoopback},
		{Name: "down0", Flags: net.FlagMulticast},
		{Name: "nomult", Flags: net.FlagUp},
		{Name: "v6only", Flags: up},
	}, lookup)

	want := []Interface{{Name: "eth0", IP: "192.168.1.42"}, {Name: "wlan0", IP: "10.0.0.5"}}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("interface %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestUsableInterfacesOnAnEmptyMachine(t *testing.T) {
	lookup := func(net.Interface) ([]net.Addr, error) { return nil, nil }
	got := UsableInterfaces([]net.Interface{
		{Name: "lo", Flags: net.FlagUp | net.FlagLoopback | net.FlagMulticast},
		{Name: "eth0", Flags: net.FlagMulticast},
	}, lookup)
	if len(got) != 0 {
		t.Errorf("got %+v, want none", got)
	}
}

// TestProbeReadsAndEmits drives the real read loop over loopback. The queries go
// to the multicast groups and are simply lost there, which is the same thing
// that happens on an interface with no multicast route: the loop must keep
// listening either way.
func TestProbeReadsAndEmits(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer conn.Close()

	rec := &recorder{}
	c := &collector{out: rec.sink(), adapters: 1}
	deadline := time.Now().Add(3 * time.Second)

	done := make(chan struct{})
	go func() {
		listen(context.Background(), conn, Interface{Name: "lo0", IP: "127.0.0.1"}, deadline, c)
		close(done)
	}()

	sender, err := net.Dial("udp4", conn.LocalAddr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer sender.Close()

	if _, err := sender.Write(mustHex(t, mdnsPrinterHex)); err != nil {
		t.Fatalf("send mdns: %v", err)
	}
	ssdp := "HTTP/1.1 200 OK\r\nLOCATION: http://127.0.0.1:49152/d.xml\r\n" +
		"ST: upnp:rootdevice\r\nSERVER: TestServer/1.0\r\n\r\n"
	if _, err := sender.Write([]byte(ssdp)); err != nil {
		t.Fatalf("send ssdp: %v", err)
	}

	got := rec.waitFor(t, 2)
	<-done

	var sawMDNS, sawSSDP bool
	for _, d := range got {
		if d.Protocol == ProtocolMDNS {
			sawMDNS = true
			if d.Adapter != "lo0" {
				t.Errorf("adapter = %q, want lo0", d.Adapter)
			}
		}
		if d.Protocol == ProtocolSSDP {
			sawSSDP = true
			if d.IP != "127.0.0.1" {
				t.Errorf("ssdp ip = %q, want the source address", d.IP)
			}
		}
	}
	if !sawMDNS {
		t.Error("the mDNS response was not emitted")
	}
	if !sawSSDP {
		t.Error("the SSDP response was not emitted")
	}

	summary := c.summary()
	if summary["mdns"] != 1 || summary["ssdp"] != 1 {
		t.Errorf("summary = %v, want one of each", summary)
	}
	if len(rec.progress) == 0 {
		t.Error("no progress was reported")
	}
}

// TestProbeSurvivesAMalformedPacket: one broken device must never end the run.
func TestProbeSurvivesAMalformedPacket(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer conn.Close()

	rec := &recorder{}
	c := &collector{out: rec.sink(), adapters: 1}
	done := make(chan struct{})
	go func() {
		listen(context.Background(), conn, Interface{Name: "lo0", IP: "127.0.0.1"}, time.Now().Add(3*time.Second), c)
		close(done)
	}()

	sender, err := net.Dial("udp4", conn.LocalAddr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer sender.Close()

	// Garbage first, then a real answer. The real one must still arrive.
	_, _ = sender.Write([]byte{0xFF, 0xFE, 0xFD, 0xFC})
	_, _ = sender.Write(mustHex(t, mdnsLoopHex))
	_, _ = sender.Write(mustHex(t, mdnsPrinterHex))

	got := rec.waitFor(t, 1)
	<-done
	if got[0].Name != "Brother HL-L2350DW" {
		t.Errorf("got %+v, want the real device after the garbage", got[0])
	}
}

func TestProbeStopsOnCancel(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	rec := &recorder{}
	c := &collector{out: rec.sink(), adapters: 1}

	started := time.Now()
	listen(ctx, conn, Interface{Name: "lo0", IP: "127.0.0.1"}, time.Now().Add(30*time.Second), c)
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Errorf("a cancelled listen took %v, want it to stop promptly", elapsed)
	}
	if len(rec.devices) != 0 {
		t.Errorf("emitted %d devices after a cancel", len(rec.devices))
	}
}

// TestCollectorStopsAtMaxDevices pins the literal 2000.
func TestCollectorStopsAtMaxDevices(t *testing.T) {
	if MaxDevices != 2000 {
		t.Fatalf("MaxDevices = %d, want 2000", MaxDevices)
	}
	rec := &recorder{}
	c := &collector{out: rec.sink(), adapters: 1}

	for i := 0; i < MaxDevices+50; i++ {
		c.add([]Device{newDevice(Device{
			Protocol: ProtocolMDNS,
			IP:       "10.0.0.1",
			Service:  "_http._tcp",
			Name:     "device-" + string(rune('a'+i%26)) + "-" + time.Duration(i).String(),
		})})
	}
	summary := c.summary()
	if summary["devices"] != MaxDevices {
		t.Errorf("devices = %v, want %d", summary["devices"], MaxDevices)
	}
	if summary["truncated"] != true {
		t.Error("truncated = false after passing the cap")
	}
	if !strings.Contains(summary["note"].(string), "Stopped after 2000 devices.") {
		t.Errorf("note = %q", summary["note"])
	}
}

func TestSummaryNote(t *testing.T) {
	tests := []struct {
		name      string
		devices   int
		adapters  int
		failures  int
		truncated bool
		wantIn    string
	}{
		{"devices found", 12, 2, 0, false, "Only devices that advertise themselves appear here."},
		{"nothing answered", 0, 2, 0, false, "Nothing answered."},
		{"every adapter refused", 0, 2, 2, false, "None of this computer's adapters would send the questions"},
		{"one adapter refused", 5, 2, 1, false, "1 of 2 adapters would not send the questions."},
		{"truncated", 2000, 1, 0, true, "Stopped after 2000 devices."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := note(tt.devices, tt.adapters, tt.failures, tt.truncated)
			if !strings.Contains(got, tt.wantIn) {
				t.Errorf("note = %q, want it to contain %q", got, tt.wantIn)
			}
			// Every note carries the silence sentence in some form, whatever
			// else happened. It is the one thing about this tool a tech must
			// not forget.
			if !strings.Contains(strings.ToLower(got), "silence") &&
				!strings.Contains(strings.ToLower(got), "does not mean the network is empty") {
				t.Errorf("note %q does not say that silence is not proof of absence", got)
			}
		})
	}
}

func TestCollectorReemitsButCountsOnce(t *testing.T) {
	rec := &recorder{}
	c := &collector{out: rec.sink(), adapters: 1}
	d := newDevice(Device{Protocol: ProtocolMDNS, IP: "10.0.0.1", Service: "_ipp._tcp", Name: "P"})

	c.add([]Device{d})
	c.add([]Device{d})

	// Emitted twice, because a later reply may carry more than the first did,
	// and the UI upserts by key.
	if len(rec.devices) != 2 {
		t.Errorf("emitted %d times, want 2", len(rec.devices))
	}
	if got := c.summary()["devices"]; got != 1 {
		t.Errorf("counted %v devices, want 1", got)
	}
}

func TestLooksLikeSSDP(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"HTTP/1.1 200 OK\r\n", true},
		{"NOTIFY * HTTP/1.1\r\n", true},
		{"M-SEARCH * HTTP/1.1\r\n", true},
		{"\x00\x00\x84\x00\x00", false},
		{"abc", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := looksLikeSSDP([]byte(tt.in)); got != tt.want {
			t.Errorf("looksLikeSSDP(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestServiceTypesAreWellFormed(t *testing.T) {
	if len(ServiceTypes) != 23 {
		t.Errorf("ServiceTypes has %d entries, want 23", len(ServiceTypes))
	}
	if ServiceTypes[0] != "_services._dns-sd._udp.local." {
		t.Errorf("the first entry is %q, want the meta-query", ServiceTypes[0])
	}
	seen := map[string]bool{}
	for _, s := range ServiceTypes {
		if seen[s] {
			t.Errorf("%q appears twice", s)
		}
		seen[s] = true
		if !strings.HasSuffix(s, ".local.") {
			t.Errorf("%q does not end with .local.", s)
		}
		if !strings.HasPrefix(s, "_") {
			t.Errorf("%q does not start with an underscore", s)
		}
		if _, err := encodeName(s); err != nil {
			t.Errorf("%q cannot be encoded: %v", s, err)
		}
	}
}

// TestJoinGroupIsNotFatal pins the behaviour a live run needed: a group that
// cannot be joined returns nil rather than stopping the probe, which is what
// happens on a container bridge or a VPN tunnel.
func TestJoinGroupIsNotFatal(t *testing.T) {
	if got := joinGroup("no-such-interface-xyz", MDNSAddress); got != nil {
		got.Close()
		t.Error("joinGroup on an unknown interface returned a socket")
	}
	if got := joinGroup("lo", "not an address"); got != nil {
		got.Close()
		t.Error("joinGroup with a bad address returned a socket")
	}
}
