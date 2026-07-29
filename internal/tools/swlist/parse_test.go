package swlist

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestParseInstallDate(t *testing.T) {
	tests := map[string]string{
		"20260304":   "2026-03-04",
		"20261231":   "2026-12-31",
		"2026030":    "", // seven characters
		"202603045":  "",
		"00000000":   "",
		"20261345":   "", // month 13, day 45
		"20260230":   "", // 30 February
		"":           "",
		"not a date": "",
	}
	for in, want := range tests {
		if got := parseInstallDate(in); got != want {
			t.Errorf("parseInstallDate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSkipEntry(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]string
		want   bool
	}{
		{"a plain program", map[string]string{"DisplayName": "7-Zip"}, false},
		{"a system component", map[string]string{"DisplayName": "x", "SystemComponent": "1"}, true},
		{"SystemComponent 0 is kept", map[string]string{"DisplayName": "x", "SystemComponent": "0"}, false},
		{"a child of another entry", map[string]string{"DisplayName": "x", "ParentKeyName": "Office"}, true},
		{"a child by display name", map[string]string{"DisplayName": "x", "ParentDisplayName": "Office"}, true},
		{"a security update", map[string]string{"DisplayName": "x", "ReleaseType": "Security Update"}, true},
		{"an update", map[string]string{"DisplayName": "x", "ReleaseType": "Update"}, true},
		{"a hotfix", map[string]string{"DisplayName": "x", "ReleaseType": "Hotfix"}, true},
		{"lower case still skipped", map[string]string{"DisplayName": "x", "ReleaseType": "update"}, true},
		{"an empty release type is kept", map[string]string{"DisplayName": "x", "ReleaseType": ""}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := skipEntry(tt.values); got != tt.want {
				t.Errorf("skipEntry(%v) = %v, want %v", tt.values, got, tt.want)
			}
		})
	}
}

func TestSizeFromKilobytes(t *testing.T) {
	tests := map[int64]int64{1024: 1048576, 1: 1024, 0: 0, -5: 0}
	for kb, want := range tests {
		if got := sizeFromKilobytes(kb); got != want {
			t.Errorf("sizeFromKilobytes(%d) = %d, want %d", kb, got, want)
		}
	}
}

// Real "pacman -Qi" output from this machine, captured before the parser was
// written. The second block has a date in a layout the parser knows and the
// third has one it does not.
const pacmanFixture = `Name            : 1password-beta
Version         : 8.12.24_31.BETA-31.1
Description     : Password manager and secure wallet
Architecture    : x86_64
Depends On      : hicolor-icon-theme  libgtk-3.so=0  nss  xdg-utils
Installed Size  : 533.16 MiB
Packager        : Unknown Packager
Build Date      : Fri 12 Jun 2026 04:00:08 AM PDT
Install Date    : Sun 28 Jun 2026 12:09:07 AM PDT
Install Reason  : Explicitly installed

Name            : bash
Version         : 5.3.3-1
Installed Size  : 8.72 MiB
Packager        : Frederik Schwan <freswa@archlinux.org>
Install Date    : Mon 06 Jul 2026 01:02:03 AM PDT

Name            : zlib
Version         : 1:1.3.1-2
Installed Size  : 332.00 B
Packager        : Levente Polyak <anthraxx@archlinux.org>
Install Date    : lundi 6 juillet 2026, 01:02:03 (UTC-0700)
`

func TestParsePacman(t *testing.T) {
	programs := parsePacman(pacmanFixture)
	if len(programs) != 3 {
		t.Fatalf("got %d programs, want 3", len(programs))
	}

	first := programs[0]
	if first.Name != "1password-beta" || first.Version != "8.12.24_31.BETA-31.1" {
		t.Errorf("first = %+v", first)
	}
	if first.Publisher != "" {
		t.Errorf("pacman's Unknown Packager placeholder must not reach the screen, got %q", first.Publisher)
	}
	if first.InstalledOn != "2026-06-28" {
		t.Errorf("installed = %q, want 2026-06-28", first.InstalledOn)
	}
	// 533.16 MiB. The literal was computed with python3 before it was written in,
	// so a KiB/MiB slip in the parser fails here.
	if first.SizeBytes != 559058780 {
		t.Errorf("size = %d, want 559058780", first.SizeBytes)
	}
	if first.Source != SourcePacman {
		t.Errorf("source = %q", first.Source)
	}

	if programs[1].Publisher != "Frederik Schwan" {
		t.Errorf("the email address must be dropped, got %q", programs[1].Publisher)
	}
	if programs[1].SizeBytes != 9143582 {
		t.Errorf("size = %d, want 9143582", programs[1].SizeBytes)
	}

	// A locale-formatted date the parser does not know leaves the field empty
	// rather than putting a guessed date into an audit export.
	if programs[2].InstalledOn != "" {
		t.Errorf("an unparseable date must be dropped, got %q", programs[2].InstalledOn)
	}
	if programs[2].SizeBytes != 332 {
		t.Errorf("a size in bytes = %d, want 332", programs[2].SizeBytes)
	}
}

func TestParsePacmanEdges(t *testing.T) {
	if got := parsePacman(""); len(got) != 0 {
		t.Errorf("no output yields nothing, got %v", got)
	}
	// A block with no Name is not a package.
	if got := parsePacman("Version         : 1.0\nInstalled Size  : 1.00 MiB\n"); len(got) != 0 {
		t.Errorf("a nameless block must be dropped, got %v", got)
	}
	// A continuation line belongs to the key above it, not to a key of its own.
	const wrapped = `Name            : test
Version         : 1.0
Depends On      : one  two
                  three  four
`
	got := parsePacman(wrapped)
	if len(got) != 1 || got[0].Name != "test" || got[0].Version != "1.0" {
		t.Errorf("a wrapped value must not split the package, got %+v", got)
	}
}

func TestParsePacmanSize(t *testing.T) {
	tests := map[string]int64{
		// Every literal came out of python3 before it was written in.
		"533.16 MiB": 559058780,
		"8.72 KiB":   8929,
		"332.00 B":   332,
		"1.50 GiB":   1610612736,
		"":           0,
		"lots":       0,
		"12":         0,
		"0.00 B":     0,
	}
	for in, want := range tests {
		if got := parsePacmanSize(in); got != want {
			t.Errorf("parsePacmanSize(%q) = %d, want %d", in, got, want)
		}
	}
}

const dpkgFixture = "bash\t5.2.21-2\tMatthias Klose <doko@debian.org>\t7000\n" +
	"coreutils\t9.4-3\t\t18000\n" +
	"short-line\n" +
	"\t1.0\tsomebody\t100\n"

func TestParseDpkg(t *testing.T) {
	programs := parseDpkg(dpkgFixture)
	if len(programs) != 2 {
		t.Fatalf("got %d programs, want 2 (a short line and a nameless one are dropped)", len(programs))
	}
	if programs[0].Name != "bash" || programs[0].Version != "5.2.21-2" {
		t.Errorf("first = %+v", programs[0])
	}
	if programs[0].Publisher != "Matthias Klose" {
		t.Errorf("publisher = %q", programs[0].Publisher)
	}
	if programs[0].SizeBytes != 7000*1024 {
		t.Errorf("dpkg reports kilobytes, got %d", programs[0].SizeBytes)
	}
	if programs[1].Publisher != "" {
		t.Errorf("an empty maintainer stays empty, got %q", programs[1].Publisher)
	}
	if programs[0].InstalledOn != "" {
		t.Error("dpkg records no install date, so it must stay empty")
	}
}

// 1751760000 is 2025-07-06 00:00:00 UTC, computed with python3
// (datetime.fromtimestamp(1751760000, timezone.utc)) before this test was written.
const rpmFixture = "bash\t5.2.26-3.fc40\tFedora Project\t8500000\t1751760000\n" +
	"local-thing\t1.0-1\t(none)\t1024\t0\n" +
	"short\t1.0\n"

func TestParseRPM(t *testing.T) {
	programs := parseRPM(rpmFixture)
	if len(programs) != 2 {
		t.Fatalf("got %d programs, want 2", len(programs))
	}
	if programs[0].Name != "bash" || programs[0].Version != "5.2.26-3.fc40" {
		t.Errorf("first = %+v", programs[0])
	}
	if programs[0].Publisher != "Fedora Project" {
		t.Errorf("publisher = %q", programs[0].Publisher)
	}
	if programs[0].SizeBytes != 8500000 {
		t.Errorf("rpm reports bytes, got %d", programs[0].SizeBytes)
	}
	if programs[0].InstalledOn != "2025-07-06" {
		t.Errorf("installed = %q, want 2025-07-06", programs[0].InstalledOn)
	}
	if programs[1].Publisher != "" {
		t.Errorf("rpm's (none) must not reach the screen, got %q", programs[1].Publisher)
	}
	if programs[1].InstalledOn != "" {
		t.Errorf("a zero timestamp must leave the date empty, got %q", programs[1].InstalledOn)
	}
}

const flatpakFixture = "org.mozilla.firefox\t127.0\tflathub\n" +
	"org.gimp.GIMP\t2.10.38\tflathub\n" +
	"only\ttwo\n"

func TestParseFlatpak(t *testing.T) {
	programs := parseFlatpak(flatpakFixture)
	if len(programs) != 2 {
		t.Fatalf("got %d programs, want 2 (the two-column line is dropped)", len(programs))
	}
	if programs[0].Name != "org.mozilla.firefox" || programs[0].Publisher != "flathub" {
		t.Errorf("first = %+v", programs[0])
	}
	if programs[0].Source != SourceFlatpak {
		t.Errorf("source = %q", programs[0].Source)
	}
}

const applicationsFixture = `{"SPApplicationsDataType":[
  {"_name":"Safari","version":"17.4","obtained_from":"apple","lastModified":"2026-03-04T10:11:12Z"},
  {"_name":"Docker","obtained_from":"identified_developer","signed_by":["Developer ID Application: Docker Inc","Apple Root CA"]},
  {"_name":"Old Thing","version":"1.0","obtained_from":"unknown"},
  {"version":"9.9","obtained_from":"apple"}
]}`

func TestParseApplicationsJSON(t *testing.T) {
	programs := parseApplicationsJSON([]byte(applicationsFixture))
	if len(programs) != 3 {
		t.Fatalf("got %d programs, want 3 (the nameless one is dropped)", len(programs))
	}
	if programs[0].Name != "Safari" || programs[0].Version != "17.4" || programs[0].Publisher != "Apple" {
		t.Errorf("first = %+v", programs[0])
	}
	if programs[0].InstalledOn != "2026-03-04" {
		t.Errorf("installed = %q", programs[0].InstalledOn)
	}
	if programs[1].Version != "" {
		t.Errorf("an absent version must stay empty, got %q", programs[1].Version)
	}
	if programs[1].Publisher != "Developer ID Application: Docker Inc" {
		t.Errorf("publisher = %q, want the first signer", programs[1].Publisher)
	}
	if programs[2].Publisher != "" {
		t.Errorf("an unknown origin stays empty, got %q", programs[2].Publisher)
	}

	if got := parseApplicationsJSON([]byte("{}")); len(got) != 0 {
		t.Errorf("an empty document yields nothing, got %v", got)
	}
	if got := parseApplicationsJSON([]byte("not json")); len(got) != 0 {
		t.Errorf("unreadable output yields nothing rather than a panic, got %v", got)
	}
}

func TestObtainedFrom(t *testing.T) {
	tests := []struct {
		value, signer, want string
	}{
		{"apple", "", "Apple"},
		{"mac_app_store", "", "Mac App Store"},
		{"identified_developer", "Acme Ltd", "Acme Ltd"},
		{"identified_developer", "", "Identified developer"},
		{"unknown", "", ""},
		{"", "", ""},
		{"something_new", "", "something_new"},
	}
	for _, tt := range tests {
		if got := obtainedFrom(tt.value, tt.signer); got != tt.want {
			t.Errorf("obtainedFrom(%q,%q) = %q, want %q", tt.value, tt.signer, got, tt.want)
		}
	}
}

func TestDedupe(t *testing.T) {
	in := []Program{
		{Name: "7-Zip", Version: "23.01", Source: SourceWindowsAll},
		{Name: "7-Zip", Version: "23.01", Source: SourceWindows32},
		{Name: "7-Zip", Version: "22.00", Source: SourceWindowsAll},
		{Name: "Notepad++", Version: "8.6", Source: SourceWindowsUser},
	}
	out := dedupe(in)
	if len(out) != 3 {
		t.Fatalf("got %d, want 3: the same name at the same version appears once", len(out))
	}
	if out[0].Source != SourceWindowsAll {
		t.Errorf("the first source read must win, got %q", out[0].Source)
	}
	if out[1].Version != "22.00" {
		t.Errorf("the same name at a different version must survive, got %+v", out[1])
	}
}

func TestSourceList(t *testing.T) {
	tests := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"pacman"}, "pacman"},
		{[]string{"pacman", "flatpak"}, "pacman and flatpak"},
		{[]string{"pacman", "flatpak", "dpkg"}, "pacman, flatpak and dpkg"},
		{[]string{"a", "b", "c", "d"}, "a, b, c and d"},
	}
	for _, tt := range tests {
		if got := SourceList(tt.in); got != tt.want {
			t.Errorf("SourceList(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNoteFor(t *testing.T) {
	tests := []struct {
		name    string
		os      string
		sources []string
		want    string
	}{
		{"windows", "windows", []string{SourceWindowsAll}, noteWindows},
		{"linux with a manager", "linux", []string{SourcePacman}, noteLinux},
		{"linux with none", "linux", nil, noteLinuxNone},
		{"darwin", "darwin", []string{SourceApplications}, noteDarwin},
		{"anything else", "freebsd", nil, noteOther},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := noteFor(tt.os, tt.sources); got != tt.want {
				t.Errorf("note\n got %q\nwant %q", got, tt.want)
			}
		})
	}

	// The note is always shown, so every one must be a real sentence.
	for _, note := range []string{noteWindows, noteLinux, noteLinuxNone, noteDarwin, noteOther} {
		if len(note) < 40 || !strings.HasSuffix(note, ".") {
			t.Errorf("note is not a sentence: %q", note)
		}
	}
	if !strings.Contains(noteLinuxNone, "pacman, dpkg, rpm and flatpak") {
		t.Errorf("the no-manager note must name what CHIT looked for: %q", noteLinuxNone)
	}
}

func TestProgramNameNeverBlank(t *testing.T) {
	// Every parser must drop an entry with no name rather than emit a blank row.
	for name, programs := range map[string][]Program{
		"pacman":  parsePacman("Version : 1.0\n\nName : real\nVersion : 2.0\n"),
		"dpkg":    parseDpkg("\t1.0\tx\t10\nreal\t2.0\tx\t10\n"),
		"rpm":     parseRPM("\t1.0\tx\t10\t0\nreal\t2.0\tx\t10\t0\n"),
		"flatpak": parseFlatpak("\t1.0\tx\nreal\t2.0\tx\n"),
		"macos":   parseApplicationsJSON([]byte(`{"SPApplicationsDataType":[{"version":"1.0"},{"_name":"real"}]}`)),
	} {
		for _, p := range programs {
			if strings.TrimSpace(p.Name) == "" {
				t.Errorf("%s emitted a nameless row: %+v", name, p)
			}
		}
		if len(programs) != 1 {
			t.Errorf("%s kept %d rows, want only the named one", name, len(programs))
		}
	}
}

func TestReportSlicesAreNeverNil(t *testing.T) {
	data, err := json.Marshal(Report{Programs: []Program{}, Sources: []string{}, Unsupported: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"programs":[]`, `"sources":[]`, `"unsupported":[]`} {
		if !strings.Contains(string(data), key) {
			t.Errorf("%s missing, something marshalled as null: %s", key, data)
		}
	}
}

func TestListNeverReturnsANilSlice(t *testing.T) {
	r, err := List()
	if err != nil {
		t.Fatalf("List failed on this machine: %v", err)
	}
	if r.Programs == nil || r.Sources == nil || r.Unsupported == nil {
		t.Fatal("all three slices must be initialised so none marshals to null")
	}
	if r.Note == "" {
		t.Fatal("the note is never allowed to be empty: the page always shows it")
	}
	for _, p := range r.Programs {
		if strings.TrimSpace(p.Name) == "" {
			t.Fatalf("a nameless row reached the report: %+v", p)
		}
	}
}

func TestNoWrites(t *testing.T) {
	// This tool reads and nothing else: nothing is installed, updated or removed.
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	banned := []string{
		"os.Remove", "os.WriteFile", "os.Create", "SetValue", "CreateKey", "DeleteKey",
		`"-S"`, `"install"`, `"uninstall"`, `"remove"`, `"-U"`, `"-R"`,
	}
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
