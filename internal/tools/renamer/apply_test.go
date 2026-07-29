package renamer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"chit/internal/core"
)

// Every test in this file works inside t.TempDir() and touches nothing else.

func makeFiles(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("could not create the test file %s: %v", name, err)
		}
	}
}

func makeDir(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
		t.Fatalf("could not create the test folder %s: %v", name, err)
	}
}

func onDisk(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("could not list %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

func rowFor(t *testing.T, plan Plan, old string) Row {
	t.Helper()
	for _, row := range plan.Rows {
		if row.Old == old {
			return row
		}
	}
	t.Fatalf("no row for %q in the plan", old)
	return Row{}
}

func itemFor(t *testing.T, result ApplyResult, old string) ApplyItem {
	t.Helper()
	for _, item := range result.Items {
		if item.Old == old {
			return item
		}
	}
	t.Fatalf("no item for %q in the result", old)
	return ApplyItem{}
}

func mustPreview(t *testing.T, p Params) Plan {
	t.Helper()
	plan, err := Preview(p)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	return plan
}

func fingerprintOf(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("could not list %s: %v", dir, err)
	}
	print, err := fingerprint(entries)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	return print
}

func TestPreviewListsAndSorts(t *testing.T) {
	dir := t.TempDir()
	makeFiles(t, dir, "IMG_10.jpg", "b.txt", "IMG_9.jpg", "a.txt", "IMG_2.jpg")

	plan := mustPreview(t, Params{Folder: dir})

	want := []string{"a.txt", "b.txt", "IMG_2.jpg", "IMG_9.jpg", "IMG_10.jpg"}
	got := make([]string, 0, len(plan.Rows))
	for _, row := range plan.Rows {
		got = append(got, row.Old)
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("order = %v, want %v", got, want)
	}
	if plan.Folder != dir {
		t.Errorf("Folder = %q, want %q", plan.Folder, dir)
	}
}

func TestPreviewSkipsDirectories(t *testing.T) {
	dir := t.TempDir()
	makeFiles(t, dir, "a.txt", "c.txt")
	makeDir(t, dir, "b_folder")

	plan := mustPreview(t, Params{Folder: dir, Number: true, Start: 1, Step: 1, KeepExtension: true})

	folder := rowFor(t, plan, "b_folder")
	if folder.Kind != kindFolder {
		t.Errorf("Kind = %q, want %q", folder.Kind, kindFolder)
	}
	if folder.Action != actionSkipped {
		t.Errorf("Action = %q, want %q", folder.Action, actionSkipped)
	}
	if folder.Reason != "Only ordinary files are renamed. This one was left alone." {
		t.Errorf("Reason = %q", folder.Reason)
	}
	if plan.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", plan.Skipped)
	}
	// The folder sits between the two files in the sort order, and it must not
	// take a sequence number with it.
	if got := rowFor(t, plan, "a.txt").New; got != "a1.txt" {
		t.Errorf("a.txt -> %q, want %q", got, "a1.txt")
	}
	if got := rowFor(t, plan, "c.txt").New; got != "c2.txt" {
		t.Errorf("c.txt -> %q, want %q", got, "c2.txt")
	}
}

func TestPreviewUnchanged(t *testing.T) {
	dir := t.TempDir()
	makeFiles(t, dir, "a.txt", "b.txt")

	plan := mustPreview(t, Params{Folder: dir})

	for _, row := range plan.Rows {
		if row.Action != actionUnchanged {
			t.Errorf("%s: Action = %q, want %q", row.Old, row.Action, actionUnchanged)
		}
		if row.Reason != "The rules do not change this name." {
			t.Errorf("%s: Reason = %q", row.Old, row.Reason)
		}
	}
	if plan.Changed != 0 || plan.Blocked != 0 || plan.Unchanged != 2 {
		t.Errorf("Changed = %d, Blocked = %d, Unchanged = %d, want 0, 0, 2", plan.Changed, plan.Blocked, plan.Unchanged)
	}
	if plan.Note != "" {
		t.Errorf("Note = %q, want an empty string", plan.Note)
	}
}

func TestPreviewCollisionInBatch(t *testing.T) {
	dir := t.TempDir()
	makeFiles(t, dir, "a1.txt", "a2.txt", "c9.txt")

	plan := mustPreview(t, Params{Folder: dir, Find: `\d`, UseRegex: true, KeepExtension: true})

	const want = "Another file in this folder would get the same new name. Add numbering, or make the find and replace more specific."
	for _, name := range []string{"a1.txt", "a2.txt"} {
		row := rowFor(t, plan, name)
		if row.Action != actionBlocked {
			t.Errorf("%s: Action = %q, want %q", name, row.Action, actionBlocked)
		}
		if row.Reason != want {
			t.Errorf("%s: Reason = %q, want %q", name, row.Reason, want)
		}
	}
	if row := rowFor(t, plan, "c9.txt"); row.Action != actionRename || row.New != "c.txt" {
		t.Errorf("c9.txt: Action = %q, New = %q, want %q and %q", row.Action, row.New, actionRename, "c.txt")
	}
	if plan.Blocked != 2 || plan.Changed != 1 {
		t.Errorf("Blocked = %d, Changed = %d, want 2 and 1", plan.Blocked, plan.Changed)
	}
}

func TestPreviewCollisionOnDisk(t *testing.T) {
	t.Run("an existing name blocks the row", func(t *testing.T) {
		dir := t.TempDir()
		makeFiles(t, dir, "a.txt", "b.txt")

		plan := mustPreview(t, Params{Folder: dir, Find: "a", Replace: "b", KeepExtension: true})

		const want = "There is already something called b.txt in this folder. Renaming would overwrite it, so this file was not renamed. If that file is also being renamed, do the batch in two goes."
		row := rowFor(t, plan, "a.txt")
		if row.Action != actionBlocked {
			t.Errorf("Action = %q, want %q", row.Action, actionBlocked)
		}
		if row.Reason != want {
			t.Errorf("Reason = %q, want %q", row.Reason, want)
		}
	})

	t.Run("a change of capitals is allowed", func(t *testing.T) {
		dir := t.TempDir()
		makeFiles(t, dir, "a.txt")

		plan := mustPreview(t, Params{Folder: dir, Case: caseUpper, KeepExtension: true})

		row := rowFor(t, plan, "a.txt")
		if row.Action != actionRename || row.New != "A.txt" {
			t.Errorf("Action = %q, New = %q, want %q and %q", row.Action, row.New, actionRename, "A.txt")
		}
		const want = "Some names only change in capitals. Windows and macOS treat file names as the same either way, so those renames can look like nothing happened even though the capitals did change."
		if plan.Note != want {
			t.Errorf("Note = %q, want %q", plan.Note, want)
		}
	})
}

func TestPreviewEmptyFolder(t *testing.T) {
	t.Run("only sub-folders", func(t *testing.T) {
		dir := t.TempDir()
		makeDir(t, dir, "reports")

		plan := mustPreview(t, Params{Folder: dir})

		const want = "There are no files in that folder to rename. It may contain only sub-folders, which this tool never touches."
		if plan.Note != want {
			t.Errorf("Note = %q, want %q", plan.Note, want)
		}
		if len(plan.Rows) != 1 || plan.Skipped != 1 {
			t.Errorf("Rows = %d, Skipped = %d, want 1 and 1", len(plan.Rows), plan.Skipped)
		}
	})

	t.Run("nothing at all", func(t *testing.T) {
		plan := mustPreview(t, Params{Folder: t.TempDir()})

		if len(plan.Rows) != 0 {
			t.Errorf("Rows = %d, want 0", len(plan.Rows))
		}
		if plan.Note != "" {
			t.Errorf("Note = %q, want an empty string", plan.Note)
		}
	})
}

func TestPreviewValidatesFolder(t *testing.T) {
	dir := t.TempDir()
	makeFiles(t, dir, "a.txt")
	missing := filepath.Join(dir, "not-here")
	file := filepath.Join(dir, "a.txt")

	tests := []struct {
		name   string
		folder string
		code   string
		want   string
	}{
		{
			name:   "empty",
			folder: "",
			code:   core.CodeInvalidInput,
			want:   "Choose the folder whose files you want to rename.",
		},
		{
			name:   "only spaces",
			folder: "   ",
			code:   core.CodeInvalidInput,
			want:   "Choose the folder whose files you want to rename.",
		},
		{
			name:   "does not exist",
			folder: missing,
			code:   core.CodeNotFound,
			want:   fmt.Sprintf("There is no folder at %s. Choose it again with the Choose folder button.", missing),
		},
		{
			name:   "a file, not a folder",
			folder: file,
			code:   core.CodeInvalidInput,
			want:   fmt.Sprintf("%s is a file, not a folder. Choose the folder it is in.", file),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Preview(Params{Folder: tt.folder})
			if err == nil {
				t.Fatal("Preview accepted a folder it should have rejected")
			}
			if code := core.CodeOf(err); code != tt.code {
				t.Errorf("code = %q, want %q", code, tt.code)
			}
			if got := core.MessageOf(err); got != tt.want {
				t.Errorf("message = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPreviewRefusesHugeFolder(t *testing.T) {
	dir := t.TempDir()
	makeFiles(t, dir, "a.txt", "b.txt", "c.txt", "d.txt")

	_, err := preview(dir, Params{Folder: dir}, 3)
	if err == nil {
		t.Fatal("preview accepted a folder over the cap")
	}
	if code := core.CodeOf(err); code != core.CodeInvalidInput {
		t.Errorf("code = %q, want %q", code, core.CodeInvalidInput)
	}
	const want = "That folder has 4 items in it. This tool renames up to 5000 at a time, so choose a folder with fewer files in it."
	if got := core.MessageOf(err); got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func TestFingerprintIsStable(t *testing.T) {
	dir := t.TempDir()
	makeFiles(t, dir, "a.txt", "b.txt", "c.txt")
	first := fingerprintOf(t, dir)

	t.Run("the same folder read twice", func(t *testing.T) {
		if second := fingerprintOf(t, dir); second != first {
			t.Errorf("fingerprint changed on a second read: %q then %q", first, second)
		}
	})

	t.Run("rewriting a file to the same length does not change it", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("y"), 0o644); err != nil {
			t.Fatalf("could not rewrite a.txt: %v", err)
		}
		if got := fingerprintOf(t, dir); got != first {
			t.Error("rewriting a file to the same length changed the fingerprint")
		}
	})

	t.Run("renaming a file changes it", func(t *testing.T) {
		if err := os.Rename(filepath.Join(dir, "c.txt"), filepath.Join(dir, "d.txt")); err != nil {
			t.Fatalf("could not rename c.txt: %v", err)
		}
		if got := fingerprintOf(t, dir); got == first {
			t.Error("renaming a file left the fingerprint unchanged")
		}
	})

	t.Run("adding a file changes it", func(t *testing.T) {
		before := fingerprintOf(t, dir)
		makeFiles(t, dir, "e.txt")
		if got := fingerprintOf(t, dir); got == before {
			t.Error("adding a file left the fingerprint unchanged")
		}
	})

	// A folder with one file fewer is what a deletion leaves behind. Comparing
	// two folders proves the same thing without this package ever deleting
	// anything, which it has no code to do.
	t.Run("one file fewer is a different fingerprint", func(t *testing.T) {
		full, short := t.TempDir(), t.TempDir()
		makeFiles(t, full, "a.txt", "b.txt")
		makeFiles(t, short, "a.txt")
		if fingerprintOf(t, full) == fingerprintOf(t, short) {
			t.Error("a folder with one file fewer has the same fingerprint")
		}
	})

	t.Run("a folder and a file of the same name differ", func(t *testing.T) {
		asFile, asFolder := t.TempDir(), t.TempDir()
		makeFiles(t, asFile, "thing")
		makeDir(t, asFolder, "thing")
		if fingerprintOf(t, asFile) == fingerprintOf(t, asFolder) {
			t.Error("a file and a folder of the same name have the same fingerprint")
		}
	})
}

func TestApplyRenamesFiles(t *testing.T) {
	dir := t.TempDir()
	makeFiles(t, dir, "one.txt", "two.txt", "three.txt")

	plan := mustPreview(t, Params{Folder: dir, Prefix: "acme-", KeepExtension: true})
	result, err := Apply(plan)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if result.Renamed != 3 || result.Failed != 0 {
		t.Errorf("Renamed = %d, Failed = %d, want 3 and 0", result.Renamed, result.Failed)
	}
	want := []string{"acme-one.txt", "acme-three.txt", "acme-two.txt"}
	if got := onDisk(t, dir); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("on disk = %v, want %v", got, want)
	}
	if len(result.Batch.Renames) != 3 {
		t.Fatalf("Batch.Renames has %d entries, want 3", len(result.Batch.Renames))
	}
	for _, rename := range result.Batch.Renames {
		if rename.To != "acme-"+rename.From {
			t.Errorf("batch pair %q -> %q does not match the plan", rename.From, rename.To)
		}
	}
	if result.Batch.Folder != dir {
		t.Errorf("Batch.Folder = %q, want %q", result.Batch.Folder, dir)
	}
	if result.Batch.AppliedAt == "" {
		t.Error("Batch.AppliedAt is empty")
	}
	if result.Note != "" {
		t.Errorf("Note = %q, want an empty string", result.Note)
	}
}

// TestApplyIgnoresRulesAndUsesThePlan is the test that proves Apply never works
// out a name of its own: the row it is handed says something the rules could
// never produce, and that is what lands on disk.
func TestApplyIgnoresRulesAndUsesThePlan(t *testing.T) {
	dir := t.TempDir()
	makeFiles(t, dir, "one.txt", "two.txt")

	plan := mustPreview(t, Params{Folder: dir, Prefix: "acme-", KeepExtension: true})
	for i := range plan.Rows {
		if plan.Rows[i].Old == "one.txt" {
			plan.Rows[i].New = "hand-edited.txt"
		}
	}

	if _, err := Apply(plan); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	want := []string{"acme-two.txt", "hand-edited.txt"}
	if got := onDisk(t, dir); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("on disk = %v, want %v", got, want)
	}
}

func TestApplyRefusesStalePlan(t *testing.T) {
	dir := t.TempDir()
	makeFiles(t, dir, "one.txt", "two.txt")

	plan := mustPreview(t, Params{Folder: dir, Prefix: "acme-", KeepExtension: true})
	makeFiles(t, dir, "three.txt")

	_, err := Apply(plan)
	if err == nil {
		t.Fatal("Apply accepted a plan made before the folder changed")
	}
	if code := core.CodeOf(err); code != core.CodeInvalidInput {
		t.Errorf("code = %q, want %q", code, core.CodeInvalidInput)
	}
	const want = "The contents of that folder changed since the preview was made, so nothing was renamed. Press Preview again to see the current files."
	if got := core.MessageOf(err); got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
	names := []string{"one.txt", "three.txt", "two.txt"}
	if got := onDisk(t, dir); fmt.Sprint(got) != fmt.Sprint(names) {
		t.Errorf("on disk = %v, want %v, so something was renamed", got, names)
	}
}

func TestApplyRefusesBlockedPlan(t *testing.T) {
	dir := t.TempDir()
	makeFiles(t, dir, "a1.txt", "a2.txt")

	plan := mustPreview(t, Params{Folder: dir, Find: `\d`, UseRegex: true, KeepExtension: true})
	if plan.Blocked != 2 {
		t.Fatalf("Blocked = %d, want 2", plan.Blocked)
	}

	_, err := Apply(plan)
	if err == nil {
		t.Fatal("Apply accepted a plan with blocked rows")
	}
	if code := core.CodeOf(err); code != core.CodeInvalidInput {
		t.Errorf("code = %q, want %q", code, core.CodeInvalidInput)
	}
	const want = "This plan still has 2 file(s) that cannot be renamed safely, so nothing was changed. Press Preview again and change the rules until nothing is marked Blocked."
	if got := core.MessageOf(err); got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
	names := []string{"a1.txt", "a2.txt"}
	if got := onDisk(t, dir); fmt.Sprint(got) != fmt.Sprint(names) {
		t.Errorf("on disk = %v, want %v, so something was renamed", got, names)
	}
}

func TestApplyValidatesFolder(t *testing.T) {
	dir := t.TempDir()
	makeFiles(t, dir, "one.txt")

	plan := mustPreview(t, Params{Folder: dir, Prefix: "acme-", KeepExtension: true})
	plan.Folder = filepath.Join(dir, "moved-away")

	_, err := Apply(plan)
	if err == nil {
		t.Fatal("Apply accepted a plan whose folder is not there")
	}
	if code := core.CodeOf(err); code != core.CodeNotFound {
		t.Errorf("code = %q, want %q", code, core.CodeNotFound)
	}
	want := fmt.Sprintf("There is no folder at %s. Choose it again with the Choose folder button.", plan.Folder)
	if got := core.MessageOf(err); got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func TestUndoRestoresNames(t *testing.T) {
	dir := t.TempDir()
	makeFiles(t, dir, "one.txt", "two.txt", "three.txt")

	applied, err := Apply(mustPreview(t, Params{Folder: dir, Prefix: "acme-", KeepExtension: true}))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	undone, err := Undo(applied.Batch)
	if err != nil {
		t.Fatalf("Undo: %v", err)
	}
	if undone.Renamed != 3 || undone.Skipped != 0 || undone.Failed != 0 {
		t.Errorf("Renamed = %d, Skipped = %d, Failed = %d, want 3, 0, 0", undone.Renamed, undone.Skipped, undone.Failed)
	}
	want := []string{"one.txt", "three.txt", "two.txt"}
	if got := onDisk(t, dir); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("on disk = %v, want %v", got, want)
	}
	if len(undone.Batch.Renames) != 0 {
		t.Errorf("Batch.Renames has %d entries, want none", len(undone.Batch.Renames))
	}
	if undone.Note != "" {
		t.Errorf("Note = %q, want an empty string", undone.Note)
	}
	item := itemFor(t, undone, "acme-one.txt")
	if item.New != "one.txt" || item.State != stateRenamed {
		t.Errorf("item = %+v, want one.txt and %q", item, stateRenamed)
	}
}

func TestUndoSkipsMissingFile(t *testing.T) {
	dir := t.TempDir()
	makeFiles(t, dir, "one.txt", "two.txt")

	applied, err := Apply(mustPreview(t, Params{Folder: dir, Prefix: "acme-", KeepExtension: true}))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// Somebody renamed one of them again behind the tool's back.
	if err := os.Rename(filepath.Join(dir, "acme-one.txt"), filepath.Join(dir, "elsewhere.txt")); err != nil {
		t.Fatalf("could not move acme-one.txt: %v", err)
	}

	undone, err := Undo(applied.Batch)
	if err != nil {
		t.Fatalf("Undo: %v", err)
	}
	item := itemFor(t, undone, "acme-one.txt")
	const want = "acme-one.txt is not in that folder any more, so it was left alone. It may have been moved, deleted or renamed again."
	if item.State != stateSkipped || item.Reason != want {
		t.Errorf("item = %+v, want state %q and reason %q", item, stateSkipped, want)
	}
	if undone.Renamed != 1 || undone.Skipped != 1 {
		t.Errorf("Renamed = %d, Skipped = %d, want 1 and 1", undone.Renamed, undone.Skipped)
	}
	const note = "1 of 2 names were put back. The rest are listed with the reason."
	if undone.Note != note {
		t.Errorf("Note = %q, want %q", undone.Note, note)
	}
	names := []string{"elsewhere.txt", "two.txt"}
	if got := onDisk(t, dir); fmt.Sprint(got) != fmt.Sprint(names) {
		t.Errorf("on disk = %v, want %v", got, names)
	}
}

func TestUndoSkipsWhenOldNameTaken(t *testing.T) {
	dir := t.TempDir()
	makeFiles(t, dir, "one.txt", "two.txt")

	applied, err := Apply(mustPreview(t, Params{Folder: dir, Prefix: "acme-", KeepExtension: true}))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	makeFiles(t, dir, "one.txt")

	undone, err := Undo(applied.Batch)
	if err != nil {
		t.Fatalf("Undo: %v", err)
	}
	item := itemFor(t, undone, "acme-one.txt")
	const want = "There is already something called one.txt in that folder, so acme-one.txt was left where it is. Putting the name back would overwrite the other file."
	if item.State != stateSkipped || item.Reason != want {
		t.Errorf("item = %+v, want state %q and reason %q", item, stateSkipped, want)
	}
	names := []string{"acme-one.txt", "one.txt", "two.txt"}
	if got := onDisk(t, dir); fmt.Sprint(got) != fmt.Sprint(names) {
		t.Errorf("on disk = %v, want %v", got, names)
	}
}

func TestUndoValidatesFolder(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone")

	_, err := Undo(Batch{Folder: missing, Renames: []Rename{{From: "a.txt", To: "b.txt"}}})
	if err == nil {
		t.Fatal("Undo accepted a batch whose folder is not there")
	}
	if code := core.CodeOf(err); code != core.CodeNotFound {
		t.Errorf("code = %q, want %q", code, core.CodeNotFound)
	}
	want := fmt.Sprintf("There is no folder at %s. Choose it again with the Choose folder button.", missing)
	if got := core.MessageOf(err); got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}
