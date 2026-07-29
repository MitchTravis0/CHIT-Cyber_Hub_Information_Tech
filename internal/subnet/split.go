package subnet

import (
	"net/netip"

	"chit/internal/core"
)

// MaxSplitSubnets caps how many subnets a split may produce. Listing more than
// this is never what a tech wants and would take the UI down with it.
const MaxSplitSubnets = 4096

// Subnet is one slice of a split network.
type Subnet struct {
	Index          int    `json:"index"`
	CIDR           string `json:"cidr"`
	Network        string `json:"network"`
	Netmask        string `json:"netmask"`
	Broadcast      string `json:"broadcast"`
	FirstHost      string `json:"firstHost"`
	LastHost       string `json:"lastHost"`
	UsableHosts    string `json:"usableHosts"`
	TotalAddresses string `json:"totalAddresses"`
}

// Split carves p into equal subnets of length newBits, for example a /24 into
// /26s. The subnets are returned in address order.
func Split(p netip.Prefix, newBits int) ([]Subnet, error) {
	network := p.Masked()
	maxBits := network.Addr().BitLen()
	if newBits < network.Bits() || newBits > maxBits {
		return nil, core.Errorf(core.CodeInvalidInput,
			"Cannot split %s into /%d. The new prefix must be between /%d and /%d.",
			network, newBits, network.Bits(), maxBits)
	}

	steps := newBits - network.Bits()
	if steps > 12 || 1<<steps > MaxSplitSubnets {
		return nil, core.Errorf(core.CodeInvalidInput,
			"Splitting %s into /%d subnets would list more than %d networks. Pick a shorter new prefix.",
			network, newBits, MaxSplitSubnets)
	}

	count := 1 << steps
	out := make([]Subnet, 0, count)
	addr := network.Addr()
	for i := 0; i < count; i++ {
		sub := netip.PrefixFrom(addr, newBits)
		out = append(out, describeSubnet(i+1, sub))
		addr = LastAddress(sub).Next()
		if !addr.IsValid() {
			break // ran off the end of the address space, e.g. 0.0.0.0/0 split
		}
	}
	return out, nil
}

// SplitInto carves p into at least count equal subnets. Equal subnets only come
// in powers of two, so asking for 3 gives 4.
func SplitInto(p netip.Prefix, count int) ([]Subnet, error) {
	if count < 1 {
		return nil, core.Errorf(core.CodeInvalidInput, "Enter how many subnets you need, for example 4.")
	}
	steps := 0
	for 1<<steps < count {
		steps++
		if steps > 12 {
			return nil, core.Errorf(core.CodeInvalidInput,
				"%d subnets is more than the %d this tool lists. Ask for fewer.", count, MaxSplitSubnets)
		}
	}
	return Split(p, p.Masked().Bits()+steps)
}

func describeSubnet(index int, p netip.Prefix) Subnet {
	s := Subnet{
		Index:          index,
		CIDR:           p.String(),
		Network:        p.Addr().String(),
		FirstHost:      FirstHost(p).String(),
		LastHost:       LastHost(p).String(),
		UsableHosts:    UsableHosts(p).String(),
		TotalAddresses: TotalAddresses(p).String(),
	}
	if p.Addr().Is4() {
		s.Netmask = Netmask(p.Bits()).String()
	}
	if b, ok := Broadcast(p); ok {
		s.Broadcast = b.String()
	}
	return s
}
