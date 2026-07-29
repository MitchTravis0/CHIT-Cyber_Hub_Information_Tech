//go:build !windows && !linux && !darwin

package wifi

// collect on an operating system nobody has written a reader for. The list is
// empty and the note says why, so a blank page is never mistaken for an adapter
// that is switched off.
func collect(r *Report) {
	r.markUnsupported(FieldSignalDBm, FieldWidth, FieldSecurity, FieldSSID)
	r.Note = noteOther
}
