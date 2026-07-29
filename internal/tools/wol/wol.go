// Package wol wakes a sleeping machine with a magic packet, and reports
// exactly where that packet went. Sending it to the wrong address is the usual
// reason Wake-on-LAN looks like it does nothing, so the destinations are part
// of the result rather than a detail hidden in the code.
package wol

import (
	"encoding/hex"
	"fmt"
	"net"
	"strings"

	"chit/internal/core"
	"chit/internal/netinfo"
	"chit/internal/ouidb"
)

// defaultPort is the discard port, which is what nearly every network card
// listens on for a magic packet.
const defaultPort = 9

// sendFailure is what the UI shows for a dial or write that the OS refused.
// The raw error text is deliberately dropped: it means nothing to a tech.
const sendFailure = "The operating system refused to send on this network."

type WakeParams struct {
	MAC string `json:"mac"`
	// Broadcast overrides the automatic choice. Empty is the normal case.
	Broadcast string `json:"broadcast"`
	// Port is 9 when zero.
	Port int `json:"port"`
	// Password is the optional SecureOn password as hex, e.g. "11-22-33-44" or
	// "112233445566". Empty for almost every machine.
	Password string `json:"password"`
}

// Send is one packet that was attempted.
type Send struct {
	Adapter string `json:"adapter"`
	From    string `json:"from"`
	To      string `json:"to"`
	Port    int    `json:"port"`
	Bytes   int    `json:"bytes"`
	// Error is a plain sentence, empty when the packet went out.
	Error string `json:"error"`
}

type WakeResult struct {
	MAC string `json:"mac"` // canonical AA:BB:CC:DD:EE:FF
	// Vendor names the maker, so a tech can sanity check the address before
	// blaming the BIOS. Empty when the prefix is unknown.
	Vendor string `json:"vendor"`
	Sent   []Send `json:"sent"`
	Failed []Send `json:"failed"`
	// PacketHex is the packet as hex, for a tech who wants to prove what was
	// sent. Always present.
	PacketHex string `json:"packetHex"`
	Note      string `json:"note"`
}

type TargetList struct {
	Targets []Target `json:"targets"`
	// Note explains an empty list.
	Note string `json:"note"`
}

// Wake sends one magic packet to every address the sleeping machine might be
// listening on, and reports every one of them.
func Wake(p WakeParams) (WakeResult, error) {
	info, ok := ouidb.Describe(p.MAC)
	if !ok {
		return WakeResult{}, core.Errorf(core.CodeInvalidInput,
			"That is not a MAC address. Enter 12 hex digits, for example AA:BB:CC:DD:EE:FF.")
	}
	mac, _ := ouidb.ParseMAC(p.MAC)

	password, err := decodePassword(p.Password)
	if err != nil {
		return WakeResult{}, err
	}

	port := p.Port
	if port == 0 {
		port = defaultPort
	}
	if port < 1 || port > 65535 {
		return WakeResult{}, core.Errorf(core.CodeInvalidInput,
			"%d is not a port number. Ports run from 1 to 65535. Wake-on-LAN normally uses 9.", port)
	}

	packet, err := MagicPacket(mac, password)
	if err != nil {
		return WakeResult{}, err
	}

	report, err := netinfo.List()
	if err != nil {
		return WakeResult{}, err
	}
	targets, err := SelectTargets(report.Adapters, p.Broadcast, port)
	if err != nil {
		return WakeResult{}, err
	}

	result := WakeResult{
		MAC:       info.MAC,
		Vendor:    info.Vendor,
		Sent:      []Send{},
		Failed:    []Send{},
		PacketHex: hex.EncodeToString(packet),
	}
	for _, t := range targets {
		s := sendTo(t, packet)
		if s.Error == "" {
			result.Sent = append(result.Sent, s)
			continue
		}
		result.Failed = append(result.Failed, s)
	}

	if len(result.Sent) == 0 {
		return WakeResult{}, core.Errorf(core.CodeNetwork,
			"The wake-up packet could not be sent on any network. Check that CHIT is allowed through the firewall.")
	}
	result.Note = wakeNote(result.Sent, result.Failed)
	return result, nil
}

// Targets lists where a packet would go right now, so the UI can show it before
// anything is sent. A machine with no usable network gets the explanation in
// Note rather than an error, because nothing was asked for yet.
func Targets() (TargetList, error) {
	report, err := netinfo.List()
	if err != nil {
		return TargetList{}, err
	}
	targets, err := SelectTargets(report.Adapters, "", defaultPort)
	if err != nil {
		return TargetList{Targets: []Target{}, Note: core.MessageOf(err)}, nil
	}
	return TargetList{Targets: targets, Note: ""}, nil
}

// sendTo sends one packet and translates any failure into a sentence. The
// source address is bound explicitly because on a laptop with Wi-Fi, Ethernet
// and a VPN an unbound broadcast leaves by whichever interface the routing
// table prefers, which is regularly the VPN.
func sendTo(t Target, packet []byte) Send {
	out := Send{Adapter: t.Adapter, From: t.From, To: t.To, Port: t.Port}

	conn, err := net.DialUDP("udp4",
		&net.UDPAddr{IP: net.ParseIP(t.From)},
		&net.UDPAddr{IP: net.ParseIP(t.To), Port: t.Port})
	if err != nil {
		out.Error = sendFailure
		return out
	}
	defer conn.Close()

	n, err := conn.Write(packet)
	if err != nil {
		out.Error = sendFailure
		return out
	}
	out.Bytes = n
	return out
}

// decodePassword accepts the separators a tech will paste from a manual.
func decodePassword(text string) ([]byte, error) {
	cleaned := strings.NewReplacer(":", "", "-", "", ".", "", " ", "").Replace(text)
	if cleaned == "" {
		return nil, nil
	}
	b, err := hex.DecodeString(cleaned)
	if err != nil {
		return nil, core.Errorf(core.CodeInvalidInput,
			"The SecureOn password must be hex digits only, for example 11-22-33-44.")
	}
	return b, nil
}

func wakeNote(sent, failed []Send) string {
	if len(failed) > 0 {
		return fmt.Sprintf("The packet went out on %d of %d networks. The ones that failed are listed below.",
			len(sent), len(sent)+len(failed))
	}
	if n := countAdapters(sent); n > 1 {
		return fmt.Sprintf("Sent on %d networks, because this computer is on more than one. Only the one the sleeping machine is plugged into matters.", n)
	}
	return ""
}

func countAdapters(sends []Send) int {
	seen := make(map[string]bool, len(sends))
	for _, s := range sends {
		seen[s.Adapter] = true
	}
	return len(seen)
}
