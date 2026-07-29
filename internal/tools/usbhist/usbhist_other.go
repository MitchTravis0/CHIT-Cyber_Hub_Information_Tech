//go:build !windows && !linux && !darwin

package usbhist

// collect on an operating system nobody has written a reader for. The list is
// empty and the note says why, so a blank page is never mistaken for a machine
// with nothing plugged in.
func collect(r *Report) {
	r.History = false
	r.markUnsupported(FieldHistory)
	r.Note = noteOther
}
