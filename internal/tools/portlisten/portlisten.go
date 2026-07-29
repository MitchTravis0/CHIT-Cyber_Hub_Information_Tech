// Package portlisten opens a port on this machine so a test from another
// machine proves the path through the firewall. Every other network tool in
// CHIT tests towards something that already listens; this one is the thing
// that answers, which is the only way to check a firewall rule before the
// service it is for exists.
//
// Nothing is served. One fixed line of text goes out on TCP and a datagram is
// echoed back on UDP, so whoever is testing from the other side sees proof on
// their own screen. The bytes a client sends are counted and previewed, never
// interpreted.
package portlisten

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"chit/internal/core"
)

// JobKind is the job manager kind for a listening session.
const JobKind = "portlisten"

// KindHit is the result kind of every arrival.
const KindHit = "hit"

// Protocols accepted in Params.Protocol.
const (
	ProtoTCP  = "tcp"
	ProtoUDP  = "udp"
	ProtoBoth = "both"
)

const (
	// defaultPort is above 1024 (so no elevation), out of the way, easy to read
	// out loud, and next to LAN File Drop's 8722.
	defaultPort = 8730
	minPort     = 1024
	maxPort     = 65535

	// maxHits caps the rows emitted. A port scanner sweeping this machine can
	// produce thousands in a second; past this the table stops growing and the
	// summary counts the rest.
	maxHits = 500

	// readLimit is how much of one TCP client's input is read before the
	// connection is closed. previewChars is how much of that is rendered.
	readLimit    = 1024
	previewChars = 80

	// udpDatagram is the read buffer for one datagram.
	udpDatagram = 2048
)

// Params is what the UI sends. Every zero value means "use the default".
type Params struct {
	// Port of zero means the default.
	Port int `json:"port"`
	// Protocol is "tcp", "udp" or "both". Empty means "tcp".
	Protocol string `json:"protocol"`
}

// Hit is one arrival.
type Hit struct {
	Time string `json:"time"`
	// Protocol is "tcp" or "udp", never "both".
	Protocol string `json:"protocol"`
	Peer     string `json:"peer"`
	PeerPort int    `json:"peerPort"`
	// Bytes is what the other machine sent. Zero is normal: a port scanner
	// connects and closes without sending anything.
	Bytes int `json:"bytes"`
	// Preview is a printable rendering of those bytes. Empty when Bytes is zero.
	Preview string `json:"preview"`
}

// options is the validated form of Params, produced before anything binds.
type options struct {
	port     int
	protocol string
}

func (o options) wantTCP() bool { return o.protocol == ProtoTCP || o.protocol == ProtoBoth }
func (o options) wantUDP() bool { return o.protocol == ProtoUDP || o.protocol == ProtoBoth }

// validate catches everything a user can get wrong, so bad input rejects the
// StartListen call instead of binding a port that immediately fails.
func (p Params) validate() (options, error) {
	opts := options{port: p.Port, protocol: strings.ToLower(strings.TrimSpace(p.Protocol))}

	if opts.protocol == "" {
		opts.protocol = ProtoTCP
	}
	switch opts.protocol {
	case ProtoTCP, ProtoUDP, ProtoBoth:
	default:
		return opts, core.Errorf(core.CodeInvalidInput, "Choose TCP, UDP, or both.")
	}

	if opts.port == 0 {
		opts.port = defaultPort
	}
	if opts.port < minPort || opts.port > maxPort {
		return opts, core.Errorf(core.CodeInvalidInput,
			"The port must be between %d and %d. Below %d needs administrator rights, which CHIT never asks for.",
			minPort, maxPort, minPort)
	}
	return opts, nil
}

// protocolWords names what is being listened on, for the progress line.
func protocolWords(protocol string) string {
	switch protocol {
	case ProtoUDP:
		return "UDP"
	case ProtoBoth:
		return "TCP and UDP"
	}
	return "TCP"
}

// bannerFor is the single line a TCP client gets back, so somebody using telnet
// or nc on the other machine sees proof on their own screen rather than a blank
// window.
func bannerFor(peer string) string {
	return "CHIT port listener: this port is reachable. You reached it from " + peer + ".\r\n"
}

// preview renders what arrived as something safe to put in a table cell:
// anything unprintable becomes a dot, and the result is cut on a rune boundary.
func preview(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	var out strings.Builder
	count := 0
	for i := 0; i < len(b) && count < previewChars; {
		r, size := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && size <= 1 {
			out.WriteByte('.')
		} else if unicode.IsPrint(r) {
			out.WriteRune(r)
		} else {
			out.WriteByte('.')
		}
		i += size
		count++
	}
	return out.String()
}

// sessionNote explains a session in which nothing arrived, and any rows the cap
// swallowed. A tech who does not know rows were dropped will trust a table that
// is not the whole story.
func sessionNote(tcp, udp, dropped int, protocol string) string {
	if tcp+udp == 0 {
		note := "Nothing reached this port. The block is usually this computer's own firewall (Windows asks the first time CHIT listens) or a firewall, ACL or NAT rule between the two machines."
		if protocol == ProtoUDP || protocol == ProtoBoth {
			note += " Nothing acknowledges a blocked datagram, so on UDP an empty list is not proof on its own."
		}
		return note
	}
	if dropped > 0 {
		return "Stopped listing arrivals after " + thousands(maxHits) + " rows. " +
			thousands(dropped) + " more arrived and are counted in the totals above."
	}
	return ""
}

// thousands groups a count so a four-figure number reads as one.
func thousands(n int) string {
	text := strconv.Itoa(n)
	if n < 0 {
		return text
	}
	var out strings.Builder
	for i, digit := range text {
		if i > 0 && (len(text)-i)%3 == 0 {
			out.WriteByte(',')
		}
		out.WriteRune(digit)
	}
	return out.String()
}

// summaryFor is the job:done summary. Its keys are a contract with the page and
// are checked nowhere by the compiler, in either language, so they are built in
// one named place that a test can pin.
func summaryFor(port int, protocol string, tcp, udp, peers, dropped int) map[string]any {
	return map[string]any{
		"port":     port,
		"protocol": protocol,
		"tcp":      tcp,
		"udp":      udp,
		"peers":    peers,
		"dropped":  dropped,
		"note":     sessionNote(tcp, udp, dropped, protocol),
	}
}
