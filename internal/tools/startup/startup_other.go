//go:build !windows && !linux && !darwin

package startup

// collect on an operating system nobody has written a reader for. The list is
// empty and the note says why, so a blank page is never mistaken for a machine
// with nothing set to start.
func collect(r *Report) {
	r.markUnsupported(FieldStartup, FieldServices)
	r.addNote("CHIT does not know how to read startup entries on this operating system.")
}
