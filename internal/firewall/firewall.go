// Package firewall answers one question for the three tools that open an
// inbound socket: is a firewall on this computer likely to be dropping the
// connections the other machine is making?
//
// It exists because of a real failure. On 2026-07-29 a LAN File Drop was
// running correctly, bound to *:8722, answering 200 on the machine's own LAN
// address, and no other device could open it. ufw was dropping every SYN. A
// dropped SYN produces no RST, so the far end simply hangs and CHIT sees
// nothing at all: no error, no connection, no clue. Windows and macOS at least
// ask the tech a question the first time; Linux says nothing, which is the case
// this package is for.
//
// It never changes a firewall rule and never asks for elevation. It reads a
// state and writes a sentence.
package firewall

import "strconv"

// Hint is what the page shows. Every field is empty when nothing was detected,
// which is the normal case on a machine with no firewall running, and also on
// every platform except Linux (see firewall_other.go for why).
type Hint struct {
	// Firewall is the name of what is running: "ufw", "firewalld", "nftables"
	// or "iptables". Empty means nothing was detected.
	Firewall string `json:"firewall"`
	// Message is the whole sentence to show, already written for a junior tech.
	// Empty means show nothing.
	Message string `json:"message"`
	// Command is the exact line to run, for a copy button. Empty when this
	// firewall has no single command that is safe to hand somebody.
	Command string `json:"command"`
}

// Protocols a caller may ask about. Port Listener answers on both at once.
const (
	ProtoTCP  = "tcp"
	ProtoUDP  = "udp"
	ProtoBoth = "both"
)

// Check reports whether a host firewall is running that could be dropping
// inbound connections to this port. It is deliberately a *hint*: knowing that
// ufw is active does not prove this port is blocked, because the tech may
// already have allowed it. Saying "this might be why" is useful; claiming "this
// is why" would be wrong about half the time.
func Check(port int, proto string) Hint {
	name := detect()
	if name == "" {
		return Hint{}
	}
	command := allowCommand(name, port, proto)
	return Hint{Firewall: name, Message: message(name, port, command), Command: command}
}

// message is the sentence, built in one place so the three pages render a
// string rather than each assembling their own.
func message(name string, port int, command string) string {
	base := "This computer's firewall (" + name + ") is switched on. If the other machine cannot " +
		"reach this, that is the first thing to check: nothing on this screen can tell the " +
		"difference between a blocked port and nobody trying."
	if command == "" {
		return base + " Allow port " + strconv.Itoa(port) + " through it, then try again."
	}
	return base + " To let it through, run: " + command
}

// allowCommand returns the line a tech can paste. An empty string means this
// firewall has no one-liner that is safe to give somebody without knowing their
// table and chain names, which is true of nftables.
func allowCommand(name string, port int, proto string) string {
	p := strconv.Itoa(port)
	switch name {
	case "ufw":
		// "ufw allow 8730" with no protocol covers TCP and UDP together, which
		// is exactly what Port Listener needs.
		switch proto {
		case ProtoBoth:
			return "sudo ufw allow " + p
		case ProtoUDP:
			return "sudo ufw allow " + p + "/udp"
		default:
			return "sudo ufw allow " + p + "/tcp"
		}
	case "firewalld":
		switch proto {
		case ProtoBoth:
			return "sudo firewall-cmd --add-port=" + p + "/tcp --add-port=" + p + "/udp"
		case ProtoUDP:
			return "sudo firewall-cmd --add-port=" + p + "/udp"
		default:
			return "sudo firewall-cmd --add-port=" + p + "/tcp"
		}
	case "iptables":
		switch proto {
		case ProtoUDP:
			return "sudo iptables -I INPUT -p udp --dport " + p + " -j ACCEPT"
		default:
			return "sudo iptables -I INPUT -p tcp --dport " + p + " -j ACCEPT"
		}
	}
	// nftables: the rule needs the table and chain names, which vary per
	// machine. A wrong nft command is worse than none, so the message says what
	// to do instead.
	return ""
}
