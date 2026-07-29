package usbhist

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestFiletimeToTime(t *testing.T) {
	// Both expected values were computed with python3 from the two published
	// definitions before this test was written:
	//   datetime(1601,1,1,tzinfo=utc) + timedelta(microseconds=ticks//10)
	// An encoder and its own inverse can agree on the same mistake, so nothing
	// here is derived from filetimeToTime.
	tests := []struct {
		name  string
		ticks uint64
		want  string
		ok    bool
	}{
		{"the unix epoch boundary", 116444736000000000, "1970-01-01T00:00:00Z", true},
		{"a real install time", 133700000000000000, "2024-09-05T08:53:20Z", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := make([]byte, 8)
			binary.LittleEndian.PutUint64(raw, tt.ticks)

			got, ok := filetimeToTime(raw)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if formatted := got.Format(timeLayout); formatted != tt.want {
				t.Errorf("filetimeToTime(%d) = %s, want %s", tt.ticks, formatted, tt.want)
			}
		})
	}
}

func TestFiletimeToTimeRefusesRubbish(t *testing.T) {
	zero := make([]byte, 8)
	if _, ok := filetimeToTime(zero); ok {
		t.Error("a zero timestamp was accepted, which would show 1601 as an install date")
	}
	if _, ok := filetimeToTime(make([]byte, 7)); ok {
		t.Error("a seven byte value was accepted")
	}
	if _, ok := filetimeToTime(nil); ok {
		t.Error("a nil value was accepted")
	}
}

// The exact little-endian bytes of 133700000000000000, from python3's
// struct.pack('<Q', ...), so the byte order itself is pinned by an independent
// implementation rather than by this package.
func TestFiletimeByteOrder(t *testing.T) {
	raw := []byte{0, 64, 120, 14, 113, 255, 218, 1}

	got, ok := filetimeToTime(raw)
	if !ok {
		t.Fatal("filetimeToTime refused a valid timestamp")
	}
	if formatted := got.Format(timeLayout); formatted != "2024-09-05T08:53:20Z" {
		t.Errorf("got %s, want 2024-09-05T08:53:20Z: the bytes are little-endian", formatted)
	}
}

func TestParseUSBStorKey(t *testing.T) {
	tests := []struct {
		name                       string
		key                        string
		wantMfg, wantProd, wantRev string
	}{
		{
			name: "a real key", key: "Disk&Ven_SanDisk&Prod_Cruzer_Blade&Rev_1.00",
			wantMfg: "SanDisk", wantProd: "Cruzer Blade", wantRev: "1.00",
		},
		{
			name: "no vendor", key: "Disk&Prod_Generic_Drive&Rev_2.00",
			wantProd: "Generic Drive", wantRev: "2.00",
		},
		{name: "only the disk marker", key: "Disk&"},
		{name: "empty", key: ""},
		{
			name: "a vendor with several underscores", key: "Disk&Ven_Kingston_Digital_Inc&Prod_X",
			wantMfg: "Kingston Digital Inc", wantProd: "X",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mfg, prod, rev := parseUSBStorKey(tt.key)
			if mfg != tt.wantMfg || prod != tt.wantProd || rev != tt.wantRev {
				t.Errorf("got (%q, %q, %q), want (%q, %q, %q)",
					mfg, prod, rev, tt.wantMfg, tt.wantProd, tt.wantRev)
			}
		})
	}
}

func TestParseVidPidKey(t *testing.T) {
	tests := []struct {
		key                     string
		wantVendor, wantProduct string
	}{
		{"VID_1D6B&PID_0002", "1d6b", "0002"},
		{"VID_1D6B&PID_0002&MI_00", "1d6b", "0002"},
		{"VID_27C6&PID_609C", "27c6", "609c"},
		{"VID_1D6B", "1d6b", ""},
		{"PID_0002", "", "0002"},
		{"", "", ""},
		{"nonsense", "", ""},
		{"VID_ZZZZ&PID_0002", "", "0002"},
	}
	for _, tt := range tests {
		vendor, product := parseVidPidKey(tt.key)
		if vendor != tt.wantVendor || product != tt.wantProduct {
			t.Errorf("parseVidPidKey(%q) = (%q, %q), want (%q, %q)",
				tt.key, vendor, product, tt.wantVendor, tt.wantProduct)
		}
	}
}

func TestParseDeviceDesc(t *testing.T) {
	tests := []struct{ in, want string }{
		{"@usbstor.inf,%disk_devdesc%;Disk drive", "Disk drive"},
		{"@input.inf,%hid.devicedesc%;USB Input Device", "USB Input Device"},
		{"Plain description", "Plain description"},
		{"  spaced  ", "spaced"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := parseDeviceDesc(tt.in); got != tt.want {
			t.Errorf("parseDeviceDesc(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestKindFromService(t *testing.T) {
	tests := []struct{ in, want string }{
		{"USBSTOR", KindStorage},
		{"usbstor", KindStorage},
		{"disk", KindStorage},
		{"hidusb", KindInput},
		{"kbdhid", KindInput},
		{"mouhid", KindInput},
		{"usbaudio", KindAudio},
		{"usbvideo", KindVideo},
		{"usbhub3", KindHub},
		{"usbprint", KindPrinter},
		{"rndismp", KindNetwork},
		{"something-new", KindOther},
		{"", KindOther},
	}
	for _, tt := range tests {
		if got := kindFromService(tt.in); got != tt.want {
			t.Errorf("kindFromService(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestKindFromClass(t *testing.T) {
	tests := []struct{ in, want string }{
		{"08", KindStorage},
		{"03", KindInput},
		{"01", KindAudio},
		{"0e", KindVideo},
		{"0E", KindVideo},
		{"09", KindHub},
		{"07", KindPrinter},
		{"02", KindNetwork},
		{"0a", KindNetwork},
		// Class 00 means "look at the interface", not "other".
		{"00", ""},
		{"", ""},
		{"ff", KindOther},
		{"ef", ""},
	}
	for _, tt := range tests {
		if got := kindFromClass(tt.in); got != tt.want {
			t.Errorf("kindFromClass(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseSysfsHex(t *testing.T) {
	tests := []struct{ in, want string }{
		{"1d6b\n", "1d6b"},
		{"1D6B", "1d6b"},
		{"  1d6b  ", "1d6b"},
		{"27c6", "27c6"},
		{"", ""},
		{"zzzz", ""},
		{"not hex at all", ""},
	}
	for _, tt := range tests {
		if got := parseSysfsHex(tt.in); got != tt.want {
			t.Errorf("parseSysfsHex(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseProfilerHex(t *testing.T) {
	tests := []struct {
		in                 string
		wantID, wantVendor string
	}{
		{"0x05ac  (Apple Inc.)", "05ac", "Apple Inc."},
		{"0x1d6b", "1d6b", ""},
		{"0x27C6 (Goodix)", "27c6", "Goodix"},
		{"apple_vendor_id", "", ""},
		{"", "", ""},
		{"(Just A Name)", "", "Just A Name"},
	}
	for _, tt := range tests {
		id, vendor := parseProfilerHex(tt.in)
		if id != tt.wantID || vendor != tt.wantVendor {
			t.Errorf("parseProfilerHex(%q) = (%q, %q), want (%q, %q)",
				tt.in, id, vendor, tt.wantID, tt.wantVendor)
		}
	}
}

func TestNoteFor(t *testing.T) {
	if got := noteFor("windows"); !strings.Contains(got, "records USB storage devices it has seen before") {
		t.Errorf("windows note = %q", got)
	}
	if got := noteFor("linux"); got != noteLinux {
		t.Errorf("linux note = %q", got)
	}
	if got := noteFor("darwin"); got != noteDarwin {
		t.Errorf("darwin note = %q", got)
	}
	if got := noteFor("plan9"); got != noteOther {
		t.Errorf("unknown OS note = %q", got)
	}
	for _, os := range []string{"windows", "linux", "darwin", "plan9"} {
		if noteFor(os) == "" {
			t.Errorf("the note for %s is empty, and an empty history with no note reads as a broken tool", os)
		}
	}
}

func TestReportNoteIsNeverEmpty(t *testing.T) {
	r, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if r.Note == "" {
		t.Error("Report.Note is empty: a tech would take an empty list to mean nothing was ever plugged in")
	}
}

func TestReportSlicesAreNeverNil(t *testing.T) {
	r, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, `"devices":null`) {
		t.Error("devices marshalled as null, which breaks the page")
	}
	if strings.Contains(text, `"unsupported":null`) {
		t.Error("unsupported marshalled as null, which breaks the page")
	}
}

func TestNoDeviceRowIsNameless(t *testing.T) {
	r, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, device := range r.Devices {
		if strings.TrimSpace(device.Name) == "" {
			t.Errorf("a device row has no name at all: %+v", device)
		}
	}
}

func TestSortDevices(t *testing.T) {
	devices := []Device{
		{Name: "Disk 10", Kind: KindStorage, Connected: true},
		{Name: "Old Stick", Kind: KindStorage, Connected: false},
		{Name: "Keyboard", Kind: KindInput, Connected: true},
		{Name: "Disk 2", Kind: KindStorage, Connected: true},
	}
	sortDevices(devices)

	got := []string{devices[0].Name, devices[1].Name, devices[2].Name, devices[3].Name}
	want := []string{"Disk 2", "Disk 10", "Keyboard", "Old Stick"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v: connected first, then storage before input, then numbers as numbers", got, want)
		}
	}
}

func TestNothingInThisPackageWrites(t *testing.T) {
	for _, name := range []string{"usbhist.go", "parse.go"} {
		data, err := readSource(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"SetValue", "CreateKey", "DeleteKey", "os.Remove", "os.WriteFile", "os.Create",
		} {
			if strings.Contains(data, forbidden) {
				t.Errorf("%s contains %q: this tool must never change anything", name, forbidden)
			}
		}
	}
}

func readSource(name string) (string, error) {
	data, err := os.ReadFile(name)
	return string(data), err
}

// A composite device declares class ef ("miscellaneous", the Interface
// Association Descriptor marker) and puts its real class on the first
// interface, exactly as class 00 does. Treating ef as "other" reported a real
// webcam on this machine as Other instead of Camera.
func TestKindFromClassDefersToTheInterface(t *testing.T) {
	for _, code := range []string{"00", "ef", "EF", ""} {
		if got := kindFromClass(code); got != "" {
			t.Errorf("kindFromClass(%q) = %q, want \"\" so the caller reads the interface class", code, got)
		}
	}
	// The interface class is then what decides it.
	if got := kindFromClass("0e"); got != KindVideo {
		t.Errorf("kindFromClass(\"0e\") = %q, want %q", got, KindVideo)
	}
}
