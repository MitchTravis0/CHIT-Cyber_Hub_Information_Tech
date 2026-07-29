package renamer

import (
	"fmt"
	"strings"
)

// illegalChars are the characters Windows refuses in a file name. They are
// checked on every operating system on purpose: the folder is usually on a
// share a Windows machine also reads, so a name that is legal on ext4 and
// illegal on NTFS would produce a file nobody on the customer's network can
// open.
const illegalChars = `<>:"/\|?*`

// reservedNames are the names Windows keeps for hardware. Only the exact names
// count, so CONSOLE.txt is fine.
var reservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// checkName says why a new name cannot be used, or returns "" when it can. The
// rules are checked in the order of the spec, and the first one that matches is
// the one the user is told about.
func checkName(name string) string {
	if name == "" {
		return "The rules leave this file with no name at all. Add a prefix, a suffix, or some text to keep."
	}
	for _, r := range name {
		if r < 0x20 {
			return "That name would contain an invisible control character, which no operating system will accept."
		}
	}
	if i := strings.IndexAny(name, illegalChars); i >= 0 {
		return fmt.Sprintf("A file name cannot contain the character %q. Windows will not allow it.", name[i:i+1])
	}
	if last := name[len(name)-1]; last == '.' || last == ' ' {
		return "A file name cannot end with a dot or a space on Windows. Windows quietly removes them, which would give the file a name you did not ask for."
	}
	stem := name
	if i := strings.Index(stem, "."); i >= 0 {
		stem = stem[:i]
	}
	// A name that is all extension hides the file on Linux and macOS, so the
	// tech watches a file vanish from the folder rather than get renamed.
	if stem == "" {
		return fmt.Sprintf("The rules leave this file with no name in front of its extension, so it would become a hidden file called %q. Add a prefix, a suffix, or some text to keep.", name)
	}
	if reservedNames[strings.ToUpper(stem)] {
		return fmt.Sprintf("%s is a name Windows reserves for hardware, so no file can be called that. Choose something else.", stem)
	}
	if n := len([]rune(name)); n > maxNameLength {
		return fmt.Sprintf("That name would be %d characters long. Most file systems stop at 255, so this file was not renamed.", n)
	}
	return ""
}
