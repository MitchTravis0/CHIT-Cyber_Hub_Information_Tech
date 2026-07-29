package wifi

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
)

// The real output of iw on this machine, captured before the parsers were
// written, so the fixtures are iw's output rather than this package's idea of it.
const iwDevFixture = `phy#0
	Unnamed/non-netdev interface
		wdev 0x3
		addr f6:28:9d:04:fa:3f
		type P2P-device
	Interface wlan0
		ifindex 3
		wdev 0x2
		addr f4:28:9d:04:fa:3f
		ssid EV
		type managed
		channel 3 (2422 MHz), width: 20 MHz, center1: 2422 MHz
		txpower 3.00 dBm
`

const iwLinkFixture = `Connected to 38:ff:36:90:a8:a8 (on wlan0)
	SSID: EV
	freq: 2422.0
	RX: 171498906 bytes (682721 packets)
	TX: 864691387 bytes (662812 packets)
	signal: -39 dBm
	rx bitrate: 144.4 MBit/s MCS 15 short GI
	tx bitrate: 144.4 MBit/s MCS 15 short GI
	bss flags: short-preamble
	dtim period: 1
	beacon int: 100
`

const iwInfoFixture = `Interface wlan0
	ifindex 3
	wdev 0x2
	addr f4:28:9d:04:fa:3f
	ssid EV
	type managed
	wiphy 0
	channel 3 (2422 MHz), width: 20 MHz, center1: 2422 MHz
	txpower 3.00 dBm
`

func TestParseIwDev(t *testing.T) {
	names := parseIwDev(iwDevFixture)
	if len(names) != 1 || names[0] != "wlan0" {
		t.Fatalf("got %v, want exactly [wlan0]: the P2P-device is not a card", names)
	}
	if len(parseIwDev("")) != 0 {
		t.Error("no output yields no interfaces")
	}
	if got := parseIwDev("	Interface wlan9\n		ifindex 4\n"); len(got) != 0 {
		t.Errorf("an interface with no type line must be dropped, got %v", got)
	}
	two := iwDevFixture + "	Interface wlan1\n		type managed\n"
	if got := parseIwDev(two); len(got) != 2 {
		t.Errorf("two managed interfaces must both come back, got %v", got)
	}
}

func TestParseIwLink(t *testing.T) {
	link := parseIwLink(iwLinkFixture)
	if !link.Connected {
		t.Error("the fixture is connected")
	}
	if link.SSID != "EV" {
		t.Errorf("ssid = %q", link.SSID)
	}
	if link.BSSID != "38:ff:36:90:a8:a8" {
		t.Errorf("bssid = %q", link.BSSID)
	}
	// iw prints the frequency as a float; reading it as an int would give 2.
	if link.FrequencyMHz != 2422 {
		t.Errorf("frequency = %d, want 2422", link.FrequencyMHz)
	}
	if link.SignalDBm != -39 {
		t.Errorf("signal = %d, want -39", link.SignalDBm)
	}
	if math.Abs(link.RxMbps-144.4) > 1e-9 || math.Abs(link.TxMbps-144.4) > 1e-9 {
		t.Errorf("rates = %v / %v, want 144.4 each", link.RxMbps, link.TxMbps)
	}
}

func TestParseIwLinkNotConnected(t *testing.T) {
	link := parseIwLink("Not connected.\n")
	if link.Connected {
		t.Error("Not connected. must yield Connected false")
	}
	if link.SSID != "" || link.SignalDBm != 0 {
		t.Errorf("nothing else may be filled in: %+v", link)
	}
}

func TestParseIwLinkWithNoSignalLine(t *testing.T) {
	link := parseIwLink("Connected to 38:ff:36:90:a8:a8 (on wlan0)\n\tSSID: EV\n")
	if !link.Connected || link.SSID != "EV" {
		t.Errorf("link = %+v", link)
	}
	if link.SignalDBm != 0 {
		t.Errorf("an absent signal must stay zero, got %d", link.SignalDBm)
	}
}

func TestParseIwInfo(t *testing.T) {
	channel, mhz, width := parseIwInfo(iwInfoFixture)
	if channel != 3 || mhz != 2422 || width != 20 {
		t.Errorf("got %d %d %d, want 3 2422 20", channel, mhz, width)
	}

	channel, mhz, width = parseIwInfo("	channel 149 (5745 MHz)\n")
	if channel != 149 || mhz != 5745 || width != 0 {
		t.Errorf("got %d %d %d, want 149 5745 0", channel, mhz, width)
	}

	if c, m, w := parseIwInfo("Interface wlan0\n"); c != 0 || m != 0 || w != 0 {
		t.Errorf("no channel line must yield zeroes, got %d %d %d", c, m, w)
	}
}

func TestBandFor(t *testing.T) {
	tests := map[int]string{
		2412: "2.4 GHz", 2484: "2.4 GHz", 2400: "2.4 GHz", 2500: "2.4 GHz",
		4900: "5 GHz", 5180: "5 GHz", 5745: "5 GHz", 5895: "5 GHz", 5924: "5 GHz",
		5925: "6 GHz", 6425: "6 GHz", 7125: "6 GHz",
		7126: "", 0: "", 2399: "", 4899: "", 2501: "",
	}
	for mhz, want := range tests {
		if got := bandFor(mhz); got != want {
			t.Errorf("bandFor(%d) = %q, want %q", mhz, got, want)
		}
	}
}

func TestChannelFor(t *testing.T) {
	tests := map[int]int{
		2412: 1, 2422: 3, 2437: 6, 2472: 13,
		2484: 14, // does not follow the 5 MHz rule, which is why it is a case
		5180: 36, 5745: 149, 5825: 165,
		5955: 1, 6175: 45,
		0: 0, 100: 0, 2413: 0, 8000: 0,
	}
	for mhz, want := range tests {
		if got := channelFor(mhz); got != want {
			t.Errorf("channelFor(%d) = %d, want %d", mhz, got, want)
		}
	}
}

func TestFrequencyFor(t *testing.T) {
	tests := map[int]int{
		1: 2412, 3: 2422, 13: 2472, 14: 2484,
		36: 5180, 149: 5745, 165: 5825,
		0: 0, -1: 0, 15: 0, 200: 0,
	}
	for channel, want := range tests {
		if got := frequencyFor(channel); got != want {
			t.Errorf("frequencyFor(%d) = %d, want %d", channel, got, want)
		}
	}
}

func TestChannelAndFrequencyRoundTrip(t *testing.T) {
	// Every channel the reverse function knows must survive a round trip, or a
	// Windows reading (channel only) and a Linux reading (frequency only) would
	// not line up.
	for _, channel := range []int{1, 6, 11, 13, 14, 36, 40, 44, 48, 149, 153, 157, 161, 165} {
		mhz := frequencyFor(channel)
		if got := channelFor(mhz); got != channel {
			t.Errorf("channel %d -> %d MHz -> channel %d", channel, mhz, got)
		}
	}
}

func TestPercentFromDBm(t *testing.T) {
	// Computed from Microsoft's published 2 * (dbm + 100) rule and written in as
	// literals, not read back out of the function.
	tests := map[int]int{
		-50: 100, -60: 80, -67: 66, -75: 50, -100: 0,
		-120: 0,   // clamped
		-40:  100, // clamped
		-30:  100,
	}
	for dbm, want := range tests {
		if got := percentFromDBm(dbm); got != want {
			t.Errorf("percentFromDBm(%d) = %d, want %d", dbm, got, want)
		}
	}
}

func TestReadingFromDBm(t *testing.T) {
	tests := []struct {
		dbm  int
		want string
	}{
		{-30, "Excellent. This is right next to the access point."},
		{-50, "Excellent. This is right next to the access point."},
		{-51, "Good. Normal for the same room as the access point."},
		{-60, "Good. Normal for the same room as the access point."},
		{-61, "Usable. This is about the edge of what voice and video calls need."},
		{-67, "Usable. This is about the edge of what voice and video calls need."},
		{-68, "Weak. Expect drops and slow transfers at this desk."},
		{-75, "Weak. Expect drops and slow transfers at this desk."},
		{-76, "Very weak. Move closer, or the site needs another access point here."},
		{-95, "Very weak. Move closer, or the site needs another access point here."},
	}
	for _, tt := range tests {
		if got := readingFromDBm(tt.dbm); got != tt.want {
			t.Errorf("readingFromDBm(%d)\n got %q\nwant %q", tt.dbm, got, tt.want)
		}
	}
}

// TestReadingLaddersAgree is what stops the dBm ladder and the percent ladder
// drifting apart. A hand-written second table is exactly what would let them.
func TestReadingLaddersAgree(t *testing.T) {
	for dbm := -100; dbm <= -30; dbm++ {
		pct := percentFromDBm(dbm)
		if got, want := readingFromPercent(pct), readingFromDBm(dbm); got != want {
			t.Fatalf("at %d dBm (%d%%)\n got %q\nwant %q", dbm, pct, got, want)
		}
	}
}

func TestReadingFromPercent(t *testing.T) {
	// The Windows figures a tech actually sees.
	tests := map[int]string{
		100: "Excellent. This is right next to the access point.",
		82:  "Good. Normal for the same room as the access point.",
		80:  "Good. Normal for the same room as the access point.",
		66:  "Usable. This is about the edge of what voice and video calls need.",
		50:  "Weak. Expect drops and slow transfers at this desk.",
		20:  "Very weak. Move closer, or the site needs another access point here.",
		0:   "Very weak. Move closer, or the site needs another access point here.",
	}
	for pct, want := range tests {
		if got := readingFromPercent(pct); got != want {
			t.Errorf("readingFromPercent(%d)\n got %q\nwant %q", pct, got, want)
		}
	}
}

// A two-interface netsh fixture, written to the documented English output. The
// second block is disconnected.
const netshFixture = `
There is 1 interface on the system:

    Name                   : Wi-Fi
    Description            : Intel(R) Wi-Fi 6 AX201 160MHz
    GUID                   : 1234abcd-0000-0000-0000-00000000abcd
    Physical address       : f4:28:9d:04:fa:3f
    State                  : connected
    SSID                   : EV
    BSSID                  : 38:ff:36:90:a8:a8
    Network type           : Infrastructure
    Radio type             : 802.11ac
    Authentication         : WPA2-Personal
    Cipher                 : CCMP
    Connection mode        : Profile
    Band                   : 2.4 GHz
    Channel                : 3
    Receive rate (Mbps)    : 144.4
    Transmit rate (Mbps)   : 144.4
    Signal                 : 82%
    Profile                : EV

    Name                   : Wi-Fi 2
    State                  : disconnected
`

func TestParseNetshInterfaces(t *testing.T) {
	links := parseNetshInterfaces(netshFixture)
	if len(links) != 2 {
		t.Fatalf("got %d links, want 2", len(links))
	}

	first := links[0]
	if first.Interface != "Wi-Fi" || !first.Connected {
		t.Errorf("first = %+v", first)
	}
	if first.SSID != "EV" {
		t.Errorf("ssid = %q", first.SSID)
	}
	// A BSSID holds colons of its own, so only the first one may split the line.
	if first.BSSID != "38:ff:36:90:a8:a8" {
		t.Errorf("bssid = %q, want the whole address", first.BSSID)
	}
	if first.SignalPercent != 82 {
		t.Errorf("signal = %d, want 82", first.SignalPercent)
	}
	if first.SignalDBm != 0 {
		t.Errorf("netsh reports no dBm, so it must stay zero, got %d", first.SignalDBm)
	}
	if math.Abs(first.RxMbps-144.4) > 1e-9 {
		t.Errorf("rx = %v", first.RxMbps)
	}
	if first.Channel != 3 || first.Band != "2.4 GHz" {
		t.Errorf("channel/band = %d %q", first.Channel, first.Band)
	}
	if first.Security != "WPA2-Personal (802.11ac)" {
		t.Errorf("security = %q", first.Security)
	}

	if links[1].Interface != "Wi-Fi 2" || links[1].Connected {
		t.Errorf("second = %+v", links[1])
	}
}

func TestParseNetshOnALocalisedWindows(t *testing.T) {
	// German keys match nothing, so the parser must return no link rather than a
	// link full of zeroes that reads as a connected adapter with no signal.
	const german = `
    Name                   : WLAN
    Status                 : Verbunden
    SSID                   : EV
    Signal                 : 82%
`
	links := parseNetshInterfaces(german)
	if len(links) != 1 {
		t.Fatalf("got %d links", len(links))
	}
	// The Name key happens to be the same word, so a block is produced, but the
	// State key is not and the adapter is honestly reported as not connected.
	if links[0].Connected {
		t.Error("a localised State value must not be read as connected")
	}
}

func TestParseNetshEmpty(t *testing.T) {
	if got := parseNetshInterfaces(""); len(got) != 0 {
		t.Errorf("no output yields no links, got %v", got)
	}
	if got := parseNetshInterfaces("There is 0 interface on the system:\n"); len(got) != 0 {
		t.Errorf("a machine with no adapter yields no links, got %v", got)
	}
}

func TestParseAirportChannel(t *testing.T) {
	tests := []struct {
		in      string
		channel int
		band    string
		width   int
	}{
		{"3 (2GHz, 20MHz)", 3, "2.4 GHz", 20},
		{"149 (5GHz, 80MHz)", 149, "5 GHz", 80},
		{"37 (6GHz, 160MHz)", 37, "6 GHz", 160},
		{"36", 36, "", 0},
		{"", 0, "", 0},
	}
	for _, tt := range tests {
		channel, band, width := parseAirportChannel(tt.in)
		if channel != tt.channel || band != tt.band || width != tt.width {
			t.Errorf("parseAirportChannel(%q) = %d %q %d, want %d %q %d",
				tt.in, channel, band, width, tt.channel, tt.band, tt.width)
		}
	}
}

func TestParseSignalNoise(t *testing.T) {
	tests := map[string]int{
		"-39 dBm / -92 dBm": -39,
		"-39dBm":            -39,
		"-92":               -92,
		"":                  0,
		"not a number":      0,
	}
	for in, want := range tests {
		if got := parseSignalNoise(in); got != want {
			t.Errorf("parseSignalNoise(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestLeadingInt(t *testing.T) {
	tests := map[string]int{
		" -39 dBm": -39, "3 (2422 MHz)": 3, "82%": 82, "144": 144,
		"": 0, "abc": 0, "20 MHz": 20,
	}
	for in, want := range tests {
		if got := leadingInt(in); got != want {
			t.Errorf("leadingInt(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestLeadingFloat(t *testing.T) {
	tests := map[string]float64{
		" 144.4 MBit/s MCS 15 short GI": 144.4,
		"2422.0":                        2422,
		"144":                           144,
		"":                              0,
		"abc":                           0,
		"-39.5 dBm":                     -39.5,
	}
	for in, want := range tests {
		if got := leadingFloat(in); math.Abs(got-want) > 1e-9 {
			t.Errorf("leadingFloat(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestNoteFor(t *testing.T) {
	tests := []struct {
		name  string
		os    string
		links int
		ssid  bool
		want  string
	}{
		{"no adapter on linux", "linux", 0, false, noteNoAdapter},
		{"no adapter on windows", "windows", 0, false, noteNoAdapter},
		{"linux with an adapter", "linux", 1, true, noteLinux},
		{"windows with an adapter", "windows", 1, true, noteWindows},
		{"darwin with an ssid", "darwin", 1, true, noteDarwin},
		{"darwin without an ssid", "darwin", 1, false, noteDarwinNo},
		{"anything else", "freebsd", 0, false, noteOther},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := noteFor(tt.os, tt.links, tt.ssid); got != tt.want {
				t.Errorf("note\n got %q\nwant %q", got, tt.want)
			}
		})
	}

	// The note is always shown, so every one must be a real sentence.
	for _, note := range []string{noteNoAdapter, noteLinux, noteNoIw, noteWindows, noteDarwin, noteDarwinNo, noteOther} {
		if len(note) < 40 || !strings.HasSuffix(note, ".") {
			t.Errorf("note is not a sentence: %q", note)
		}
	}
}

func TestReportSlicesAreNeverNil(t *testing.T) {
	data, err := json.Marshal(Report{Links: []Link{}, Unsupported: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"links":[]`) {
		t.Errorf("links marshalled as null: %s", data)
	}
	if !strings.Contains(string(data), `"unsupported":[]`) {
		t.Errorf("unsupported marshalled as null: %s", data)
	}
}

func TestListFillsTheReadingAndTheNote(t *testing.T) {
	r, err := List()
	if err != nil {
		t.Fatalf("List failed on this machine: %v", err)
	}
	if r.Links == nil || r.Unsupported == nil {
		t.Fatal("both slices must be initialised so neither marshals to null")
	}
	if r.Note == "" {
		t.Fatal("the note is never allowed to be empty: the page always shows it")
	}
	for _, link := range r.Links {
		if !link.Connected {
			continue
		}
		if link.SignalDBm != 0 && link.Reading == "" {
			t.Errorf("%s has a signal and no reading beside it", link.Interface)
		}
		if link.SignalDBm != 0 && link.SignalPercent == 0 {
			t.Errorf("%s has dBm but no percentage, so the two units are not comparable", link.Interface)
		}
	}
}

func TestNoWrites(t *testing.T) {
	// This tool reads and nothing else: no connect, no disconnect, no saved
	// password.
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	banned := []string{"os.Remove", "os.WriteFile", "os.Create", `"connect"`, `"disconnect"`, `"set"`, `"add"`, `"delete"`, "key=clear"}
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
				t.Errorf("%s contains %q; this tool must only read", name, call)
			}
		}
	}
}

// TestParseIwDevDropsANamedNonManagedInterface pins the type filter. The fixture
// above happens to drop its P2P device by the name logic rather than by the type
// check, so the type check itself was unexercised: a monitor-mode interface with
// a real name would have been reported as a card. Found by mutation.
func TestParseIwDevDropsANamedNonManagedInterface(t *testing.T) {
	const withMonitor = `phy#0
	Interface mon0
		ifindex 4
		type monitor
	Interface wlan0
		ifindex 3
		type managed
`
	names := parseIwDev(withMonitor)
	if len(names) != 1 || names[0] != "wlan0" {
		t.Fatalf("got %v, want exactly [wlan0]: a monitor interface is not a card", names)
	}
}

// TestRoundedMHz pins the rounding. iw prints the frequency as a float, and
// reading it with an integer parse instead of rounding is invisible for whole
// numbers and wrong for everything else. Found by mutation.
func TestRoundedMHz(t *testing.T) {
	tests := map[string]int{
		" 2422.0": 2422,
		" 2437.0": 2437,
		" 2437.5": 2438, // rounds up rather than truncating to 2437
		" 5955.4": 5955,
		" 5180":   5180,
		"":        0,
		" nope":   0,
	}
	for in, want := range tests {
		if got := roundedMHz(in); got != want {
			t.Errorf("roundedMHz(%q) = %d, want %d", in, got, want)
		}
	}
}
