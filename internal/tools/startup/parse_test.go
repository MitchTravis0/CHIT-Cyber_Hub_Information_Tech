package startup

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// Captured from a real "systemctl list-unit-files --type=service --no-pager
// --no-legend --plain" on this machine, plus a masked unit and a summary line.
const unitFilesSample = `anydesk.service                              disabled        disabled
archlinux-keyring-wkd-sync.service           static          -
arptables.service                            disabled        disabled
autovt@.service                              alias           -
avahi-daemon.service                         enabled         disabled
systemd-networkd.service                     masked          masked
some-generator.service                       generated       -
tmp.mount                                    static          -

8 unit files listed.
`

func TestParseUnitFiles(t *testing.T) {
	got := parseUnitFiles(unitFilesSample)

	want := []UnitFile{
		{"anydesk", "disabled"},
		{"archlinux-keyring-wkd-sync", "static"},
		{"arptables", "disabled"},
		{"autovt@", "alias"},
		{"avahi-daemon", "enabled"},
		{"systemd-networkd", "masked"},
		{"some-generator", "generated"},
	}

	if len(got) != len(want) {
		t.Fatalf("kept %d units, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("unit %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseUnitFilesDropsNonServices(t *testing.T) {
	for _, unit := range parseUnitFiles(unitFilesSample) {
		if strings.Contains(unit.Name, ".mount") {
			t.Errorf("a mount unit reached the service list: %+v", unit)
		}
	}
}

// Captured from a real "systemctl list-units --type=service --all", plus a
// failed unit, which systemd marks with a bullet.
const unitsSample = `archlinux-keyring-wkd-sync.service   loaded    inactive dead    Refresh existing keys
auditd.service                      loaded    inactive dead    Security Audit Logging Service
auto-cpufreq.service                not-found inactive dead    auto-cpufreq.service
avahi-daemon.service                loaded    active   running Avahi mDNS/DNS-SD Stack
bluetooth.service                   loaded    active   running Bluetooth service
● broken.service                    loaded    failed   failed  A unit that failed
starting.service                    loaded    activating start  A unit coming up
`

func TestParseUnits(t *testing.T) {
	got := parseUnits(unitsSample)

	if len(got) != 7 {
		t.Fatalf("kept %d units, want 7: %+v", len(got), got)
	}

	byName := map[string]Unit{}
	for _, unit := range got {
		byName[unit.Name] = unit
	}

	if u := byName["avahi-daemon"]; u.Load != "loaded" || u.Active != "active" || u.Sub != "running" {
		t.Errorf("avahi-daemon = %+v, want loaded/active/running", u)
	}
	if u := byName["auto-cpufreq"]; u.Load != "not-found" {
		t.Errorf("auto-cpufreq load = %q, want not-found", u.Load)
	}
	// The bullet is a separate field, not part of the name.
	if u, ok := byName["broken"]; !ok || u.Active != "failed" {
		t.Errorf("a failed unit was parsed as %+v, want its name without the bullet", u)
	}
}

func TestUnitStartMode(t *testing.T) {
	tests := []struct{ in, want string }{
		{"enabled", StartAutomatic},
		{"enabled-runtime", StartAutomatic},
		{"alias", StartAutomatic},
		{"disabled", StartDisabled},
		{"masked", StartDisabled},
		{"masked-runtime", StartDisabled},
		{"bad", StartDisabled},
		{"static", StartManual},
		{"indirect", StartManual},
		{"generated", StartManual},
		{"transient", StartManual},
		{"linked", StartManual},
		{"something-new", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := unitStartMode(tt.in); got != tt.want {
			t.Errorf("unitStartMode(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestUnitState(t *testing.T) {
	tests := []struct{ in, want string }{
		{"active", StateRunning},
		{"activating", StateRunning},
		{"reloading", StateRunning},
		{"inactive", StateStopped},
		{"failed", StateStopped},
		{"deactivating", StateStopped},
		{"", ""},
	}
	for _, tt := range tests {
		if got := unitState(tt.in); got != tt.want {
			t.Errorf("unitState(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseDesktopEntry(t *testing.T) {
	// The first block is a real /etc/xdg/autostart file from this machine.
	real := `[Desktop Entry]
Type=Application
Name=AT-SPI D-Bus Bus
Exec=/usr/lib/at-spi-bus-launcher --launch-immediately
OnlyShowIn=GNOME;Unity;
NoDisplay=true
X-GNOME-AutoRestart=true
`

	tests := []struct {
		name     string
		text     string
		file     string
		wantName string
		wantExec string
		wantOn   bool
	}{
		{
			name: "a real autostart file", text: real, file: "at-spi-dbus-bus.desktop",
			wantName: "AT-SPI D-Bus Bus",
			wantExec: "/usr/lib/at-spi-bus-launcher --launch-immediately",
			wantOn:   true,
		},
		{
			name: "hidden true means disabled",
			text: "[Desktop Entry]\nName=Thing\nExec=/bin/thing\nHidden=true\n",
			file: "thing.desktop", wantName: "Thing", wantExec: "/bin/thing", wantOn: false,
		},
		{
			name: "the GNOME autostart flag also disables",
			text: "[Desktop Entry]\nName=Thing\nExec=/bin/thing\nX-GNOME-Autostart-enabled=false\n",
			file: "thing.desktop", wantName: "Thing", wantExec: "/bin/thing", wantOn: false,
		},
		{
			name: "an action section's Name must not win",
			text: "[Desktop Entry]\nName=Real Name\nExec=/bin/real\n\n[Desktop Action new]\nName=New Window\nExec=/bin/other\n",
			file: "x.desktop", wantName: "Real Name", wantExec: "/bin/real", wantOn: true,
		},
		{
			name: "a localised name is ignored",
			text: "[Desktop Entry]\nName[de]=Deutscher Name\nName=English Name\nExec=/bin/x\n",
			file: "x.desktop", wantName: "English Name", wantExec: "/bin/x", wantOn: true,
		},
		{
			name: "no Name falls back to the file name",
			text: "[Desktop Entry]\nExec=/bin/x\n",
			file: "some-program.desktop", wantName: "some-program", wantExec: "/bin/x", wantOn: true,
		},
		{
			name: "CRLF line endings",
			text: "[Desktop Entry]\r\nName=Thing\r\nExec=/bin/thing\r\n",
			file: "thing.desktop", wantName: "Thing", wantExec: "/bin/thing", wantOn: true,
		},
		{
			name: "an empty file still gets a name",
			text: "", file: "empty.desktop", wantName: "empty", wantExec: "", wantOn: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDesktopEntry(tt.text, tt.file)
			if got.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tt.wantName)
			}
			if got.Exec != tt.wantExec {
				t.Errorf("Exec = %q, want %q", got.Exec, tt.wantExec)
			}
			if got.Enabled != tt.wantOn {
				t.Errorf("Enabled = %v, want %v", got.Enabled, tt.wantOn)
			}
		})
	}
}

// The three-column form launchctl list prints. No macOS machine was available
// to capture this from, so it is written from the documented format; the report
// says so.
const launchctlSample = `PID	Status	Label
1234	0	com.apple.Finder
-	0	com.example.notrunning
-	1	com.example.failed
5678	0	com.apple.dock.extra
`

func TestParseLaunchctlList(t *testing.T) {
	got := parseLaunchctlList(launchctlSample)

	if len(got) != 4 {
		t.Fatalf("parsed %d labels, want 4: %v", len(got), got)
	}
	if !got["com.apple.Finder"] {
		t.Error("com.apple.Finder has a PID, so it must be reported as running")
	}
	if got["com.example.notrunning"] {
		t.Error("a dash in the PID column means not running")
	}
	if got["com.example.failed"] {
		t.Error("a dash means not running whatever the exit status was")
	}
	if _, ok := got["Label"]; ok {
		t.Error("the header line was parsed as a job")
	}
}

func TestParsePlist(t *testing.T) {
	tests := []struct {
		name        string
		text        string
		wantOK      bool
		wantLabel   string
		wantProgram string
		wantRun     bool
		wantOff     bool
	}{
		{
			name: "program arguments, first element wins",
			text: `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
	<key>Label</key><string>com.example.job</string>
	<key>ProgramArguments</key>
	<array>
		<string>/usr/local/bin/thing</string>
		<string>--daemon</string>
	</array>
	<key>RunAtLoad</key><true/>
</dict>
</plist>`,
			wantOK: true, wantLabel: "com.example.job",
			wantProgram: "/usr/local/bin/thing", wantRun: true,
		},
		{
			name: "a plain Program key",
			text: `<plist version="1.0"><dict>
	<key>Label</key><string>com.example.two</string>
	<key>Program</key><string>/usr/bin/two</string>
</dict></plist>`,
			wantOK: true, wantLabel: "com.example.two", wantProgram: "/usr/bin/two",
		},
		{
			name: "disabled",
			text: `<plist><dict>
	<key>Label</key><string>com.example.off</string>
	<key>Disabled</key><true/>
	<key>RunAtLoad</key><false/>
</dict></plist>`,
			wantOK: true, wantLabel: "com.example.off", wantOff: true, wantRun: false,
		},
		{
			name:   "a label and nothing else is still readable",
			text:   `<plist><dict><key>Label</key><string>com.example.bare</string></dict></plist>`,
			wantOK: true, wantLabel: "com.example.bare",
		},
		{
			name:   "a dictionary with none of the keys",
			text:   `<plist><dict><key>Other</key><string>value</string></dict></plist>`,
			wantOK: false,
		},
		{
			name:   "a binary plist is refused rather than shown blank",
			text:   "bplist00\xd1\x01\x02_\x10\x05Label",
			wantOK: false,
		},
		{name: "empty", text: "", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parsePlist(tt.text)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (got %+v)", ok, tt.wantOK, got)
			}
			if !ok {
				return
			}
			if got.Label != tt.wantLabel {
				t.Errorf("Label = %q, want %q", got.Label, tt.wantLabel)
			}
			if got.Program != tt.wantProgram {
				t.Errorf("Program = %q, want %q", got.Program, tt.wantProgram)
			}
			if got.RunAtLoad != tt.wantRun {
				t.Errorf("RunAtLoad = %v, want %v", got.RunAtLoad, tt.wantRun)
			}
			if got.Disabled != tt.wantOff {
				t.Errorf("Disabled = %v, want %v", got.Disabled, tt.wantOff)
			}
		})
	}
}

func TestWindowsStartMode(t *testing.T) {
	tests := []struct {
		in   uint64
		want string
	}{
		{0, StartBoot},
		{1, StartBoot},
		{2, StartAutomatic},
		{3, StartManual},
		{4, StartDisabled},
		{99, ""},
	}
	for _, tt := range tests {
		if got := windowsStartMode(tt.in); got != tt.want {
			t.Errorf("windowsStartMode(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestWindowsIsService(t *testing.T) {
	tests := []struct {
		in   uint64
		want bool
	}{
		{0x01, false}, // kernel driver
		{0x02, false}, // file system driver
		{0x08, false}, // recogniser driver
		{0x10, true},  // own process
		{0x20, true},  // shared process
		{0x110, true}, // own process, interactive
		{0x120, true}, // shared process, interactive
	}
	for _, tt := range tests {
		if got := windowsIsService(tt.in); got != tt.want {
			t.Errorf("windowsIsService(%#x) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestWindowsDeviceDesc(t *testing.T) {
	tests := []struct{ in, want string }{
		{"@%SystemRoot%\\system32\\wlansvc.dll,-257;WLAN AutoConfig", "WLAN AutoConfig"},
		{"Plain Name", "Plain Name"},
		{"  Spaced  ", "Spaced"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := windowsDeviceDesc(tt.in); got != tt.want {
			t.Errorf("windowsDeviceDesc(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSortItems(t *testing.T) {
	items := []Item{
		{Name: "Item 10", Kind: KindService},
		{Name: "Zed", Kind: KindStartup},
		{Name: "Item 2", Kind: KindService},
		{Name: "Alpha", Kind: KindStartup},
	}
	sortItems(items)

	got := []string{items[0].Name, items[1].Name, items[2].Name, items[3].Name}
	want := []string{"Alpha", "Zed", "Item 2", "Item 10"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v: startup entries first, then services, numbers as numbers", got, want)
		}
	}
}

func TestNaturalLess(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"Item 2", "Item 10", true},
		{"Item 10", "Item 2", false},
		{"item 2", "Item 10", true},
		{"a", "b", true},
		{"b", "a", false},
		{"a", "a", false},
		{"a", "ab", true},
		{"COM1", "COM9", true},
		{"COM9", "COM10", true},
	}
	for _, tt := range tests {
		if got := naturalLess(tt.a, tt.b); got != tt.want {
			t.Errorf("naturalLess(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestReportSlicesAreNeverNil(t *testing.T) {
	r, _ := List()
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, `"items":null`) {
		t.Error("items marshalled as null, which breaks the page")
	}
	if strings.Contains(text, `"unsupported":null`) {
		t.Error("unsupported marshalled as null, which breaks the page")
	}
}

func TestAddNoteKeepsEverySentence(t *testing.T) {
	r := Report{}
	r.addNote("First thing.")
	r.addNote("Second thing.")
	if r.Note != "First thing. Second thing." {
		t.Errorf("Note = %q, want both sentences: one failing source must not hide another", r.Note)
	}
}

func TestCount(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{{0, "0 files"}, {1, "1 file"}, {2, "2 files"}}
	for _, tt := range tests {
		if got := count(tt.n, "file", "files"); got != tt.want {
			t.Errorf("count(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestNothingInThisPackageWrites(t *testing.T) {
	// The tool's safety promise is that it only reads. A grep is a blunt check
	// and it is the right one: it fails the moment somebody adds a write.
	for _, name := range []string{"startup.go", "concern.go", "parse.go"} {
		data, err := readSource(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"SetValue", "CreateKey", "DeleteKey", "StartService", "ControlService",
			"os.Remove", "os.Rename", "os.WriteFile", "os.Create",
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

// A service with no command line must not be flagged. Linux and macOS report
// what is configured without an ExecStart, so judging those on an empty command
// flagged 381 of 393 entries on a real machine and made the flag worthless.
func TestServicesWithNoCommandAreNotFlagged(t *testing.T) {
	r, err := List()
	if err != nil {
		t.Fatal(err)
	}

	blankFlagged := 0
	for _, item := range r.Items {
		if item.Kind == KindService && item.Command == "" && item.Concern != "" {
			blankFlagged++
		}
	}
	if blankFlagged > 0 {
		t.Errorf("%d services with no command line are flagged, want 0", blankFlagged)
	}

	// The flag has to stay useful: on an ordinary machine almost nothing should
	// trip it. A third of the list being flagged means the rules are wrong.
	flagged := 0
	for _, item := range r.Items {
		if item.Concern != "" {
			flagged++
		}
	}
	if len(r.Items) > 20 && flagged*3 > len(r.Items) {
		t.Errorf("%d of %d entries are flagged, which is too many to be useful", flagged, len(r.Items))
	}
}

// A startup entry with nothing to run is genuinely odd and keeps its sentence.
func TestStartupEntryWithNoCommandIsStillFlagged(t *testing.T) {
	want := "CHIT could not read what this entry actually runs. Look at it in Task Manager or the registry."
	if got := Concern("Mystery", ""); got != want {
		t.Errorf("Concern = %q, want %q", got, want)
	}
}

// A desktop turns an autostart entry off by leaving behind a file holding only
// Hidden=true. That is the normal shape of a disabled entry, not a fault, and
// flagging it teaches techs to ignore the flag.
func TestDisabledEntryWithNoCommandIsNotFlagged(t *testing.T) {
	entry := parseDesktopEntry("[Desktop Entry]\nHidden=true\n", "org.example.Thing.desktop")
	if entry.Enabled {
		t.Fatal("Hidden=true did not disable the entry")
	}
	if entry.Exec != "" {
		t.Fatalf("Exec = %q, want empty", entry.Exec)
	}

	r, err := List()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range r.Items {
		if !item.Enabled && item.Command == "" && item.Concern != "" {
			t.Errorf("a disabled entry with no command is flagged: %s (%s)", item.Name, item.Concern)
		}
	}
}
