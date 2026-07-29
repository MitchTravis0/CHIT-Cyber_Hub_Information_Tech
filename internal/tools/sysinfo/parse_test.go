package sysinfo

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseOSRelease(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		wantName    string
		wantVersion string
	}{
		{
			name: "arch, pretty name wins",
			in: "NAME=\"Arch Linux\"\nPRETTY_NAME=\"Arch Linux\"\nID=arch\nBUILD_ID=rolling\n" +
				"ANSI_COLOR=\"38;2;23;147;209\"\n",
			wantName: "Arch Linux",
		},
		{
			name: "ubuntu, with a version id",
			in: "NAME=\"Ubuntu\"\nVERSION=\"24.04.1 LTS (Noble Numbat)\"\nID=ubuntu\n" +
				"PRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\nVERSION_ID=\"24.04\"\n",
			wantName:    "Ubuntu 24.04.1 LTS",
			wantVersion: "24.04",
		},
		{
			name:        "no pretty name falls back to name plus version",
			in:          "NAME=Fedora\nVERSION_ID=41\n",
			wantName:    "Fedora 41",
			wantVersion: "41",
		},
		{
			name:     "no pretty name and no version",
			in:       "NAME=\"Some Distro\"\n",
			wantName: "Some Distro",
		},
		{
			name:     "unquoted value",
			in:       "PRETTY_NAME=Debian GNU/Linux 12 (bookworm)\n",
			wantName: "Debian GNU/Linux 12 (bookworm)",
		},
		{
			name:     "an equals sign inside a quoted value",
			in:       "PRETTY_NAME=\"Weird=Distro 1.0\"\n",
			wantName: "Weird=Distro 1.0",
		},
		{
			name:     "CRLF line endings",
			in:       "PRETTY_NAME=\"Arch Linux\"\r\nID=arch\r\n",
			wantName: "Arch Linux",
		},
		{name: "empty file"},
		{name: "no key/value lines at all", in: "just some words\nand more\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotVersion := parseOSRelease(tt.in)
			if gotName != tt.wantName {
				t.Errorf("name = %q, want %q", gotName, tt.wantName)
			}
			if gotVersion != tt.wantVersion {
				t.Errorf("version = %q, want %q", gotVersion, tt.wantVersion)
			}
		})
	}
}

func TestParseMeminfo(t *testing.T) {
	// 16311532 * 1024 = 16703008768, computed with python3 before this test was
	// written. The "kB" in /proc/meminfo means 1024 bytes, not 1000, and that
	// is the whole point of this case.
	const wantTotal = 16703008768
	const wantAvailable = 4002480128 // 3908672 * 1024

	tests := []struct {
		name         string
		in           string
		total        int64
		available    int64
		hasAvailable bool
	}{
		{
			name: "a real meminfo",
			in: "MemTotal:       16311532 kB\nMemFree:         1230228 kB\n" +
				"MemAvailable:    3908672 kB\nBuffers:            5124 kB\n",
			total:        wantTotal,
			available:    wantAvailable,
			hasAvailable: true,
		},
		{
			name:         "no MemAvailable line, as on kernels before 3.14",
			in:           "MemTotal:       16311532 kB\nMemFree:         1230228 kB\n",
			total:        wantTotal,
			hasAvailable: false,
		},
		{
			name:         "a garbage line is skipped",
			in:           "MemTotal:       not a number\nMemAvailable:    3908672 kB\n",
			available:    wantAvailable,
			hasAvailable: true,
		},
		{name: "empty file"},
		{name: "a line with a colon but no number", in: "MemTotal:\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			total, available, has := parseMeminfo(tt.in)
			if total != tt.total {
				t.Errorf("total = %d, want %d", total, tt.total)
			}
			if available != tt.available {
				t.Errorf("available = %d, want %d", available, tt.available)
			}
			if has != tt.hasAvailable {
				t.Errorf("hasAvailable = %v, want %v", has, tt.hasAvailable)
			}
		})
	}
}

func TestParseProcUptime(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int64
		ok   bool
	}{
		{"two fields", "1036800.53 8294400.12", 1036800, true},
		{"one field", "2312.45", 2312, true},
		{"whole seconds", "60 0", 60, true},
		{"just booted", "0.01 0.01", 0, true},
		{"trailing newline", "1036800.53 8294400.12\n", 1036800, true},
		{"empty", "", 0, false},
		{"not a number", "hello world", 0, false},
		{"negative", "-5.0 0", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseProcUptime(tt.in)
			if got != tt.want || ok != tt.ok {
				t.Errorf("parseProcUptime(%q) = %d, %v; want %d, %v", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestParseCPUInfo(t *testing.T) {
	twoCores := "processor\t: 0\nvendor_id\t: AuthenticAMD\n" +
		"model name\t: AMD Ryzen AI 7 350 w/ Radeon 860M\n" +
		"processor\t: 1\nmodel name\t: AMD Ryzen AI 7 350 w/ Radeon 860M\n"

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"two cores, reported once", twoCores, "AMD Ryzen AI 7 350 w/ Radeon 860M"},
		{"an ARM board with no model name", "processor\t: 0\nHardware\t: BCM2835\n", ""},
		{"empty", "", ""},
		{"a model name with a colon in it", "model name\t: Intel(R) Core(TM): i7\n", "Intel(R) Core(TM): i7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseCPUInfo(tt.in); got != tt.want {
				t.Errorf("parseCPUInfo = %q, want %q", got, tt.want)
			}
		})
	}
}

// A real /proc/mounts from this machine, plus a bind mount, a second disk and a
// mount path with a space in it.
const mountsSample = `proc /proc proc rw,nosuid,nodev,noexec,relatime 0 0
sys /sys sysfs rw,nosuid,nodev,noexec,relatime 0 0
dev /dev devtmpfs rw,nosuid,relatime,size=15944796k 0 0
run /run tmpfs rw,nosuid,nodev,relatime,mode=755 0 0
efivarfs /sys/firmware/efi/efivars efivarfs rw,nosuid,nodev,noexec,relatime 0 0
devpts /dev/pts devpts rw,nosuid,noexec,relatime,gid=5,mode=620 0 0
/dev/mapper/root / btrfs rw,relatime,compress=zstd:3,ssd,subvol=/@ 0 0
securityfs /sys/kernel/security securityfs rw,nosuid,nodev,noexec,relatime 0 0
tmpfs /dev/shm tmpfs rw,nosuid,nodev,inode64 0 0
cgroup2 /sys/fs/cgroup cgroup2 rw,nosuid,nodev,noexec,relatime 0 0
none /sys/fs/pstore pstore rw,nosuid,nodev,noexec,relatime 0 0
bpf /sys/fs/bpf bpf rw,nosuid,nodev,noexec,relatime,mode=700 0 0
systemd-1 /proc/sys/fs/binfmt_misc autofs rw,relatime,fd=43 0 0
configfs /sys/kernel/config configfs rw,nosuid,nodev,noexec,relatime 0 0
/dev/nvme0n1p1 /boot vfat rw,relatime,fmask=0022,codepage=437 0 0
/dev/mapper/root /home btrfs rw,relatime,compress=zstd:3,subvol=/@home 0 0
/dev/sdb1 /mnt/my\040disk ext4 rw,relatime 0 0
overlay /var/lib/docker/overlay2/x/merged overlay rw,relatime 0 0
tracefs /sys/kernel/tracing tracefs rw,nosuid,nodev,noexec,relatime 0 0
gvfsd-fuse /run/user/1000/gvfs fuse.gvfsd-fuse rw,nosuid,nodev,relatime 0 0
`

func TestParseProcMounts(t *testing.T) {
	got := parseProcMounts(mountsSample)

	want := []Mount{
		{Device: "/dev/mapper/root", Path: "/", FS: "btrfs"},
		{Device: "/dev/nvme0n1p1", Path: "/boot", FS: "vfat"},
		{Device: "/dev/mapper/root", Path: "/home", FS: "btrfs"},
		{Device: "/dev/sdb1", Path: "/mnt/my disk", FS: "ext4"},
	}

	if len(got) != len(want) {
		t.Fatalf("kept %d mounts, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("mount %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseProcMountsKeepsOneDeviceMountedTwice(t *testing.T) {
	got := parseProcMounts(mountsSample)
	roots := 0
	for _, m := range got {
		if m.Device == "/dev/mapper/root" {
			roots++
		}
	}
	if roots != 2 {
		t.Errorf("kept %d mounts of /dev/mapper/root, want 2: both / and /home are real places", roots)
	}
}

func TestParseProcMountsEmpty(t *testing.T) {
	got := parseProcMounts("")
	if got == nil {
		t.Fatal("parseProcMounts returned nil, want an empty slice so it marshals as []")
	}
	if len(got) != 0 {
		t.Errorf("kept %d mounts from an empty file, want 0", len(got))
	}
}

func TestUnescapeMountPath(t *testing.T) {
	tests := []struct{ in, want string }{
		{"/mnt/my\\040disk", "/mnt/my disk"},
		{"/mnt/plain", "/mnt/plain"},
		{"/mnt/tab\\011here", "/mnt/tab\there"},
		{"/mnt/back\\134slash", "/mnt/back\\slash"},
		{"/mnt/trailing\\", "/mnt/trailing\\"},
		{"/mnt/bad\\09", "/mnt/bad\\09"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := unescapeMountPath(tt.in); got != tt.want {
			t.Errorf("unescapeMountPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseSwVers(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantName  string
		wantBuild string
	}{
		{
			name:      "a full answer",
			in:        "ProductName:\tmacOS\nProductVersion:\t15.2\nBuildVersion:\t24C101\n",
			wantName:  "macOS 15.2",
			wantBuild: "24C101",
		},
		{
			name:     "no ProductVersion",
			in:       "ProductName:\tmacOS\n",
			wantName: "macOS",
		},
		{
			name:      "no ProductName",
			in:        "ProductVersion:\t15.2\nBuildVersion:\t24C101\n",
			wantName:  "15.2",
			wantBuild: "24C101",
		},
		{name: "empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotBuild := parseSwVers(tt.in)
			if gotName != tt.wantName {
				t.Errorf("name = %q, want %q", gotName, tt.wantName)
			}
			if gotBuild != tt.wantBuild {
				t.Errorf("build = %q, want %q", gotBuild, tt.wantBuild)
			}
		})
	}
}

func TestParseIoregSerial(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "a real ioreg line",
			in: "    | |   \"IOPlatformUUID\" = \"AAAAAAAA-BBBB\"\n" +
				"    | |   \"IOPlatformSerialNumber\" = \"C02XY1234567\"\n",
			want: "C02XY1234567",
		},
		{name: "no such line", in: "    | |   \"IOPlatformUUID\" = \"AAAA\"\n"},
		{name: "the value is not quoted", in: "\"IOPlatformSerialNumber\" = C02XY1234567\n"},
		{name: "no equals sign", in: "\"IOPlatformSerialNumber\"\n"},
		{name: "empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseIoregSerial(tt.in); got != tt.want {
				t.Errorf("parseIoregSerial = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseKernBoottime(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int64
		ok   bool
	}{
		{"the usual form", "{ sec = 1753612800, usec = 123456 } Sun Jul 27 10:00:00 2026\n", 1753612800, true},
		{"no spaces around the equals", "{ sec=1753612800, usec=0 }", 0, false},
		{"closing brace straight after", "{ sec = 1753612800 }", 1753612800, true},
		{"no sec at all", "{ usec = 0 }", 0, false},
		{"zero", "{ sec = 0, usec = 0 }", 0, false},
		{"empty", "", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseKernBoottime(tt.in)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if ok && got.Unix() != tt.want {
				t.Errorf("sec = %d, want %d", got.Unix(), tt.want)
			}
		})
	}
}

func TestWindowsOSName(t *testing.T) {
	tests := []struct {
		name    string
		product string
		build   int
		want    string
	}{
		// Windows 11 never updated ProductName in the registry, so the build
		// number is the only thing that tells the two apart. 22000 is the first
		// Windows 11 build.
		{"windows 11 at the first build", "Windows 10 Pro", 22000, "Windows 11 Pro"},
		{"windows 10 one build below", "Windows 10 Pro", 21999, "Windows 10 Pro"},
		{"a later windows 11", "Windows 10 Enterprise", 26100, "Windows 11 Enterprise"},
		{"windows 10 home", "Windows 10 Home", 19045, "Windows 10 Home"},
		{"server is left alone", "Windows Server 2022 Standard", 26100, "Windows Server 2022 Standard"},
		{"an unknown product is left alone", "Something Else", 26100, "Something Else"},
		{"empty stays empty", "", 26100, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := windowsOSName(tt.product, tt.build); got != tt.want {
				t.Errorf("windowsOSName(%q, %d) = %q, want %q", tt.product, tt.build, got, tt.want)
			}
		})
	}
}

func TestWindowsVersionString(t *testing.T) {
	tests := []struct {
		major, minor, build, ubr int
		want                     string
	}{
		{10, 0, 26100, 2314, "10.0.26100.2314"},
		{10, 0, 26100, 0, "10.0.26100"},
		{10, 0, 19045, 5011, "10.0.19045.5011"},
		{0, 0, 0, 0, "0.0.0"},
	}
	for _, tt := range tests {
		got := windowsVersionString(tt.major, tt.minor, tt.build, tt.ubr)
		if got != tt.want {
			t.Errorf("windowsVersionString(%d,%d,%d,%d) = %q, want %q",
				tt.major, tt.minor, tt.build, tt.ubr, got, tt.want)
		}
	}
}

func TestDiskFrom(t *testing.T) {
	tests := []struct {
		name              string
		total, free, used int64
		wantPct           float64
		wantFree          int64
	}{
		{"a nearly full disk", 1000, 60, 940, 94, 60},
		{"empty disk", 1000, 1000, 0, 0, 1000},
		{"a disk with no size at all", 0, 0, 0, 0, 0},
		{"rounds to one decimal", 1000, 55, 945, 94.5, 55},
		{"one third", 3, 2, 1, 33.3, 2},
		{"full to the reserve, which df calls 100 percent", 1000, 0, 950, 100, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diskFrom("/", "ext4", tt.total, tt.free, tt.used)
			if got.UsedPct != tt.wantPct {
				t.Errorf("UsedPct = %v, want %v", got.UsedPct, tt.wantPct)
			}
			if got.Free != tt.wantFree {
				t.Errorf("Free = %d, want %d", got.Free, tt.wantFree)
			}
			if got.Total != tt.total || got.Used != tt.used {
				t.Errorf("Total/Used = %d/%d, want %d/%d", got.Total, got.Used, tt.total, tt.used)
			}
			if got.Mount != "/" || got.FS != "ext4" {
				t.Errorf("Mount/FS = %q/%q, want / and ext4", got.Mount, got.FS)
			}
		})
	}
}

func TestMarkUnsupportedIsUniqueAndOrdered(t *testing.T) {
	r := Report{}
	r.markUnsupported(FieldSerial)
	r.markUnsupported(FieldModel, FieldSerial)
	r.markUnsupported(FieldSerial)

	want := []string{FieldSerial, FieldModel}
	if len(r.Unsupported) != len(want) {
		t.Fatalf("Unsupported = %v, want %v", r.Unsupported, want)
	}
	for i := range want {
		if r.Unsupported[i] != want[i] {
			t.Errorf("Unsupported[%d] = %q, want %q", i, r.Unsupported[i], want[i])
		}
	}
}

func TestReportSlicesAreNeverNil(t *testing.T) {
	// A nil Go slice marshals to null, and the page then reads null.length.
	// Checking the JSON rather than len() is what pins it.
	r, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `"disks":[`) {
		t.Errorf("disks marshalled as %s, want an array", text)
	}
	if !strings.Contains(text, `"unsupported":[`) {
		t.Errorf("unsupported marshalled as %s, want an array", text)
	}
	if strings.Contains(text, `"disks":null`) || strings.Contains(text, `"unsupported":null`) {
		t.Error("a slice marshalled as null")
	}
}
