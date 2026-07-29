package logview

import "strings"

// Line levels, decided from the text of the line itself.
const (
	LevelError = "error"
	LevelWarn  = "warn"
	LevelInfo  = "info"
	LevelNone  = ""
)

// levelScan is how far into a line Level looks. A stack trace mentioning
// "error" three hundred characters in should not paint the whole file red.
const levelScan = 200

var (
	errorWords = []string{"error", "errors", "err", "fatal", "critical", "crit", "panic", "failed", "failure", "exception", "severe"}
	warnWords  = []string{"warn", "warning", "deprecated"}
	infoWords  = []string{"info", "notice", "debug", "trace", "verbose"}
)

// Level classifies a log line from its text. It matches only on whole words,
// case-insensitively, and only inside the first levelScan characters, because a
// false "error" on every line is worse than no colour at all.
func Level(text string) string {
	head := strings.ToLower(text)
	if len(head) > levelScan {
		head = head[:levelScan]
	}

	switch {
	case containsWord(head, errorWords):
		return LevelError
	case containsWord(head, warnWords):
		return LevelWarn
	case containsWord(head, infoWords):
		return LevelInfo
	}
	return LevelNone
}

func containsWord(haystack string, words []string) bool {
	for _, word := range words {
		if hasWord(haystack, word) {
			return true
		}
	}
	return false
}

// hasWord reports whether word appears in haystack with a non-alphanumeric
// character (or nothing) on each side, so "terror" does not match "error".
func hasWord(haystack, word string) bool {
	from := 0
	for {
		at := strings.Index(haystack[from:], word)
		if at < 0 {
			return false
		}
		at += from
		end := at + len(word)
		if !alnum(haystack, at-1) && !alnum(haystack, end) {
			return true
		}
		from = at + 1
	}
}

func alnum(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return false
	}
	c := s[i]
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}
