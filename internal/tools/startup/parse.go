package startup

import (
	"encoding/xml"
	"path/filepath"
	"strings"
)

// UnitFile is one line of "systemctl list-unit-files".
type UnitFile struct {
	Name  string
	State string // enabled, disabled, static, masked, alias, ...
}

// Unit is one line of "systemctl list-units --all".
type Unit struct {
	Name   string
	Load   string // loaded, not-found, masked
	Active string // active, inactive, failed, activating
	Sub    string // running, dead, exited, ...
}

// parseUnitFiles reads the configured list. systemd prints a summary line and
// sometimes a legend; both are dropped by requiring a name ending in .service.
func parseUnitFiles(out string) []UnitFile {
	items := []UnitFile{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(strings.TrimPrefix(strings.TrimSpace(line), "●"))
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimSpace(fields[0])
		if !strings.HasSuffix(name, ".service") {
			continue
		}
		items = append(items, UnitFile{Name: strings.TrimSuffix(name, ".service"), State: fields[1]})
	}
	return items
}

// parseUnits reads the loaded list, which is where the running state lives.
func parseUnits(out string) []Unit {
	items := []Unit{}
	for _, line := range strings.Split(out, "\n") {
		// systemd puts a bullet in front of a failed unit, and it is a separate
		// field rather than part of the name.
		fields := strings.Fields(strings.TrimPrefix(strings.TrimSpace(line), "●"))
		if len(fields) < 4 {
			continue
		}
		name := strings.TrimSpace(fields[0])
		if !strings.HasSuffix(name, ".service") {
			continue
		}
		items = append(items, Unit{
			Name:   strings.TrimSuffix(name, ".service"),
			Load:   fields[1],
			Active: fields[2],
			Sub:    fields[3],
		})
	}
	return items
}

// unitStartMode normalises systemd's enablement states onto the shared column.
func unitStartMode(state string) string {
	switch state {
	case "enabled", "enabled-runtime", "alias":
		return StartAutomatic
	case "disabled", "masked", "masked-runtime", "bad":
		return StartDisabled
	case "static", "indirect", "generated", "transient", "linked", "linked-runtime":
		return StartManual
	}
	return ""
}

// unitState normalises systemd's active states onto the shared column.
func unitState(active string) string {
	switch active {
	case "active", "activating", "reloading":
		return StateRunning
	case "":
		return ""
	}
	return StateStopped
}

// DesktopEntry is what CHIT needs out of an XDG autostart file.
type DesktopEntry struct {
	Name    string
	Exec    string
	Enabled bool
}

// parseDesktopEntry reads an XDG .desktop file. Only the [Desktop Entry]
// section counts: a [Desktop Action] section has its own Name= that must not
// win, and a localised Name[de]= is not this machine's name.
func parseDesktopEntry(text, fallbackName string) DesktopEntry {
	entry := DesktopEntry{Enabled: true}
	inMain := false

	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if strings.HasPrefix(line, "[") {
			inMain = line == "[Desktop Entry]"
			continue
		}
		if !inMain {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "Name":
			entry.Name = value
		case "Exec":
			entry.Exec = value
		case "Hidden":
			if strings.EqualFold(value, "true") {
				entry.Enabled = false
			}
		case "X-GNOME-Autostart-enabled":
			if strings.EqualFold(value, "false") {
				entry.Enabled = false
			}
		}
	}

	if entry.Name == "" {
		entry.Name = strings.TrimSuffix(fallbackName, filepath.Ext(fallbackName))
	}
	return entry
}

// parseLaunchctlList reads "launchctl list", whose columns are PID, last exit
// status, and label. A dash in the PID column means the job is not running.
func parseLaunchctlList(out string) map[string]bool {
	running := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if fields[0] == "PID" {
			continue
		}
		running[fields[2]] = fields[0] != "-"
	}
	return running
}

// Plist is what CHIT needs out of a launchd job file.
type Plist struct {
	Label     string
	Program   string
	RunAtLoad bool
	Disabled  bool
}

// parsePlist reads an XML launchd job. Apple's format is a flat dictionary of
// alternating <key> and value elements, so this walks the token stream and
// takes the four keys the tool shows.
//
// It is not a general plist parser and does not pretend to be: a binary plist,
// which some launchd jobs are, returns ok=false so the caller can count it and
// say so rather than showing a blank row.
func parsePlist(text string) (Plist, bool) {
	dec := xml.NewDecoder(strings.NewReader(text))
	// Some plists reference Apple's DTD, which is not fetched, and a few carry
	// entities Go's strict mode rejects. Neither affects the keys read here.
	dec.Strict = false
	dec.Entity = xml.HTMLEntity

	out := Plist{}
	key := ""
	expecting := false
	arrayFor := ""
	dictDepth := 0
	var chars strings.Builder

	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			chars.Reset()
			switch t.Name.Local {
			case "dict":
				dictDepth++
			case "array":
				if expecting {
					arrayFor, expecting = key, false
				}
			}
		case xml.CharData:
			chars.Write(t)
		case xml.EndElement:
			value := strings.TrimSpace(chars.String())
			chars.Reset()
			// Only the outermost dictionary is read, so a nested one cannot
			// overwrite the job's own Label.
			if dictDepth != 1 && t.Name.Local != "dict" {
				continue
			}
			switch t.Name.Local {
			case "dict":
				dictDepth--
			case "array":
				arrayFor = ""
			case "key":
				key, expecting = value, true
			case "string":
				switch {
				case expecting && key == "Label":
					out.Label = value
				case expecting && key == "Program":
					out.Program = value
				case arrayFor == "ProgramArguments" && out.Program == "":
					out.Program = value
				}
				expecting = false
			case "true", "false":
				if expecting {
					switch key {
					case "RunAtLoad":
						out.RunAtLoad = t.Name.Local == "true"
					case "Disabled":
						out.Disabled = t.Name.Local == "true"
					}
					expecting = false
				}
			}
		}
	}

	if out.Label == "" && out.Program == "" {
		return out, false
	}
	return out, true
}

// windowsStartMode maps the Start value of a service registry key.
func windowsStartMode(start uint64) string {
	switch start {
	case 0, 1:
		return StartBoot
	case 2:
		return StartAutomatic
	case 3:
		return StartManual
	case 4:
		return StartDisabled
	}
	return ""
}

// windowsIsService tells a service from a driver by its Type value. A tech
// asking about services does not want three hundred driver rows.
func windowsIsService(serviceType uint64) bool {
	switch serviceType {
	case 0x10, 0x20, 0x110, 0x120:
		return true
	}
	return false
}

// windowsDeviceDesc pulls the readable half out of a registry description,
// which Windows writes as "@file.inf,%resource%;Human readable name".
func windowsDeviceDesc(value string) string {
	if at := strings.LastIndex(value, ";"); at >= 0 {
		return strings.TrimSpace(value[at+1:])
	}
	return strings.TrimSpace(value)
}
