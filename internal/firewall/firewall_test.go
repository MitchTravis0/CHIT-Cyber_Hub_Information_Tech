package firewall

import (
	"strings"
	"testing"
)

func TestAllowCommand(t *testing.T) {
	tests := []struct {
		name     string
		firewall string
		port     int
		proto    string
		want     string
	}{
		// The literals are written out in full rather than assembled from the
		// same expression the code uses, so a change to either side fails here.
		{"ufw tcp", "ufw", 8722, ProtoTCP, "sudo ufw allow 8722/tcp"},
		{"ufw udp", "ufw", 8730, ProtoUDP, "sudo ufw allow 8730/udp"},
		// A bare port covers both protocols in ufw, which is what Port Listener
		// needs; two separate rules would also work but this is one line.
		{"ufw both", "ufw", 8730, ProtoBoth, "sudo ufw allow 8730"},
		{"ufw default is tcp", "ufw", 8740, "", "sudo ufw allow 8740/tcp"},
		{"firewalld tcp", "firewalld", 8722, ProtoTCP, "sudo firewall-cmd --add-port=8722/tcp"},
		{"firewalld both", "firewalld", 8730, ProtoBoth,
			"sudo firewall-cmd --add-port=8730/tcp --add-port=8730/udp"},
		{"iptables tcp", "iptables", 8722, ProtoTCP,
			"sudo iptables -I INPUT -p tcp --dport 8722 -j ACCEPT"},
		{"iptables udp", "iptables", 8730, ProtoUDP,
			"sudo iptables -I INPUT -p udp --dport 8730 -j ACCEPT"},
		// nftables needs the table and chain names, which vary per machine. A
		// wrong nft command is worse than none.
		{"nftables offers no command", "nftables", 8722, ProtoTCP, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := allowCommand(tt.firewall, tt.port, tt.proto); got != tt.want {
				t.Errorf("allowCommand(%q, %d, %q) = %q, want %q",
					tt.firewall, tt.port, tt.proto, got, tt.want)
			}
		})
	}
}

func TestMessageNamesTheFirewallAndThePort(t *testing.T) {
	with := message("ufw", 8722, "sudo ufw allow 8722/tcp")
	if !strings.Contains(with, "ufw") {
		t.Error("the message does not name the firewall, so the tech cannot act on it")
	}
	if !strings.Contains(with, "sudo ufw allow 8722/tcp") {
		t.Error("the message does not carry the command")
	}

	// With no command, the message must still say which port to allow, or it is
	// just an announcement that a firewall exists.
	without := message("nftables", 8722, "")
	if !strings.Contains(without, "8722") {
		t.Errorf("the no-command message does not name the port: %q", without)
	}
	if strings.Contains(without, "run:") {
		t.Errorf("the no-command message still offers to run something: %q", without)
	}
}

// The hint must never claim the port IS blocked. Knowing ufw is running does
// not prove that: the tech may already have allowed it, which is exactly what
// happens right after they follow this advice once.
func TestMessageDoesNotClaimThePortIsBlocked(t *testing.T) {
	m := message("ufw", 8722, "sudo ufw allow 8722/tcp")
	for _, wrong := range []string{"is blocking", "is blocked", "has blocked", "cannot connect"} {
		if strings.Contains(m, wrong) {
			t.Errorf("the message asserts %q, which it cannot know: %q", wrong, m)
		}
	}
}

func TestCheckSaysNothingWhenNoFirewallIsDetected(t *testing.T) {
	// detect() is the only per-OS part. Whatever it returns on the machine
	// running this test, an empty name must produce a completely empty Hint,
	// because the page renders nothing at all for that case.
	if got := (Hint{}); got.Message != "" || got.Command != "" || got.Firewall != "" {
		t.Fatal("the zero Hint is not empty")
	}
	h := Check(8722, ProtoTCP)
	if h.Firewall == "" && (h.Message != "" || h.Command != "") {
		t.Errorf("nothing was detected but the hint still says %q / %q", h.Message, h.Command)
	}
	if h.Firewall != "" && h.Message == "" {
		t.Errorf("%q was detected but the hint has no message", h.Firewall)
	}
}
