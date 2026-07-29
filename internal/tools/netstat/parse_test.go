package netstat

import (
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Real lines captured from this machine's /proc/net before the parser was
// written, so the fixture is the kernel's output rather than this package's idea
// of it.
const (
	procHeader4 = "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode"
	procDocker  = "   0: 010011AC:0035 00000000:0000 0A 00000000:00000000 00:00000000 00000000   974        0 12482 1 00000000490dd859 100 0 0 10 5"
	procLocal   = "   1: 0100007F:B62D 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 6088 2 00000000efe0a35f 100 0 0 10 0"
	procCups6   = "   0: 00000000000000000000000001000000:0277 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 10720 1 000000001418ad59 100 0 0 10 0"
	procUDP     = "  930: 00000000:14E9 00000000:0000 07 00000000:00000000 00:00000000 00000000   969        0 6063 2 000000009671564c 0"
	// An established connection, which must never appear in a list of listeners.
	procEstablished = "   4: 0100007F:8AE2 0100007F:1F90 01 00000000:00000000 00:00000000 00000000  1000        0 44210 1 0000000012345678 100 0 0 10 0"
)

func TestParseProcNetLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		v6      bool
		wantOK  bool
		address string
		port    int
		state   string
		inode   uint64
	}{
		{"docker dns listener", procDocker, false, true, "172.17.0.1", 53, "0A", 12482},
		{"loopback listener", procLocal, false, true, "127.0.0.1", 46637, "0A", 6088},
		{"ipv6 cups listener", procCups6, true, true, "::1", 631, "0A", 10720},
		{"udp socket", procUDP, false, true, "0.0.0.0", 5353, "07", 6063},
		{"established is parsed but not listening", procEstablished, false, true, "127.0.0.1", 35554, "01", 44210},
		{"the header line", procHeader4, false, false, "", 0, "", 0},
		{"a short line", "   0: 0100007F:0050", false, false, "", 0, "", 0},
		// A truncated line with more than a couple of fields but fewer than ten.
		// Without the guard this reaches fields[9] and panics, which is why the
		// two-field case above is not enough on its own.
		{"a line truncated mid-row", "   0: 0100007F:0050 00000000:0000 0A 00000000:00000000 00:00000000", false, false, "", 0, "", 0},
		{"a line one field short", "   0: 0100007F:0050 00000000:0000 0A a b c d 0", false, false, "", 0, "", 0},
		{"empty", "", false, false, "", 0, "", 0},
		{"a non-hex port", "   0: 0100007F:ZZZZ 00000000:0000 0A a b c d 0 0 123", false, false, "", 0, "", 0},
		{"a non-numeric inode", "   0: 0100007F:0050 00000000:0000 0A a b c d 0 xyz", false, false, "", 0, "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row, ok := parseProcNetLine(tt.line, tt.v6)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if row.Address != tt.address || row.Port != tt.port || row.State != tt.state || row.Inode != tt.inode {
				t.Errorf("got %+v, want %s:%d state %s inode %d",
					row, tt.address, tt.port, tt.state, tt.inode)
			}
		})
	}
}

func TestOnlyListeningTCPCounts(t *testing.T) {
	// The listener filter lives in the linux collector, so this pins the constant
	// it turns on rather than the loop.
	listening, _ := parseProcNetLine(procLocal, false)
	established, _ := parseProcNetLine(procEstablished, false)
	if listening.State != stateListen {
		t.Errorf("a real listener has state %q, not %q", listening.State, stateListen)
	}
	if established.State == stateListen {
		t.Error("an established connection must not look like a listener")
	}
}

func TestParseHexAddress4(t *testing.T) {
	// Every expected value here came from python3
	// (socket.inet_ntoa(struct.pack('<I', int(hex, 16)))) before this function
	// was written, so the parser is not checked against itself.
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"0100007F", "127.0.0.1", true},
		{"00000000", "0.0.0.0", true},
		{"010011AC", "172.17.0.1", true},
		{"9915282E", "46.40.21.153", true},
		{"3601A8C0", "192.168.1.54", true},
		{"0100007", "", false},
		{"0100007FF", "", false},
		{"ZZZZZZZZ", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, ok := parseHexAddress4(tt.in)
		if ok != tt.ok || got != tt.want {
			t.Errorf("parseHexAddress4(%q) = %q,%v want %q,%v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestParseHexAddress6(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"00000000000000000000000000000000", "::", true},
		{"00000000000000000000000001000000", "::1", true},
		{"0000000000000000FFFF00000100007F", "127.0.0.1", true},
		{"0000000000000000000000000100007", "", false},
		{"00000000000000000000000000000ZZZ", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, ok := parseHexAddress6(tt.in)
		if ok != tt.ok || got != tt.want {
			t.Errorf("parseHexAddress6(%q) = %q,%v want %q,%v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestReachFor(t *testing.T) {
	tests := map[string]string{
		"0.0.0.0":      ReachEverywhere,
		"::":           ReachEverywhere,
		"127.0.0.1":    ReachLocal,
		"127.0.0.54":   ReachLocal,
		"::1":          ReachLocal,
		"10.40.21.153": ReachOne,
		"172.17.0.1":   ReachOne,
		"fe80::1":      ReachOne,
		// An address that cannot be read is reported as "one address" rather than
		// "local only": understating the exposure would be the dangerous direction.
		"not an address": ReachOne,
		"":               ReachOne,
	}
	for in, want := range tests {
		if got := ReachFor(in); got != want {
			t.Errorf("ReachFor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNtohsPort(t *testing.T) {
	// Every DWORD here was produced by python3
	// (struct.unpack('<I', struct.pack('>H', port) + b'\x00\x00')) before this
	// function was written. Getting the byte order wrong is the most common
	// mistake in the GetExtendedTcpTable API and the reason this test exists.
	tests := map[uint32]int{
		0x00005000: 80,
		0x00001600: 22,
		0x0000901F: 8080,
		0x00007702: 631,
		0x0000FFFF: 65535,
		0x00000100: 1,
		0x00000000: 0,
	}
	for in, want := range tests {
		if got := ntohsPort(in); got != want {
			t.Errorf("ntohsPort(0x%08X) = %d, want %d", in, got, want)
		}
	}
}

// A two-process lsof -F fixture, written to the documented field format: one
// letter per line, a process set introduced by p, each file by f.
const lsofFixture = `p512
csshd
f3
PTCP
n*:22
TST=LISTEN
f4
PTCP
n[::1]:22
TST=LISTEN
p900
cmDNSResponder
f7
PUDP
n*:5353
f8
PTCP
n127.0.0.1:49152
TST=ESTABLISHED
`

func TestParseLsofFields(t *testing.T) {
	files := parseLsofFields(lsofFixture)
	if len(files) != 4 {
		t.Fatalf("got %d files, want 4", len(files))
	}
	if files[0].PID != 512 || files[0].Command != "sshd" || files[0].Name != "*:22" || files[0].State != "LISTEN" {
		t.Errorf("first file = %+v", files[0])
	}
	if files[2].PID != 900 || files[2].Command != "mDNSResponder" || files[2].Protocol != "UDP" {
		t.Errorf("third file = %+v", files[2])
	}
	if files[3].State != "ESTABLISHED" {
		t.Errorf("the state must survive so the caller can drop it: %+v", files[3])
	}
}

func TestParseLsofFieldsDropsAFileWithNoName(t *testing.T) {
	files := parseLsofFields("p1\ncinit\nf3\nPTCP\nTST=LISTEN\n")
	if len(files) != 0 {
		t.Fatalf("a file record with no n field must be dropped, got %+v", files)
	}
}

func TestSplitLsofName(t *testing.T) {
	tests := []struct {
		in      string
		v6      bool
		address string
		port    int
		ok      bool
	}{
		{"*:22", false, "0.0.0.0", 22, true},
		{"*:5353", true, "::", 5353, true},
		{"127.0.0.1:631", false, "127.0.0.1", 631, true},
		{"[::1]:631", true, "::1", 631, true},
		{"10.0.0.5:8080->1.2.3.4:443", false, "", 0, false},
		{"*:*", false, "", 0, false},
		{"", false, "", 0, false},
		{"nocolon", false, "", 0, false},
	}
	for _, tt := range tests {
		address, port, ok := splitLsofName(tt.in, tt.v6)
		if ok != tt.ok || address != tt.address || port != tt.port {
			t.Errorf("splitLsofName(%q) = %q,%d,%v want %q,%d,%v",
				tt.in, address, port, ok, tt.address, tt.port, tt.ok)
		}
	}
}

func TestParseNetstatLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantOK   bool
		protocol string
		address  string
		port     int
	}{
		{"tcp4 wildcard listener", "tcp4       0      0  *.22                   *.*                    LISTEN", true, "tcp", "0.0.0.0", 22},
		{"tcp4 loopback listener", "tcp4       0      0  127.0.0.1.631          *.*                    LISTEN", true, "tcp", "127.0.0.1", 631},
		{"tcp6 listener", "tcp6       0      0  *.22                   *.*                    LISTEN", true, "tcp6", "::", 22},
		{"tcp6 loopback listener", "tcp6       0      0  ::1.631                *.*                    LISTEN", true, "tcp6", "::1", 631},
		{"udp4 has no state", "udp4       0      0  *.5353                 *.*", true, "udp", "0.0.0.0", 5353},
		{"an established tcp line is dropped", "tcp4       0      0  10.0.0.5.51234         1.2.3.4.443            ESTABLISHED", false, "", "", 0},
		{"a unix socket line is dropped", "unix  0 0 /var/run/x", false, "", "", 0},
		{"the header", "Active Internet connections (including servers)", false, "", "", 0},
		{"empty", "", false, "", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, ok := parseNetstatLine(tt.line)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if entry.Protocol != tt.protocol || entry.Address != tt.address || entry.Port != tt.port {
				t.Errorf("got %+v, want %s %s:%d", entry, tt.protocol, tt.address, tt.port)
			}
		})
	}
}

func TestServiceName(t *testing.T) {
	tests := map[int]string{
		22:    "SSH and SFTP",
		80:    "HTTP",
		443:   "HTTPS",
		3389:  "Remote Desktop",
		9100:  "Raw printing (JetDirect)",
		445:   "SMB file sharing",
		53:    "DNS",
		161:   "SNMP",
		49152: "",
		0:     "",
	}
	for port, want := range tests {
		if got := ServiceName(port); got != want {
			t.Errorf("ServiceName(%d) = %q, want %q", port, got, want)
		}
	}
	if len(services) != 54 {
		t.Errorf("the port list has %d entries, want the 54 the reference cards ship", len(services))
	}
}

// TestServiceNamesMatchEtcServices cross-checks the port numbers against this
// machine's /etc/services, which is an independent list. Skipped where the file
// does not exist (Windows) rather than being a Linux-only test.
//
// Presence confirms a number is not invented. **Absence proves nothing**, and
// the original version of this test got that backwards: it required every port
// to appear, which held on an Arch machine carrying the full IANA list and
// failed CI on both other platforms. Ubuntu's trimmed copy omits 9100
// (HP JetDirect raw printing, which the Raw Printer Test tool is built around),
// 27017, 3128, 5900, 1723, 1900, 3268, 8443 and 1521; macOS omits 27017 and
// 6379. Every one of those is correct and widely known, so failing on them was
// asserting a property of the host, not of this package.
//
// The exact names and the count of all 54 entries are pinned by the literal
// table in TestServiceName above, which needs no host file. This test is the
// bonus cross-check, with a floor so it cannot pass vacuously.
func TestServiceNamesMatchEtcServices(t *testing.T) {
	data, err := os.ReadFile("/etc/services")
	if err != nil {
		t.Skip("no /etc/services on this machine to check against")
	}
	known := map[int]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		portText, _, _ := strings.Cut(fields[1], "/")
		if port, err := strconv.Atoi(portText); err == nil {
			known[port] = true
		}
	}
	confirmed, missing := 0, []int{}
	for port := range services {
		if known[port] {
			confirmed++
			continue
		}
		missing = append(missing, port)
	}
	sort.Ints(missing)

	// The floor is what stops this passing over nothing: a file that confirms
	// almost none of our ports is not an oracle, it is a stub, and the skip
	// above only catches the file being absent entirely. 30 of 54 is comfortably
	// below what any real services file provides and far above a stub.
	if confirmed < 30 {
		t.Skipf("only %d of %d ports appear in this machine's /etc/services, which is too trimmed to check against",
			confirmed, len(services))
	}
	t.Logf("%d of %d ports confirmed against /etc/services; not listed on this machine: %v",
		confirmed, len(services), missing)
}

func TestSortEntries(t *testing.T) {
	entries := []Entry{
		{Protocol: "udp6", Address: "::", Port: 80},
		{Protocol: "tcp", Address: "0.0.0.0", Port: 80},
		{Protocol: "tcp6", Address: "::", Port: 80},
		{Protocol: "udp", Address: "0.0.0.0", Port: 80},
		{Protocol: "tcp", Address: "0.0.0.0", Port: 22},
		{Protocol: "tcp", Address: "127.0.0.1", Port: 22},
	}
	SortEntries(entries)

	want := []string{
		"tcp 0.0.0.0 22", "tcp 127.0.0.1 22",
		"tcp 0.0.0.0 80", "tcp6 :: 80", "udp 0.0.0.0 80", "udp6 :: 80",
	}
	for i, entry := range entries {
		got := entry.Protocol + " " + entry.Address + " " + strconv.Itoa(entry.Port)
		if got != want[i] {
			t.Errorf("row %d = %q, want %q", i, got, want[i])
		}
	}
}

func TestNoteFor(t *testing.T) {
	tests := []struct {
		name         string
		os           string
		processNames bool
		hidden       int
		want         string
	}{
		{"linux with hidden owners", "linux", true, 12, noteLinuxHidden},
		{"linux with nothing hidden", "linux", true, 0, noteLinuxAll},
		{"windows", "windows", true, 0, noteWindows},
		{"darwin with lsof", "darwin", true, 3, noteDarwinLsof},
		{"darwin without lsof", "darwin", false, 0, noteDarwinPlain},
		{"anything else", "freebsd", false, 0, noteOther},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := noteFor(tt.os, tt.processNames, tt.hidden); got != tt.want {
				t.Errorf("note\n got %q\nwant %q", got, tt.want)
			}
		})
	}

	// Every note must be a real sentence, because the page always shows one.
	for _, note := range []string{noteLinuxHidden, noteLinuxAll, noteWindows, noteDarwinLsof, noteDarwinPlain, noteOther} {
		if len(note) < 40 || !strings.HasSuffix(note, ".") {
			t.Errorf("note is not a sentence: %q", note)
		}
	}
}

func TestReportSlicesAreNeverNil(t *testing.T) {
	data, err := json.Marshal(Report{Entries: []Entry{}, Unsupported: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"entries":[]`) {
		t.Errorf("entries marshalled as null: %s", data)
	}
	if !strings.Contains(string(data), `"unsupported":[]`) {
		t.Errorf("unsupported marshalled as null: %s", data)
	}
}

func TestListNeverReturnsANilSlice(t *testing.T) {
	r, err := List()
	if err != nil {
		t.Fatalf("List failed on this machine: %v", err)
	}
	if r.Entries == nil || r.Unsupported == nil {
		t.Fatal("List must initialise both slices so neither marshals to null")
	}
	if r.Note == "" {
		t.Fatal("the note is never allowed to be empty: the page always shows it")
	}
}

func TestNoWrites(t *testing.T) {
	// This tool reads and nothing else. No port is closed and no program stopped.
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	banned := []string{"os.Remove", "os.WriteFile", "os.Create", "SetValue", "Process.Kill", "syscall.Kill"}
	for _, file := range files {
		name := file.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, call := range banned {
			if strings.Contains(string(src), call) {
				t.Errorf("%s calls %s; this tool must only read", name, call)
			}
		}
	}
}

func TestTrimBrackets(t *testing.T) {
	tests := map[string]string{
		"[::1]":     "::1",
		"::1":       "::1",
		"127.0.0.1": "127.0.0.1",
		"":          "",
	}
	for in, want := range tests {
		if got := trimBrackets(in); got != want {
			t.Errorf("trimBrackets(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeWildcard(t *testing.T) {
	tests := []struct {
		host string
		v6   bool
		want string
	}{
		{"*", false, "0.0.0.0"},
		{"*", true, "::"},
		{"", false, "0.0.0.0"},
		{"", true, "::"},
		{"127.0.0.1", false, "127.0.0.1"},
		{"::1", true, "::1"},
	}
	for _, tt := range tests {
		if got := normalizeWildcard(tt.host, tt.v6); got != tt.want {
			t.Errorf("normalizeWildcard(%q,%v) = %q, want %q", tt.host, tt.v6, got, tt.want)
		}
	}
}
