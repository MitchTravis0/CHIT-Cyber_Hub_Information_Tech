//go:build windows

package swlist

import (
	"strconv"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// uninstallPath is where every classic Windows installer registers itself. It is
// readable by any user, which is what makes this the complete half of the tool.
const uninstallPath = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`

// roots are the three places software registers itself. The 64-bit and 32-bit
// views of the machine-wide key genuinely return the same entry on some
// machines, which is why List dedupes afterwards.
var roots = []struct {
	key    registry.Key
	access uint32
	source string
}{
	{registry.LOCAL_MACHINE, registry.READ | registry.WOW64_64KEY, SourceWindowsAll},
	{registry.LOCAL_MACHINE, registry.READ | registry.WOW64_32KEY, SourceWindows32},
	{registry.CURRENT_USER, registry.READ, SourceWindowsUser},
}

// valueNames are every value CHIT reads out of a subkey. Nothing else is
// touched, and nothing is written.
var valueNames = []string{
	"DisplayName", "DisplayVersion", "Publisher", "InstallDate",
	"SystemComponent", "ParentKeyName", "ParentDisplayName", "ReleaseType",
}

func collect(r *Report) {
	ok := false
	for _, root := range roots {
		programs, err := readUninstall(root.key, root.access, root.source)
		if err != nil {
			continue
		}
		ok = true
		if len(programs) > 0 {
			r.addSource(root.source)
			r.Programs = append(r.Programs, programs...)
		}
	}

	if !ok {
		r.Note = noteFor("windows", r.Sources) + failWindows
		return
	}
	r.Note = noteFor("windows", r.Sources)
}

func readUninstall(root registry.Key, access uint32, source string) ([]Program, error) {
	key, err := registry.OpenKey(root, uninstallPath, access)
	if err != nil {
		return nil, err
	}
	defer key.Close()

	names, err := key.ReadSubKeyNames(-1)
	if err != nil {
		return nil, err
	}

	var programs []Program
	for _, name := range names {
		sub, err := registry.OpenKey(key, name, access)
		if err != nil {
			continue
		}
		values := map[string]string{}
		for _, value := range valueNames {
			values[value] = readValue(sub, value)
		}
		size, _, _ := sub.GetIntegerValue("EstimatedSize")
		sub.Close()

		display := strings.TrimSpace(values["DisplayName"])
		if display == "" || skipEntry(values) {
			continue
		}

		programs = append(programs, Program{
			Name:        display,
			Version:     strings.TrimSpace(values["DisplayVersion"]),
			Publisher:   strings.TrimSpace(values["Publisher"]),
			InstalledOn: parseInstallDate(values["InstallDate"]),
			SizeBytes:   sizeFromKilobytes(int64(size)),
			Source:      source,
		})
	}
	return programs, nil
}

// readValue takes a value whichever type it was stored as. Some installers write
// SystemComponent as a DWORD and some as a string, and a tool that only reads one
// of the two would let updates through.
func readValue(key registry.Key, name string) string {
	if text, _, err := key.GetStringValue(name); err == nil {
		return text
	}
	if n, _, err := key.GetIntegerValue(name); err == nil {
		return strconv.FormatUint(n, 10)
	}
	return ""
}
