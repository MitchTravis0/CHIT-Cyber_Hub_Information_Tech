package main

import (
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"chit/internal/tools/renamer"
)

// PickRenameFolder opens the native folder dialog. A cancelled dialog returns an
// empty string and no error, which the page treats as "nothing chosen" rather
// than a failure.
func (a *App) PickRenameFolder() (string, error) {
	return wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: "Choose the folder to rename files in",
	})
}

// PreviewRename works out what every file in the folder would be called. It
// changes nothing on disk.
func (a *App) PreviewRename(params renamer.Params) (renamer.Plan, error) {
	return renamer.Preview(params)
}

// ApplyRename carries out exactly the plan it is given, which is the plan the
// user was shown. It refuses a plan with a blocked row and a plan whose folder
// has changed since the preview.
func (a *App) ApplyRename(plan renamer.Plan) (renamer.ApplyResult, error) {
	return renamer.Apply(plan)
}

// UndoRename puts the names in one saved batch back. The page reads the batch
// from the store and hands it in.
func (a *App) UndoRename(batch renamer.Batch) (renamer.ApplyResult, error) {
	return renamer.Undo(batch)
}
