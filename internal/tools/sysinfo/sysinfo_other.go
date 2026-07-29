//go:build !windows && !linux && !darwin

package sysinfo

// collect on an operating system nobody has written a reader for. Everything
// optional is marked unsupported, so the page says "not available on this OS"
// against every row rather than showing blanks. The hostname, architecture and
// core count still come from the standard library in New.
func collect(r *Report) {
	r.markUnsupported(
		FieldCPUModel, FieldMemoryFree, FieldUptime, FieldBootTime,
		FieldManufacturer, FieldModel, FieldSerial,
	)
}
