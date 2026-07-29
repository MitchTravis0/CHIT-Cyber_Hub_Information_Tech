package ouidb

// Info is everything the MAC lookup UI shows for one address. It is also what
// the scanner uses to explain a missing vendor.
type Info struct {
	MAC    string `json:"mac"`    // canonical AA:BB:CC:DD:EE:FF
	OUI    string `json:"oui"`    // AA:BB:CC, the first three octets
	Vendor string `json:"vendor"` // empty when Found is false
	Found  bool   `json:"found"`

	// Bit 1 of the first octet: the address was assigned by software, not by a
	// manufacturer. Modern phones and laptops randomize it to avoid tracking.
	LocallyAdministered bool `json:"locallyAdministered"`
	// Locally administered and addressed to a single device: a randomized MAC.
	Randomized bool `json:"randomized"`
	// Bit 0 of the first octet: addressed to a group, not one device.
	Multicast bool `json:"multicast"`
	Broadcast bool `json:"broadcast"`

	// One plain sentence for the tech, empty when there is nothing to warn about.
	Note string `json:"note"`
}

// Describe parses a MAC address and explains it. ok is false only when the text
// is not a MAC address at all.
func Describe(mac string) (Info, bool) {
	b, ok := ParseMAC(mac)
	if !ok {
		return Info{}, false
	}

	info := Info{
		MAC:                 format(b),
		Multicast:           b[0]&0x01 != 0,
		LocallyAdministered: b[0]&0x02 != 0,
		Broadcast:           b == [6]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
	}
	info.OUI = info.MAC[:8]
	info.Randomized = info.LocallyAdministered && !info.Multicast
	info.Vendor, info.Found = LookupBytes(b)
	info.Note = note(info)
	return info, true
}

func note(i Info) string {
	switch {
	case i.Broadcast:
		return "This is the broadcast address, not a device. Everything on the network listens to it."
	case i.Multicast:
		return "This is a multicast address. It reaches a group of listeners rather than one device."
	case i.Randomized && !i.Found:
		return "This looks like a randomized (private) address. Phones and laptops invent these so they " +
			"cannot be tracked, so there is no manufacturer to look up. Reconnect with randomization turned " +
			"off in the Wi-Fi settings if you need the real address."
	case i.Randomized:
		return "This address is locally administered, so something set it by hand or randomized it. The " +
			"vendor below comes from the prefix and may not be the device's real maker."
	case !i.Found:
		return "This prefix is not in the vendor database. It may be a brand new assignment, so the database " +
			"may need refreshing."
	}
	return ""
}
