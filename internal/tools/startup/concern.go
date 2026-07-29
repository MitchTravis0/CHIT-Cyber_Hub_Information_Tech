package startup

import (
	"path/filepath"
	"strings"
)

// tempFolders are path fragments that mean "this is not where installed
// software lives".
var tempFolders = []string{`\temp\`, `/tmp/`, `\appdata\local\temp\`, `\windows\temp\`}

// scriptExtensions run through an interpreter rather than being a program.
// Legitimate software occasionally does this; unwanted software nearly always
// does.
var scriptExtensions = map[string]bool{
	".vbs": true, ".js": true, ".jse": true, ".vbe": true,
	".wsf": true, ".hta": true, ".scr": true, ".pif": true,
}

var hiddenPowerShell = []string{
	"-enc", "-encodedcommand", "-e ", "frombase64string",
	"iex", "invoke-expression", "-w hidden", "-windowstyle hidden",
}

// Concern returns a sentence explaining why an entry is worth a second look, or
// "" when nothing about it stands out. It is a hint for a human, not a malware
// verdict: CHIT does not check digital signatures, so plenty of perfectly
// legitimate software trips these rules and the help says so.
func Concern(name, command string) string {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return "CHIT could not read what this entry actually runs. Look at it in Task Manager or the registry."
	}

	lower := strings.ToLower(trimmed)

	// Quotes make no difference to a folder-substring test, so they simply go.
	// strings.Trim is wrong here and was: on `"C:\App\run.vbs" /silent` it
	// removes the leading quote and leaves the closing one embedded, so the
	// extension reads as `.vbs"` and the script rule never fires. That is the
	// normal shape of a Windows Run value.
	unquoted := strings.ReplaceAll(lower, `"`, "")
	normalised := strings.ReplaceAll(unquoted, "/", `\`)

	for _, folder := range tempFolders {
		probe := unquoted
		if strings.Contains(folder, `\`) {
			probe = normalised
		}
		if strings.Contains(probe, folder) {
			return "This runs from a temporary folder, which is unusual for something that starts with the computer. Temporary folders are cleared, so a legitimate program does not install itself there."
		}
	}

	if strings.Contains(normalised, `\downloads\`) || strings.Contains(unquoted, "/downloads/") {
		return "This runs from the Downloads folder. Installed software normally lives in Program Files or the user's AppData folder, not in Downloads."
	}

	// executablePart is given the original text, quotes and all, because a
	// quoted path is exactly how it tells the program from its arguments.
	if ext := filepath.Ext(executablePart(lower)); scriptExtensions[ext] {
		return "This starts a " + strings.ToUpper(strings.TrimPrefix(ext, ".")) +
			" script rather than a program. That is uncommon for normal software and is a favourite trick of unwanted software."
	}

	if strings.Contains(lower, "powershell") {
		for _, flag := range hiddenPowerShell {
			if strings.Contains(lower, flag) {
				return "This runs a hidden or scrambled PowerShell command. Legitimate software almost never needs to hide what it runs."
			}
		}
	}

	if strings.Contains(lower, "-nop") || strings.Contains(lower, "-noprofile") {
		if strings.Contains(lower, "http://") || strings.Contains(lower, "https://") {
			return "This downloads and runs something from the internet every time the computer starts."
		}
	}

	if looksGenerated(name) {
		return "The name looks randomly generated rather than chosen, which unwanted software often does so it is hard to search for."
	}

	return ""
}

// executablePart is the program a command runs, so the extension test looks at
// that rather than at a switch that happens to end in .js.
//
// Cutting at the first space would be wrong: "C:\Program Files\App\run.vbs" has
// spaces in it and no arguments at all. Arguments are found by their switch
// marker instead, and a quoted path is taken whole.
func executablePart(command string) string {
	c := strings.TrimSpace(command)
	if strings.HasPrefix(c, `"`) {
		if end := strings.Index(c[1:], `"`); end >= 0 {
			return c[1 : 1+end]
		}
	}
	for _, marker := range []string{" -", " /"} {
		if at := strings.Index(c, marker); at > 0 {
			return strings.TrimSpace(c[:at])
		}
	}
	return c
}

// looksGenerated spots a name that was produced rather than chosen: eight or
// more characters that are all hex digits, or all letters with no vowel in
// them. It has false positives (ntfsdrvsvc is a real-looking name with no
// vowels) and the help says the flag is a hint.
func looksGenerated(name string) bool {
	trimmed := strings.TrimSpace(name)
	if len(trimmed) < 8 {
		return false
	}
	lower := strings.ToLower(trimmed)

	allHex := true
	for i := 0; i < len(lower); i++ {
		c := lower[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			allHex = false
			break
		}
	}
	if allHex {
		return true
	}

	for i := 0; i < len(lower); i++ {
		c := lower[i]
		if c < 'a' || c > 'z' {
			return false
		}
	}
	return !strings.ContainsAny(lower, "aeiou")
}
