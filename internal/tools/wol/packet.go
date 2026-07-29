package wol

import (
	"encoding/hex"

	"chit/internal/core"
	"chit/internal/ouidb"
)

// MagicPacket builds the wake-up packet for mac: six 0xFF bytes then the MAC
// sixteen times. password is empty, 4 bytes or 6 bytes (SecureOn), which a few
// Realtek and Intel adapters require before they will act on the packet.
func MagicPacket(mac [6]byte, password []byte) ([]byte, error) {
	if len(password) != 0 && len(password) != 4 && len(password) != 6 {
		return nil, core.Errorf(core.CodeInvalidInput,
			"The SecureOn password must be 4 or 6 pairs of hex digits, for example 11-22-33-44. %d were given.",
			len(password))
	}

	info, _ := ouidb.Describe(hex.EncodeToString(mac[:]))
	if info.Multicast || info.Broadcast {
		return nil, core.Errorf(core.CodeInvalidInput,
			"%s is not a single device's address, so it cannot be woken. Use the MAC address printed on the machine or shown by the scanner.",
			info.MAC)
	}

	out := make([]byte, 0, 6+16*len(mac)+len(password))
	for i := 0; i < 6; i++ {
		out = append(out, 0xFF)
	}
	for i := 0; i < 16; i++ {
		out = append(out, mac[:]...)
	}
	return append(out, password...), nil
}
