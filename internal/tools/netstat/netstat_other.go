//go:build !windows && !linux && !darwin

package netstat

// collect on an operating system nobody has written a reader for. The list is
// empty and the note says why, so a blank page is never mistaken for a machine
// with nothing listening.
func collect(r *Report) {
	r.markUnsupported(FieldProcess, FieldUDP)
	r.Note = noteOther
}
