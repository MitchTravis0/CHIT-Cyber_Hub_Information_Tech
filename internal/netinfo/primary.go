package netinfo

import (
	"net"
	"strconv"

	"chit/internal/core"
)

// Subnet describes the network the machine itself sits on. The IP Range
// Scanner uses it for its "scan my subnet" shortcut.
type Subnet struct {
	Adapter string `json:"adapter"`
	IP      string `json:"ip"`
	// CIDR is the network, not the host address, e.g. 192.168.1.0/24.
	CIDR    string `json:"cidr"`
	Mask    string `json:"mask"`
	Gateway string `json:"gateway"`
	// First and Last are the usable host addresses, empty for a /31 or /32.
	First string `json:"first"`
	Last  string `json:"last"`
}

// PrimarySubnet returns the network of the adapter this machine would use to
// reach the rest of the world.
func PrimarySubnet() (Subnet, error) {
	r, err := List()
	if err != nil {
		return Subnet{}, err
	}
	for _, a := range r.Adapters {
		if !a.Primary {
			continue
		}
		for _, v4 := range a.IPv4 {
			ip, ipnet, err := net.ParseCIDR(v4.CIDR)
			if err != nil || !usableIPv4(ip) {
				continue
			}
			ones, _ := ipnet.Mask.Size()
			first, last := hostRange(ip, ipnet.Mask)
			return Subnet{
				Adapter: a.Name,
				IP:      v4.IP,
				CIDR:    ipnet.IP.String() + "/" + strconv.Itoa(ones),
				Mask:    v4.Mask,
				Gateway: a.Gateway,
				First:   first,
				Last:    last,
			}, nil
		}
	}
	return Subnet{}, core.Errorf(core.CodeNotFound,
		"This machine is not connected to a network, so there is no subnet to scan. Connect it and try again.")
}

// selectPrimary picks the adapter carrying the machine's traffic and returns
// its index, or -1 when nothing is connected. The OS's own default route wins
// outright; the score only breaks ties when the OS did not tell us.
func selectPrimary(adapters []Adapter, defaultIface string) int {
	best, bestScore := -1, 0
	for i, a := range adapters {
		if score := primaryScore(a, defaultIface); score > bestScore {
			best, bestScore = i, score
		}
	}
	return best
}

func primaryScore(a Adapter, defaultIface string) int {
	if !a.Up || a.Loopback || !hasUsableIPv4(a) {
		return 0
	}
	score := 1
	if !a.Virtual {
		score += 2
	}
	if a.Gateway != "" {
		score += 4
	}
	if defaultIface != "" && a.Name == defaultIface {
		score += 8
	}
	return score
}
