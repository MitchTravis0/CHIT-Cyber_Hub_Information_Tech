//go:build !windows && !linux && !darwin

package swlist

// collect on an operating system nobody has written a reader for. The list is
// empty and the note says why, so a blank page is never mistaken for a machine
// with nothing installed.
func collect(r *Report) {
	r.markUnsupported(FieldInstalledOn, FieldSize, FieldPublisher)
	r.Note = noteOther
}
