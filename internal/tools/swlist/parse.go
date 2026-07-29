package swlist

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// parseInstallDate reads the YYYYMMDD form Windows writes into InstallDate and
// returns an RFC3339 date, or "" when it is not a real date. A wrong date in an
// audit export is worse than none, so an unparseable value is dropped rather
// than guessed at.
func parseInstallDate(text string) string {
	// No length check: time.Parse with this layout already rejects every string
	// that is not exactly eight digits forming a real date, so a guard here would
	// change no outcome for any input. Proven across the boundary in
	// TestParseInstallDate.
	at, err := time.Parse("20060102", strings.TrimSpace(text))
	if err != nil {
		return ""
	}
	return at.Format("2006-01-02")
}

// skipEntry says whether a registry Uninstall subkey is an update or a component
// of something already listed rather than a program in its own right. Without
// these three rules the list is four hundred numbered Windows updates and the
// software is buried underneath them.
func skipEntry(values map[string]string) bool {
	if values["SystemComponent"] == "1" {
		return true
	}
	if values["ParentKeyName"] != "" || values["ParentDisplayName"] != "" {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(values["ReleaseType"])) {
	case "update", "hotfix", "security update":
		return true
	}
	return false
}

// sizeFromKilobytes converts the KB figure Windows and dpkg both report.
func sizeFromKilobytes(kb int64) int64 {
	if kb <= 0 {
		return 0
	}
	return kb * 1024
}

// pacmanDateLayouts are the shapes pacman's locale-formatted Install Date can
// take. Anything that matches none of them leaves the date empty rather than
// being guessed at.
var pacmanDateLayouts = []string{
	"Mon 02 Jan 2006 03:04:05 PM MST",
	"Mon 02 Jan 2006 15:04:05 MST",
	"Mon Jan  2 15:04:05 2006",
	time.RFC1123,
}

// parsePacman reads "pacman -Qi": blocks of "Key : Value" separated by a blank
// line. One process for a thousand packages, rather than a thousand processes.
func parsePacman(out string) []Program {
	var programs []Program
	block := map[string]string{}
	lastKey := ""

	flush := func() {
		if name := block["Name"]; name != "" {
			programs = append(programs, Program{
				Name:        name,
				Version:     block["Version"],
				Publisher:   packagerName(block["Packager"]),
				InstalledOn: parsePacmanDate(block["Install Date"]),
				SizeBytes:   parsePacmanSize(block["Installed Size"]),
				Source:      SourcePacman,
			})
		}
		block = map[string]string{}
		lastKey = ""
	}

	for _, raw := range strings.Split(out, "\n") {
		if strings.TrimSpace(raw) == "" {
			flush()
			continue
		}
		// A continuation line is indented and belongs to the key above it, so it
		// must not be read as a key of its own.
		if strings.HasPrefix(raw, " ") && !strings.Contains(raw, " : ") {
			if lastKey != "" {
				block[lastKey] += " " + strings.TrimSpace(raw)
			}
			continue
		}
		key, value, found := strings.Cut(raw, ":")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		block[key] = strings.TrimSpace(value)
		lastKey = key
	}
	flush()
	return programs
}

// packagerName drops pacman's placeholder rather than showing it as a publisher.
func packagerName(text string) string {
	text = strings.TrimSpace(text)
	if text == "Unknown Packager" {
		return ""
	}
	// "Name <email@example.com>" reads better without the address.
	if at := strings.Index(text, " <"); at > 0 {
		return text[:at]
	}
	return text
}

func parsePacmanDate(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	for _, layout := range pacmanDateLayouts {
		if at, err := time.Parse(layout, text); err == nil {
			return at.Format("2006-01-02")
		}
	}
	return ""
}

// parsePacmanSize reads "533.16 MiB", "12.34 KiB" or "512.00 B".
func parsePacmanSize(text string) int64 {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) != 2 {
		return 0
	}
	value, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || value <= 0 {
		return 0
	}
	switch strings.ToUpper(fields[1]) {
	case "B":
		return int64(value)
	case "KIB":
		return int64(value * 1024)
	case "MIB":
		return int64(value * 1024 * 1024)
	case "GIB":
		return int64(value * 1024 * 1024 * 1024)
	}
	return 0
}

// parseDpkg reads the tab-separated form dpkg-query is asked for. A short line
// is dropped rather than half read.
func parseDpkg(out string) []Program {
	var programs []Program
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 4 || strings.TrimSpace(fields[0]) == "" {
			continue
		}
		kb, _ := strconv.ParseInt(strings.TrimSpace(fields[3]), 10, 64)
		programs = append(programs, Program{
			Name:      strings.TrimSpace(fields[0]),
			Version:   strings.TrimSpace(fields[1]),
			Publisher: packagerName(fields[2]),
			SizeBytes: sizeFromKilobytes(kb),
			Source:    SourceDpkg,
		})
	}
	return programs
}

// parseRPM reads the tab-separated form rpm is asked for. rpm writes "(none)"
// where a field is empty, which must not reach the screen.
func parseRPM(out string) []Program {
	var programs []Program
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 5 || strings.TrimSpace(fields[0]) == "" {
			continue
		}
		size, _ := strconv.ParseInt(strings.TrimSpace(fields[3]), 10, 64)
		installed := ""
		if unix, err := strconv.ParseInt(strings.TrimSpace(fields[4]), 10, 64); err == nil && unix > 0 {
			installed = time.Unix(unix, 0).UTC().Format("2006-01-02")
		}
		programs = append(programs, Program{
			Name:        strings.TrimSpace(fields[0]),
			Version:     strings.TrimSpace(fields[1]),
			Publisher:   noneToEmpty(fields[2]),
			InstalledOn: installed,
			SizeBytes:   size,
			Source:      SourceRPM,
		})
	}
	return programs
}

// parseFlatpak reads the three columns flatpak list is asked for.
func parseFlatpak(out string) []Program {
	var programs []Program
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 3 || strings.TrimSpace(fields[0]) == "" {
			continue
		}
		programs = append(programs, Program{
			Name:      strings.TrimSpace(fields[0]),
			Version:   strings.TrimSpace(fields[1]),
			Publisher: strings.TrimSpace(fields[2]),
			Source:    SourceFlatpak,
		})
	}
	return programs
}

func noneToEmpty(text string) string {
	text = strings.TrimSpace(text)
	if text == "(none)" {
		return ""
	}
	return text
}

// obtainedFrom turns system_profiler's obtained_from value into something a
// person would write in an asset record.
func obtainedFrom(value, signedBy string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "apple":
		return "Apple"
	case "mac_app_store":
		return "Mac App Store"
	case "identified_developer":
		if signedBy != "" {
			return signedBy
		}
		return "Identified developer"
	case "unknown", "":
		return ""
	}
	return strings.TrimSpace(value)
}

// parseApplicationsJSON walks system_profiler -json SPApplicationsDataType
// defensively: every key may be absent and none may be trusted to be the type it
// usually is.
func parseApplicationsJSON(raw []byte) []Program {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	items, _ := doc["SPApplicationsDataType"].([]any)

	var programs []Program
	for _, node := range items {
		item, _ := node.(map[string]any)
		if item == nil {
			continue
		}
		name := stringAt(item, "_name")
		if name == "" {
			continue
		}
		programs = append(programs, Program{
			Name:        name,
			Version:     stringAt(item, "version"),
			Publisher:   obtainedFrom(stringAt(item, "obtained_from"), firstSigner(item)),
			InstalledOn: dateOf(stringAt(item, "lastModified")),
			Source:      SourceApplications,
		})
	}
	return programs
}

// firstSigner reads the signing chain's first entry, which is the developer.
func firstSigner(item map[string]any) string {
	switch value := item["signed_by"].(type) {
	case string:
		return strings.TrimSpace(value)
	case []any:
		if len(value) > 0 {
			if first, ok := value[0].(string); ok {
				return strings.TrimSpace(first)
			}
		}
	}
	return ""
}

// dateOf keeps the date part of an RFC3339 stamp, or "" when it is not one.
func dateOf(text string) string {
	at, err := time.Parse(time.RFC3339, strings.TrimSpace(text))
	if err != nil {
		return ""
	}
	return at.Format("2006-01-02")
}

func stringAt(item map[string]any, key string) string {
	if value, ok := item[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}
