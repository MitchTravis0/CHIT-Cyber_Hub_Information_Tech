// Package startup lists what launches when this machine starts or when a user
// signs in, plus the services configured on it. Everything here is read only:
// enabling, disabling or stopping any of it needs administrator rights, which
// CHIT never asks for.
package startup

import (
	"runtime"
	"sort"
	"strconv"
	"strings"

	"chit/internal/core"
)

// Kinds. An entry is one or the other, never both.
const (
	KindStartup = "startup"
	KindService = "service"
)

// StartModes, normalised across the three operating systems so one column makes
// sense everywhere. An empty string means this OS did not say.
const (
	StartAutomatic = "automatic"
	StartManual    = "manual"
	StartDisabled  = "disabled"
	StartBoot      = "boot"
)

// States. An empty string means this OS did not say.
const (
	StateRunning = "running"
	StateStopped = "stopped"
)

// Ids used in Report.Unsupported.
const (
	FieldState     = "state"
	FieldPublisher = "publisher"
	FieldServices  = "services"
	FieldStartup   = "startup"
)

// commandTimeout caps every external command. A timeout is "this operating
// system did not say", never an error.
const commandTimeout = 5 // seconds

// Item is one startup entry or one service.
type Item struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	// Source is where CHIT found it, in words a tech can go and look at.
	Source    string `json:"source"`
	Command   string `json:"command"`
	Publisher string `json:"publisher"`
	StartMode string `json:"startMode"`
	State     string `json:"state"`
	Enabled   bool   `json:"enabled"`
	// Concern is a full sentence when something about this entry is worth a
	// second look, and empty otherwise. It is a hint, never a verdict.
	Concern string `json:"concern"`
}

type Report struct {
	OS          string   `json:"os"`
	Items       []Item   `json:"items"`
	Unsupported []string `json:"unsupported"`
	// Note explains anything the operating system would not hand over.
	Note string `json:"note"`
}

// List reads every startup entry and service this operating system will report.
// A source that fails contributes nothing and adds a sentence to the note; it
// never turns into a returned error, because a partial list is still useful.
func List() (Report, error) {
	r := Report{
		OS: runtime.GOOS,
		// Both slices are initialised so they marshal as [] rather than null.
		Items:       []Item{},
		Unsupported: []string{},
	}

	collect(&r)

	for i := range r.Items {
		// An empty command is only odd where one was actually expected, and
		// both exceptions were found by running this on a real machine:
		//
		//   - A service on Linux and macOS carries no command line, because
		//     systemd and launchd are asked what is configured rather than for
		//     the ExecStart of all 380 units. Judging those on an empty command
		//     flagged 381 of 393 entries, which makes the flag worthless.
		//   - A disabled autostart entry with nothing in it is the normal way
		//     a desktop turns one off: the file is left behind holding only
		//     Hidden=true. Flagging that teaches techs to ignore the flag.
		//
		// Every other rule still applies to a disabled entry.
		if r.Items[i].Command == "" && (r.Items[i].Kind == KindService || !r.Items[i].Enabled) {
			continue
		}
		r.Items[i].Concern = Concern(r.Items[i].Name, r.Items[i].Command)
	}
	sortItems(r.Items)

	if len(r.Items) == 0 && r.Note == "" {
		return r, core.Errorf(core.CodeInternal,
			"Could not read this machine's startup entries. Try Refresh, and if it keeps happening this computer is refusing something CHIT normally gets without admin rights.")
	}
	return r, nil
}

// sortItems puts the list in the order a tech reads it: what starts with the
// computer first, then the services, each group by name with numbers compared
// as numbers so Item 2 comes before Item 10.
func sortItems(items []Item) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind == KindStartup
		}
		return naturalLess(items[i].Name, items[j].Name)
	})
}

// naturalLess compares two names treating runs of digits as numbers.
func naturalLess(a, b string) bool {
	ai, bi := 0, 0
	for ai < len(a) && bi < len(b) {
		if isDigit(a[ai]) && isDigit(b[bi]) {
			aStart, bStart := ai, bi
			for ai < len(a) && isDigit(a[ai]) {
				ai++
			}
			for bi < len(b) && isDigit(b[bi]) {
				bi++
			}
			an := strings.TrimLeft(a[aStart:ai], "0")
			bn := strings.TrimLeft(b[bStart:bi], "0")
			if len(an) != len(bn) {
				return len(an) < len(bn)
			}
			if an != bn {
				return an < bn
			}
			continue
		}
		ca, cb := lower(a[ai]), lower(b[bi])
		if ca != cb {
			return ca < cb
		}
		ai++
		bi++
	}
	return len(a)-ai < len(b)-bi
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func lower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 32
	}
	return c
}

// count writes "1 file" or "3 files", so a note never reads "1 files".
func count(n int, singular, plural string) string {
	word := plural
	if n == 1 {
		word = singular
	}
	return strconv.Itoa(n) + " " + word
}

// markUnsupported records a field this operating system would not report, once.
func (r *Report) markUnsupported(fields ...string) {
	for _, f := range fields {
		found := false
		for _, existing := range r.Unsupported {
			if existing == f {
				found = true
				break
			}
		}
		if !found {
			r.Unsupported = append(r.Unsupported, f)
		}
	}
}

// addNote appends a sentence to the note, so several failing sources each get
// their say instead of the last one winning.
func (r *Report) addNote(sentence string) {
	if r.Note == "" {
		r.Note = sentence
		return
	}
	r.Note += " " + sentence
}
