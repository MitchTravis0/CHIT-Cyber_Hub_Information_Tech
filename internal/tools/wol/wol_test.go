package wol

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"chit/internal/netinfo"
	"chit/internal/ouidb"
)

func mustParseMAC(t *testing.T, text string) [6]byte {
	t.Helper()
	b, ok := ouidb.ParseMAC(text)
	if !ok {
		t.Fatalf("ParseMAC(%q) failed, the test fixture is wrong", text)
	}
	return b
}

func TestMagicPacketBytes(t *testing.T) {
	mac := mustParseMAC(t, "AA:BB:CC:DD:EE:FF")

	got, err := MagicPacket(mac, nil)
	if err != nil {
		t.Fatalf("MagicPacket: %v", err)
	}
	if len(got) != 102 {
		t.Fatalf("packet is %d bytes, want 102", len(got))
	}

	want := make([]byte, 0, 102)
	for i := 0; i < 6; i++ {
		want = append(want, 0xFF)
	}
	for i := 0; i < 16; i++ {
		want = append(want, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("byte %d is %#02x, want %#02x", i, got[i], want[i])
		}
	}
}

func TestMagicPacketPassword(t *testing.T) {
	mac := mustParseMAC(t, "AA:BB:CC:DD:EE:FF")

	tests := []struct {
		name     string
		password []byte
		wantLen  int
		wantErr  string
	}{
		{"none", nil, 102, ""},
		{"four bytes", []byte{0x11, 0x22, 0x33, 0x44}, 106, ""},
		{"six bytes", []byte{1, 2, 3, 4, 5, 6}, 108, ""},
		{
			"five bytes",
			[]byte{1, 2, 3, 4, 5},
			0,
			"The SecureOn password must be 4 or 6 pairs of hex digits, for example 11-22-33-44. 5 were given.",
		},
		{
			"seven bytes",
			[]byte{1, 2, 3, 4, 5, 6, 7},
			0,
			"The SecureOn password must be 4 or 6 pairs of hex digits, for example 11-22-33-44. 7 were given.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MagicPacket(mac, tt.password)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("got no error, want %q", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("error is %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("MagicPacket: %v", err)
			}
			if len(got) != tt.wantLen {
				t.Fatalf("packet is %d bytes, want %d", len(got), tt.wantLen)
			}
			tail := got[len(got)-len(tt.password):]
			for i := range tt.password {
				if tail[i] != tt.password[i] {
					t.Fatalf("password byte %d is %#02x, want %#02x", i, tail[i], tt.password[i])
				}
			}
		})
	}
}

func TestMagicPacketRejectsGroupAddresses(t *testing.T) {
	tests := []struct {
		name string
		mac  string
		want string
	}{
		{
			"broadcast",
			"FF:FF:FF:FF:FF:FF",
			"FF:FF:FF:FF:FF:FF is not a single device's address, so it cannot be woken. Use the MAC address printed on the machine or shown by the scanner.",
		},
		{
			"multicast",
			"01:00:5E:00:00:01",
			"01:00:5E:00:00:01 is not a single device's address, so it cannot be woken. Use the MAC address printed on the machine or shown by the scanner.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := MagicPacket(mustParseMAC(t, tt.mac), nil)
			if err == nil {
				t.Fatal("got no error, want the group address rejection")
			}
			if err.Error() != tt.want {
				t.Fatalf("error is %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

// wired builds an eligible adapter with one IPv4 address.
func wired(name, ip, broadcast string, prefix int) netinfo.Adapter {
	return netinfo.Adapter{
		Name: name,
		Up:   true,
		IPv4: []netinfo.IPv4{{IP: ip, Prefix: prefix, Broadcast: broadcast}},
	}
}

func assertTargets(t *testing.T, got, want []Target) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d targets, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("target %d is %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestSelectTargetsSingleAdapter(t *testing.T) {
	got, err := SelectTargets([]netinfo.Adapter{wired("eth0", "192.168.1.50", "192.168.1.255", 24)}, "", 9)
	if err != nil {
		t.Fatalf("SelectTargets: %v", err)
	}
	assertTargets(t, got, []Target{
		{Adapter: "eth0", From: "192.168.1.50", To: "192.168.1.255", Port: 9},
		{Adapter: "eth0", From: "192.168.1.50", To: "255.255.255.255", Port: 9},
		{Adapter: "eth0", From: "192.168.1.50", To: "192.168.1.255", Port: 7},
		{Adapter: "eth0", From: "192.168.1.50", To: "255.255.255.255", Port: 7},
	})
}

func TestSelectTargetsSkipsIneligible(t *testing.T) {
	down := wired("eth0", "192.168.1.50", "192.168.1.255", 24)
	down.Up = false

	loopback := wired("lo", "127.0.0.1", "127.255.255.255", 8)
	loopback.Loopback = true

	virtual := wired("docker0", "172.17.0.1", "172.17.255.255", 16)
	virtual.Virtual = true

	noIPv4 := netinfo.Adapter{Name: "eth1", Up: true}
	unspecified := wired("eth2", "0.0.0.0", "255.255.255.255", 8)
	notAnIP := wired("eth3", "not an address", "192.168.1.255", 24)

	tests := []struct {
		name    string
		adapter netinfo.Adapter
	}{
		{"down", down},
		{"loopback", loopback},
		{"virtual", virtual},
		{"apipa", wired("eth4", "169.254.10.20", "169.254.255.255", 16)},
		{"slash 31", wired("eth5", "10.0.0.1", "10.0.0.1", 31)},
		{"slash 32", wired("eth6", "10.0.0.1", "", 32)},
		{"prefix zero", wired("eth7", "10.0.0.1", "10.255.255.255", 0)},
		{"empty broadcast", wired("eth8", "10.0.0.1", "", 24)},
		{"no ipv4", noIPv4},
		{"unspecified", unspecified},
		{"unparseable address", notAnIP},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SelectTargets([]netinfo.Adapter{tt.adapter}, "", 9)
			if err == nil {
				t.Fatalf("got %d targets, want none and the no-network error", len(got))
			}
			if err.Error() != noNetworkMessage {
				t.Fatalf("error is %q, want %q", err.Error(), noNetworkMessage)
			}
		})
	}
}

const noNetworkMessage = "This machine is not on a network it can send a wake-up packet from. Connect it to the same network as the machine you want to wake."

func TestSelectTargetsMultiHomed(t *testing.T) {
	adapters := []netinfo.Adapter{
		wired("eth0", "192.168.1.50", "192.168.1.255", 24),
		wired("wlan0", "10.0.5.7", "10.0.5.255", 24),
	}

	got, err := SelectTargets(adapters, "", 9)
	if err != nil {
		t.Fatalf("SelectTargets: %v", err)
	}
	assertTargets(t, got, []Target{
		{Adapter: "eth0", From: "192.168.1.50", To: "192.168.1.255", Port: 9},
		{Adapter: "eth0", From: "192.168.1.50", To: "255.255.255.255", Port: 9},
		{Adapter: "wlan0", From: "10.0.5.7", To: "10.0.5.255", Port: 9},
		{Adapter: "wlan0", From: "10.0.5.7", To: "255.255.255.255", Port: 9},
		{Adapter: "eth0", From: "192.168.1.50", To: "192.168.1.255", Port: 7},
		{Adapter: "eth0", From: "192.168.1.50", To: "255.255.255.255", Port: 7},
		{Adapter: "wlan0", From: "10.0.5.7", To: "10.0.5.255", Port: 7},
		{Adapter: "wlan0", From: "10.0.5.7", To: "255.255.255.255", Port: 7},
	})
}

func TestSelectTargetsCustomPort(t *testing.T) {
	adapters := []netinfo.Adapter{
		wired("eth0", "192.168.1.50", "192.168.1.255", 24),
		wired("wlan0", "10.0.5.7", "10.0.5.255", 24),
	}

	got, err := SelectTargets(adapters, "", 4343)
	if err != nil {
		t.Fatalf("SelectTargets: %v", err)
	}
	assertTargets(t, got, []Target{
		{Adapter: "eth0", From: "192.168.1.50", To: "192.168.1.255", Port: 4343},
		{Adapter: "eth0", From: "192.168.1.50", To: "255.255.255.255", Port: 4343},
		{Adapter: "wlan0", From: "10.0.5.7", To: "10.0.5.255", Port: 4343},
		{Adapter: "wlan0", From: "10.0.5.7", To: "255.255.255.255", Port: 4343},
	})
}

func TestSelectTargetsOverride(t *testing.T) {
	adapters := []netinfo.Adapter{
		wired("eth0", "192.168.1.50", "192.168.1.255", 24),
		wired("wlan0", "10.0.5.7", "10.0.5.255", 24),
	}

	got, err := SelectTargets(adapters, "192.168.9.255", 4343)
	if err != nil {
		t.Fatalf("SelectTargets: %v", err)
	}
	assertTargets(t, got, []Target{
		{Adapter: "eth0", From: "192.168.1.50", To: "192.168.9.255", Port: 4343},
		{Adapter: "wlan0", From: "10.0.5.7", To: "192.168.9.255", Port: 4343},
	})

	// On the default port the destination is still one per adapter, tried on
	// both the usual port and the old one.
	got, err = SelectTargets(adapters, "192.168.9.255", 9)
	if err != nil {
		t.Fatalf("SelectTargets: %v", err)
	}
	assertTargets(t, got, []Target{
		{Adapter: "eth0", From: "192.168.1.50", To: "192.168.9.255", Port: 9},
		{Adapter: "wlan0", From: "10.0.5.7", To: "192.168.9.255", Port: 9},
		{Adapter: "eth0", From: "192.168.1.50", To: "192.168.9.255", Port: 7},
		{Adapter: "wlan0", From: "10.0.5.7", To: "192.168.9.255", Port: 7},
	})

	const want = "\"192.168.1.999\" is not an address to send to. Use something like 192.168.1.255, or leave it empty to let CHIT work it out."
	if _, err := SelectTargets(adapters, "192.168.1.999", 9); err == nil {
		t.Fatal("got no error, want the bad override rejection")
	} else if err.Error() != want {
		t.Fatalf("error is %q, want %q", err.Error(), want)
	}
}

func TestSelectTargetsNoAdapters(t *testing.T) {
	_, err := SelectTargets(nil, "", 9)
	if err == nil {
		t.Fatal("got no error, want the no-network message")
	}
	if err.Error() != noNetworkMessage {
		t.Fatalf("error is %q, want %q", err.Error(), noNetworkMessage)
	}
}

func TestSelectTargetsDeduplicates(t *testing.T) {
	// A repeated address is what the OS reports on an adapter with an alias.
	repeated := netinfo.Adapter{
		Name: "eth0",
		Up:   true,
		IPv4: []netinfo.IPv4{
			{IP: "192.168.1.50", Prefix: 24, Broadcast: "192.168.1.255"},
			{IP: "192.168.1.50", Prefix: 24, Broadcast: "192.168.1.255"},
		},
	}

	got, err := SelectTargets([]netinfo.Adapter{repeated}, "", 9)
	if err != nil {
		t.Fatalf("SelectTargets: %v", err)
	}
	assertTargets(t, got, []Target{
		{Adapter: "eth0", From: "192.168.1.50", To: "192.168.1.255", Port: 9},
		{Adapter: "eth0", From: "192.168.1.50", To: "255.255.255.255", Port: 9},
		{Adapter: "eth0", From: "192.168.1.50", To: "192.168.1.255", Port: 7},
		{Adapter: "eth0", From: "192.168.1.50", To: "255.255.255.255", Port: 7},
	})

	// Two addresses in the same subnet share a broadcast but differ by source,
	// so every triple is still unique.
	shared := netinfo.Adapter{
		Name: "eth0",
		Up:   true,
		IPv4: []netinfo.IPv4{
			{IP: "192.168.1.50", Prefix: 24, Broadcast: "192.168.1.255"},
			{IP: "192.168.1.51", Prefix: 24, Broadcast: "192.168.1.255"},
		},
	}
	got, err = SelectTargets([]netinfo.Adapter{shared}, "", 9)
	if err != nil {
		t.Fatalf("SelectTargets: %v", err)
	}
	if len(got) != 8 {
		t.Fatalf("got %d targets, want 8: %+v", len(got), got)
	}
	seen := map[Target]bool{}
	for _, target := range got {
		if seen[target] {
			t.Fatalf("target %+v appears twice", target)
		}
		seen[target] = true
	}
}

func TestWakeSendsOverLoopback(t *testing.T) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("could not listen on loopback: %v", err)
	}
	defer conn.Close()

	packet, err := MagicPacket(mustParseMAC(t, "AA:BB:CC:DD:EE:FF"), nil)
	if err != nil {
		t.Fatalf("MagicPacket: %v", err)
	}

	target := Target{
		Adapter: "lo",
		From:    "127.0.0.1",
		To:      "127.0.0.1",
		Port:    conn.LocalAddr().(*net.UDPAddr).Port,
	}
	got := sendTo(target, packet)
	if got.Error != "" {
		t.Fatalf("sendTo reported %q", got.Error)
	}
	if got.Bytes != len(packet) {
		t.Fatalf("sent %d bytes, want %d", got.Bytes, len(packet))
	}

	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	buf := make([]byte, 256)
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("nothing arrived on the loopback socket: %v", err)
	}
	if n != len(packet) {
		t.Fatalf("received %d bytes, want %d", n, len(packet))
	}
	for i := range packet {
		if buf[i] != packet[i] {
			t.Fatalf("received byte %d is %#02x, want %#02x", i, buf[i], packet[i])
		}
	}
}

func TestCheckAwakeDetectsListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not listen on loopback: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	got, err := checkPortsAt(context.Background(), "127.0.0.1", []int{port})
	if err != nil {
		t.Fatalf("checkPortsAt: %v", err)
	}
	if !got.Alive {
		t.Fatal("Alive is false, want true against a socket that is listening")
	}
	want := "port " + strconv.Itoa(port)
	if got.Via != want {
		t.Fatalf("Via is %q, want %q", got.Via, want)
	}
	if got.LatencyMS < 0 {
		t.Fatalf("LatencyMS is %d, want zero or more", got.LatencyMS)
	}
	if got.IP != "127.0.0.1" {
		t.Fatalf("IP is %q, want 127.0.0.1", got.IP)
	}
}

func TestCheckAwakeDeadAddress(t *testing.T) {
	// TEST-NET-1 is reserved for documentation and is never routed, so nothing
	// can answer and no real network is involved.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	got, err := checkPortsAt(ctx, "192.0.2.1", []int{445, 3389})
	if err != nil {
		t.Fatalf("checkPortsAt: %v", err)
	}
	if got.Alive {
		t.Fatal("Alive is true, want false for an address that cannot answer")
	}
	if got.Via != "" {
		t.Fatalf("Via is %q, want empty", got.Via)
	}
}

func TestCheckAwakeBadIP(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want string
	}{
		{"empty", "", "\"\" is not an IP address, so there is nothing to check."},
		{"hostname", "reception-pc", "\"reception-pc\" is not an IP address, so there is nothing to check."},
		{"out of range", "192.168.1.999", "\"192.168.1.999\" is not an IP address, so there is nothing to check."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := checkPortsAt(context.Background(), tt.ip, checkPorts)
			if err == nil {
				t.Fatal("got no error, want the bad address rejection")
			}
			if err.Error() != tt.want {
				t.Fatalf("error is %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestDecodePassword(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    []byte
		wantErr string
	}{
		{"empty", "", nil, ""},
		{"hyphenated", "11-22-33-44", []byte{0x11, 0x22, 0x33, 0x44}, ""},
		{"colons", "11:22:33:44:55:66", []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66}, ""},
		{"bare", "11223344", []byte{0x11, 0x22, 0x33, 0x44}, ""},
		{
			"not hex",
			"zz-22-33-44",
			nil,
			"The SecureOn password must be hex digits only, for example 11-22-33-44.",
		},
		{
			"odd number of digits",
			"11-22-33-4",
			nil,
			"The SecureOn password must be hex digits only, for example 11-22-33-44.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodePassword(tt.in)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("got no error, want %q", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("error is %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("decodePassword: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d bytes, want %d", len(got), len(tt.want))
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("byte %d is %#02x, want %#02x", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestWakeNote(t *testing.T) {
	one := []Send{{Adapter: "eth0"}}
	two := []Send{{Adapter: "eth0"}, {Adapter: "wlan0"}}

	tests := []struct {
		name   string
		sent   []Send
		failed []Send
		want   string
	}{
		{"one network, all sent", one, nil, ""},
		{
			"two networks, all sent",
			two,
			nil,
			"Sent on 2 networks, because this computer is on more than one. Only the one the sleeping machine is plugged into matters.",
		},
		{
			"one failed",
			one,
			[]Send{{Adapter: "wlan0", Error: sendFailure}},
			"The packet went out on 1 of 2 networks. The ones that failed are listed below.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wakeNote(tt.sent, tt.failed); got != tt.want {
				t.Fatalf("note is %q, want %q", got, tt.want)
			}
		})
	}
}
