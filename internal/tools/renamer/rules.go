package renamer

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// numberToken is where the user says the sequence number should go. It is case
// sensitive, so a file genuinely called {N} is left alone.
const numberToken = "{n}"

// splitName splits a name into the part the rules work on and the extension.
// filepath.Ext is deliberately not used: it calls the whole of ".gitignore" an
// extension, which would leave that file with an empty base and destroy it.
func splitName(name string) (string, string) {
	i := strings.LastIndex(name, ".")
	if i <= 0 || i == len(name)-1 {
		return name, ""
	}
	return name[:i], name[i:]
}

// applyRules works out one new name. It is pure: the same name, rules and index
// always give the same answer, and it touches neither the disk nor the clock.
// re is the pattern Params.rules compiled once for the whole plan, and it is
// non-nil exactly when the find step is a pattern rather than literal text.
func applyRules(name string, p Params, index int, re *regexp.Regexp) string {
	base, ext := splitName(name)
	text := name
	if p.KeepExtension {
		text = base
	}

	switch {
	case p.Find == "":
	case re != nil:
		text = re.ReplaceAllString(text, p.Replace)
	default:
		text = strings.ReplaceAll(text, p.Find, p.Replace)
	}

	switch p.Case {
	case caseUpper:
		text = strings.ToUpper(text)
	case caseLower:
		text = strings.ToLower(text)
	case caseTitle:
		text = titleCase(text)
	}

	text = p.Prefix + text
	text = text + p.Suffix

	if p.Number {
		number := numberFor(p.Start+index*p.Step, p.Padding)
		if strings.Contains(text, numberToken) {
			text = strings.ReplaceAll(text, numberToken, number)
		} else {
			text += number
		}
	}

	if p.KeepExtension {
		return text + ext
	}
	return text
}

// titleCase capitalises the first letter of every word. strings.Title is not
// used: it is deprecated, go vet flags it, and its word boundaries are not the
// ones this tool promises.
func titleCase(text string) string {
	out := make([]rune, 0, len(text))
	wordStart := true
	for _, r := range text {
		if wordStart {
			out = append(out, unicode.ToUpper(r))
		} else {
			out = append(out, unicode.ToLower(r))
		}
		wordStart = r == ' ' || r == '-' || r == '_' || r == '.'
	}
	return string(out)
}

// numberFor formats one sequence number. A number wider than the padding is
// never truncated, because a name is worth more than a tidy column.
func numberFor(n, padding int) string {
	if padding > 0 {
		return fmt.Sprintf("%0*d", padding, n)
	}
	return strconv.Itoa(n)
}
