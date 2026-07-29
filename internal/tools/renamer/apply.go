package renamer

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"chit/internal/core"
)

const (
	stateRenamed = "renamed"
	stateFailed  = "failed"
	stateSkipped = "skipped"
)

// Rename is one file that was renamed, recorded so it can be put back.
type Rename struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Batch is the record of one applied rename. The page saves it in the store so
// the next session can still undo it.
type Batch struct {
	Folder string `json:"folder"`
	// AppliedAt is RFC3339 in UTC, set by Apply.
	AppliedAt string   `json:"appliedAt"`
	Renames   []Rename `json:"renames"`
}

// ApplyItem is one file after an apply or an undo.
type ApplyItem struct {
	Old string `json:"old"`
	New string `json:"new"`
	// State is "renamed", "failed" or "skipped".
	State string `json:"state"`
	// Reason is a full sentence. Empty only when State is "renamed".
	Reason string `json:"reason"`
}

// ApplyResult is the outcome of an apply or an undo.
type ApplyResult struct {
	Folder  string      `json:"folder"`
	Renamed int         `json:"renamed"`
	Failed  int         `json:"failed"`
	Skipped int         `json:"skipped"`
	Items   []ApplyItem `json:"items"`
	// Batch is what the page must save so this can be undone. After an Undo it
	// is the zero Batch with an empty Renames slice, and the page clears the
	// saved document instead of saving it.
	Batch Batch  `json:"batch"`
	Note  string `json:"note"`
}

// Apply carries out exactly the plan it is given. It never works out a new name
// of its own: it walks the rows the user was shown, so a rule changed after the
// preview cannot change what happens.
func Apply(plan Plan) (ApplyResult, error) {
	dir, entries, err := openFolder(plan.Folder)
	if err != nil {
		return ApplyResult{}, err
	}

	current, err := fingerprint(entries)
	if err != nil {
		return ApplyResult{}, core.Errorf(core.CodeInternal, messageUnopenable, dir)
	}
	if current != plan.Fingerprint {
		return ApplyResult{}, core.Errorf(core.CodeInvalidInput,
			"The contents of that folder changed since the preview was made, so nothing was renamed. Press Preview again to see the current files.")
	}

	blocked := 0
	for _, row := range plan.Rows {
		if row.Action == actionBlocked {
			blocked++
		}
	}
	if blocked > 0 {
		return ApplyResult{}, core.Errorf(core.CodeInvalidInput,
			"This plan still has %d file(s) that cannot be renamed safely, so nothing was changed. Press Preview again and change the rules until nothing is marked Blocked.",
			blocked)
	}

	// Items and Renames are built with make so neither can marshal to JSON null:
	// the TypeScript mirrors type them as ApplyItem[] and Rename[].
	result := ApplyResult{Folder: dir, Items: make([]ApplyItem, 0, len(plan.Rows))}
	renames := make([]Rename, 0, len(plan.Rows))

	for _, row := range plan.Rows {
		if row.Action != actionRename {
			continue
		}
		if err := os.Rename(filepath.Join(dir, row.Old), filepath.Join(dir, row.New)); err != nil {
			result.Items = append(result.Items, ApplyItem{
				Old:   row.Old,
				New:   row.New,
				State: stateFailed,
				Reason: fmt.Sprintf(
					"%s could not be renamed. It may be open in another program, or you may not have permission to change it.",
					row.Old),
			})
			result.Failed++
			continue
		}
		result.Items = append(result.Items, ApplyItem{Old: row.Old, New: row.New, State: stateRenamed})
		renames = append(renames, Rename{From: row.Old, To: row.New})
		result.Renamed++
	}

	result.Batch = Batch{
		Folder:    dir,
		AppliedAt: time.Now().UTC().Format(time.RFC3339),
		Renames:   renames,
	}
	if result.Failed > 0 {
		result.Note = fmt.Sprintf("%d of %d files were renamed. The ones that failed are listed with the reason.",
			result.Renamed, result.Renamed+result.Failed)
	}
	return result, nil
}

// Undo puts the names in one saved batch back. Anything that has moved on since
// the batch was applied is reported and left alone rather than guessed at.
func Undo(batch Batch) (ApplyResult, error) {
	dir, _, err := openFolder(batch.Folder)
	if err != nil {
		return ApplyResult{}, err
	}

	// Items and Renames are built with make so neither can marshal to JSON null:
	// the TypeScript mirrors type them as ApplyItem[] and Rename[].
	result := ApplyResult{
		Folder: dir,
		Items:  make([]ApplyItem, 0, len(batch.Renames)),
		Batch:  Batch{Renames: make([]Rename, 0)},
	}

	for _, rename := range batch.Renames {
		item := ApplyItem{Old: rename.To, New: rename.From}
		if _, err := os.Stat(filepath.Join(dir, rename.To)); errors.Is(err, fs.ErrNotExist) {
			item.State = stateSkipped
			item.Reason = fmt.Sprintf(
				"%s is not in that folder any more, so it was left alone. It may have been moved, deleted or renamed again.",
				rename.To)
			result.Items, result.Skipped = append(result.Items, item), result.Skipped+1
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, rename.From)); err == nil {
			item.State = stateSkipped
			item.Reason = fmt.Sprintf(
				"There is already something called %s in that folder, so %s was left where it is. Putting the name back would overwrite the other file.",
				rename.From, rename.To)
			result.Items, result.Skipped = append(result.Items, item), result.Skipped+1
			continue
		}
		if err := os.Rename(filepath.Join(dir, rename.To), filepath.Join(dir, rename.From)); err != nil {
			item.State = stateFailed
			item.Reason = fmt.Sprintf("%s could not be renamed back. It may be open in another program.", rename.To)
			result.Items, result.Failed = append(result.Items, item), result.Failed+1
			continue
		}
		item.State = stateRenamed
		result.Items, result.Renamed = append(result.Items, item), result.Renamed+1
	}

	if result.Skipped > 0 || result.Failed > 0 {
		result.Note = fmt.Sprintf("%d of %d names were put back. The rest are listed with the reason.",
			result.Renamed, len(batch.Renames))
	}
	return result, nil
}

// fingerprint is the folder's contents as one hash: every entry's name, whether
// it is a directory, and its size. Modification time is deliberately left out,
// because a file being edited in place does not affect what a rename would do
// and would make Apply refuse for no good reason.
func fingerprint(entries []os.DirEntry) (string, error) {
	h := sha256.New()
	for _, entry := range entries {
		kind, size := "f", int64(0)
		if entry.IsDir() {
			kind = "d"
		} else {
			info, err := entry.Info()
			if err != nil {
				return "", err
			}
			size = info.Size()
		}
		h.Write([]byte(entry.Name()))
		h.Write([]byte{0})
		h.Write([]byte(kind))
		h.Write([]byte{0})
		h.Write([]byte(strconv.FormatInt(size, 10)))
		h.Write([]byte{0x1e})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
