//go:build !linux && !windows && !darwin

package netinfo

// platformEnrich is the fallback for a platform nobody has written a collector
// for. The standard library still supplies the adapter list, addresses and MAC
// addresses; everything that needs an OS-specific source is reported as
// unavailable rather than guessed, and there is no default route to name.
func platformEnrich(r *Report) string {
	r.markUnsupported(FieldGateway, FieldDNS, FieldAdapterDNS, FieldDHCP)
	return ""
}
