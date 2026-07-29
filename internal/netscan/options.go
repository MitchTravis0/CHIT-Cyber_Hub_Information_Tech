package netscan

import (
	"time"

	"chit/internal/core"
)

const (
	DefaultWorkers    = 64
	DefaultTimeout    = time.Second
	DefaultDNSTimeout = 300 * time.Millisecond

	maxWorkers    = 1024
	maxTimeout    = 30 * time.Second
	maxDNSTimeout = 10 * time.Second
)

// DefaultPorts are the TCP ports probed when a host ignores ping: web, file
// sharing, remote access and printing cover most of what sits on an office LAN.
var DefaultPorts = []int{80, 443, 445, 22, 3389, 8080, 631}

// Options drives a Sweep. The zero value of every field means "use the
// default", so a caller only has to set Range.
type Options struct {
	Range      Range
	Workers    int
	Timeout    time.Duration // per host, shared by the ICMP and TCP stages
	DNSTimeout time.Duration // per reverse lookup
	Ports      []int

	SkipICMP      bool
	SkipTCP       bool
	SkipARP       bool
	SkipHostnames bool

	// VendorLookup turns a MAC into a manufacturer name. netscan never fills
	// Host.Vendor on its own: the OUI database belongs to the tool layer.
	VendorLookup func(mac string) string
}

// normalize validates the options and returns a copy with the defaults filled in.
func (o Options) normalize() (Options, error) {
	if o.Range.Count <= 0 || !o.Range.Start.IsValid() || !o.Range.End.IsValid() {
		return o, core.Errorf(core.CodeInvalidInput,
			"No addresses to scan. Enter a range such as 192.168.1.0/24 first.")
	}
	if o.Range.Count > MaxAddresses {
		return o, core.Errorf(core.CodeInvalidInput,
			"That covers %d addresses. Scan at most %d at a time (a /16).", o.Range.Count, MaxAddresses)
	}
	if o.Workers < 0 || o.Workers > maxWorkers {
		return o, core.Errorf(core.CodeInvalidInput,
			"Use between 1 and %d parallel checks. %d is outside that.", maxWorkers, o.Workers)
	}
	if o.Timeout < 0 || o.Timeout > maxTimeout {
		return o, core.Errorf(core.CodeInvalidInput,
			"The wait per device must be between 0 and %s.", maxTimeout)
	}
	if o.DNSTimeout < 0 || o.DNSTimeout > maxDNSTimeout {
		return o, core.Errorf(core.CodeInvalidInput,
			"The wait for a name lookup must be between 0 and %s.", maxDNSTimeout)
	}
	for _, p := range o.Ports {
		if p < 1 || p > 65535 {
			return o, core.Errorf(core.CodeInvalidInput,
				"%d is not a valid port. Ports run from 1 to 65535.", p)
		}
	}
	if o.SkipICMP && o.SkipTCP && o.SkipARP {
		return o, core.Errorf(core.CodeInvalidInput,
			"Ping, TCP checks and the ARP cache are all switched off, so there is nothing left to look for.")
	}

	if o.Workers == 0 {
		o.Workers = DefaultWorkers
	}
	if o.Workers > o.Range.Count {
		o.Workers = o.Range.Count
	}
	if o.Timeout == 0 {
		o.Timeout = DefaultTimeout
	}
	if o.DNSTimeout == 0 {
		o.DNSTimeout = DefaultDNSTimeout
	}
	if len(o.Ports) == 0 {
		o.Ports = DefaultPorts
	}
	return o, nil
}
