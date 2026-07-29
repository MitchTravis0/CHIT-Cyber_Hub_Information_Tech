// Package renamer works out what every file in one folder would be called under
// a set of rename rules, and then carries out exactly the plan the user was
// shown. It is the service layer the Bulk File Renamer page talks to.
package renamer

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"regexp/syntax"
	"sort"
	"strings"

	"chit/internal/core"
)

const (
	// maxEntries caps one folder. It bounds the ReadDir, the plan that crosses
	// the boundary twice, and the number of rows ResultsTable puts in the DOM.
	maxEntries = 5000

	maxNameLength = 255

	maxStart   = 999999
	maxStep    = 1000
	maxPadding = 10
)

const (
	caseUpper = "upper"
	caseLower = "lower"
	caseTitle = "title"
)

const (
	kindFile   = "file"
	kindFolder = "folder"
	kindOther  = "other"
)

const (
	actionRename    = "rename"
	actionUnchanged = "unchanged"
	actionSkipped   = "skipped"
	actionBlocked   = "blocked"
)

const (
	reasonNotOrdinary   = "Only ordinary files are renamed. This one was left alone."
	reasonUnchanged     = "The rules do not change this name."
	reasonBatchCollide  = "Another file in this folder would get the same new name. Add numbering, or make the find and replace more specific."
	noteNoFiles         = "There are no files in that folder to rename. It may contain only sub-folders, which this tool never touches."
	noteCaseOnly        = "Some names only change in capitals. Windows and macOS treat file names as the same either way, so those renames can look like nothing happened even though the capitals did change."
	messageNoSuchFolder = "There is no folder at %s. Choose it again with the Choose folder button."
	messageUnopenable   = "%s could not be opened. It may be on a drive or network share that is no longer connected."
)

// Params is one set of rename rules plus the folder they apply to. Every zero
// value means "do not do this step", so an empty Params renames nothing.
type Params struct {
	Folder string `json:"folder"`

	// Find is the text or pattern to look for. Empty skips this step entirely.
	Find string `json:"find"`
	// Replace is what to put in its place. May be empty (delete the match).
	Replace string `json:"replace"`
	// UseRegex switches Find from literal text to a Go regular expression.
	UseRegex bool `json:"useRegex"`

	// Case is "", "upper", "lower" or "title".
	Case string `json:"case"`

	Prefix string `json:"prefix"`
	Suffix string `json:"suffix"`

	// Number turns on sequential numbering.
	Number bool `json:"number"`
	// Start is the first number. Step is added for each following file.
	Start int `json:"start"`
	Step  int `json:"step"`
	// Padding is how many digits to pad to with leading zeros. 0 means none.
	Padding int `json:"padding"`

	// KeepExtension leaves the file extension untouched by every step above.
	KeepExtension bool `json:"keepExtension"`
}

// Row is one entry in the folder and what would happen to it.
type Row struct {
	Old string `json:"old"`
	New string `json:"new"`
	// Kind is "file", "folder" or "other".
	Kind string `json:"kind"`
	// Action is "rename", "unchanged", "skipped" or "blocked".
	Action string `json:"action"`
	// Reason is a full sentence for the user. Empty only when Action is "rename".
	Reason string `json:"reason"`
}

// Plan is a full dry run. It is what the preview table shows and it is what
// ApplyRename is handed back, so an apply can only ever do what was on screen.
type Plan struct {
	Folder string `json:"folder"`
	// Fingerprint identifies the folder's contents at the moment the plan was
	// made. Apply recomputes it and refuses the plan if it changed.
	Fingerprint string `json:"fingerprint"`
	Rows        []Row  `json:"rows"`
	Changed     int    `json:"changed"`
	Unchanged   int    `json:"unchanged"`
	Skipped     int    `json:"skipped"`
	Blocked     int    `json:"blocked"`
	Note        string `json:"note"`
}

// Preview works out what every file in the folder would be called. It reads the
// folder and changes nothing on disk.
func Preview(p Params) (Plan, error) {
	return preview(p.Folder, p, maxEntries)
}

// preview takes the entry cap as a parameter so a test can reach the "too many
// items" path without creating five thousand files.
func preview(folder string, p Params, max int) (Plan, error) {
	dir, entries, err := openFolder(folder)
	if err != nil {
		return Plan{}, err
	}
	if len(entries) > max {
		return Plan{}, core.Errorf(core.CodeInvalidInput,
			"That folder has %d items in it. This tool renames up to 5000 at a time, so choose a folder with fewer files in it.",
			len(entries))
	}
	p, re, err := p.rules()
	if err != nil {
		return Plan{}, err
	}

	// The fingerprint is taken in the order os.ReadDir gave, before the natural
	// sort below reorders the slice, so Apply can recompute it from a fresh read.
	print, err := fingerprint(entries)
	if err != nil {
		return Plan{}, core.Errorf(core.CodeInternal, messageUnopenable, dir)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return naturalLess(entries[i].Name(), entries[j].Name())
	})

	// Rows is built with make so it can never marshal to JSON null: the
	// TypeScript mirror types it as Row[], not Row[] | null.
	rows := make([]Row, 0, len(entries))
	onDisk := make(map[string]bool, len(entries))
	index := 0
	for _, entry := range entries {
		name := entry.Name()
		onDisk[strings.ToLower(name)] = true

		kind := kindOf(entry)
		if kind != kindFile {
			rows = append(rows, Row{Old: name, New: name, Kind: kind, Action: actionSkipped, Reason: reasonNotOrdinary})
			continue
		}

		newName := applyRules(name, p, index, re)
		index++

		row := Row{Old: name, New: newName, Kind: kind, Action: actionRename}
		if newName == name {
			row.Action, row.Reason = actionUnchanged, reasonUnchanged
		} else if reason := checkName(newName); reason != "" {
			row.Action, row.Reason = actionBlocked, reason
		}
		rows = append(rows, row)
	}

	blockCollisions(rows)
	blockExisting(rows, onDisk)

	plan := Plan{Folder: dir, Fingerprint: print, Rows: rows}
	files, caseOnly := 0, false
	for _, row := range rows {
		switch row.Action {
		case actionRename:
			plan.Changed++
		case actionUnchanged:
			plan.Unchanged++
		case actionSkipped:
			plan.Skipped++
		case actionBlocked:
			plan.Blocked++
		}
		if row.Kind == kindFile {
			files++
		}
		if row.Action == actionRename && strings.EqualFold(row.Old, row.New) {
			caseOnly = true
		}
	}
	switch {
	case files == 0 && len(rows) > 0:
		plan.Note = noteNoFiles
	case caseOnly:
		plan.Note = noteCaseOnly
	}
	return plan, nil
}

// blockCollisions applies rule 9: two files heading for the same name are both
// blocked, because there is no way to tell which one the user meant to keep.
// The grouping is a lower-cased map key rather than a strings.EqualFold loop so
// a folder at the 5000 cap stays linear.
func blockCollisions(rows []Row) {
	targets := make(map[string]int, len(rows))
	for _, row := range rows {
		if row.Action == actionRename {
			targets[strings.ToLower(row.New)]++
		}
	}
	for i := range rows {
		if rows[i].Action == actionRename && targets[strings.ToLower(rows[i].New)] > 1 {
			rows[i].Action, rows[i].Reason = actionBlocked, reasonBatchCollide
		}
	}
}

// blockExisting applies rule 10. A row's own old name is excluded, which is
// what lets a.txt become A.txt.
func blockExisting(rows []Row, onDisk map[string]bool) {
	for i := range rows {
		row := &rows[i]
		if row.Action != actionRename {
			continue
		}
		if onDisk[strings.ToLower(row.New)] && !strings.EqualFold(row.Old, row.New) {
			row.Action = actionBlocked
			row.Reason = fmt.Sprintf(
				"There is already something called %s in this folder. Renaming would overwrite it, so this file was not renamed. If that file is also being renamed, do the batch in two goes.",
				row.New)
		}
	}
}

func kindOf(entry os.DirEntry) string {
	switch {
	case entry.IsDir():
		return kindFolder
	case entry.Type().IsRegular():
		return kindFile
	default:
		return kindOther
	}
}

// rules validates the rename rules and compiles the pattern once for the whole
// plan. Everything a user can get wrong is caught here, before a single name is
// worked out.
func (p Params) rules() (Params, *regexp.Regexp, error) {
	switch p.Case {
	case "", caseUpper, caseLower, caseTitle:
	default:
		return p, nil, core.Errorf(core.CodeInvalidInput,
			"That is not one of the case options. Choose Leave alone, UPPER CASE, lower case or Title Case.")
	}

	// The numbering limits are checked only when numbering is on, so a value
	// left behind in a hidden field cannot reject an otherwise good preview.
	if p.Number {
		if p.Step == 0 {
			p.Step = 1
		}
		if p.Start < 0 || p.Start > maxStart {
			return p, nil, core.Errorf(core.CodeInvalidInput, "The first number must be between 0 and %d.", maxStart)
		}
		if p.Step < 1 || p.Step > maxStep {
			return p, nil, core.Errorf(core.CodeInvalidInput, "The step must be between 1 and %d.", maxStep)
		}
		if p.Padding < 0 || p.Padding > maxPadding {
			return p, nil, core.Errorf(core.CodeInvalidInput, "Pad to must be between 0 and %d digits.", maxPadding)
		}
	}

	if !p.UseRegex || p.Find == "" {
		return p, nil, nil
	}
	re, err := regexp.Compile(p.Find)
	if err != nil {
		return p, nil, core.Errorf(core.CodeInvalidInput,
			"That search pattern could not be read. The part it did not understand is %q. If you did not mean to use a pattern, untick \"Use a pattern\" and the text will be matched exactly as typed.",
			badFragment(p.Find, err))
	}
	return p, re, nil
}

// badFragment is the piece of the pattern the compiler choked on, which is far
// more use to a tech than the whole pattern. The library's own error text is
// never shown.
func badFragment(pattern string, err error) string {
	var serr *syntax.Error
	if errors.As(err, &serr) && serr.Expr != "" {
		return serr.Expr
	}
	return pattern
}

// openFolder runs every folder check in one place. Preview and Apply both call
// it, because the folder can be deleted or unplugged between the two.
func openFolder(raw string) (string, []os.DirEntry, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil, core.Errorf(core.CodeInvalidInput, "Choose the folder whose files you want to rename.")
	}
	dir := filepath.Clean(trimmed)

	info, err := os.Stat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "", nil, core.Errorf(core.CodeNotFound, messageNoSuchFolder, dir)
	case err != nil:
		return "", nil, core.Errorf(core.CodeInternal, messageUnopenable, dir)
	case !info.IsDir():
		return "", nil, core.Errorf(core.CodeInvalidInput, "%s is a file, not a folder. Choose the folder it is in.", dir)
	}

	entries, err := os.ReadDir(dir)
	switch {
	case errors.Is(err, fs.ErrPermission):
		return "", nil, core.Errorf(core.CodePermission,
			"This computer will not let CHIT list the files in %s. The folder may belong to another user.", dir)
	case err != nil:
		return "", nil, core.Errorf(core.CodeInternal, messageUnopenable, dir)
	}
	return dir, entries, nil
}
