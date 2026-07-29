//go:build !windows && !linux && !darwin

package battery

// collect on an operating system nobody has written a reader for. The list is
// empty and the note says why, so a blank page is never mistaken for a machine
// with no battery.
func collect(r *Report) {
	r.markUnsupported(FieldHealth, FieldCycles, FieldSerial)
	r.Note = noteOther
}
