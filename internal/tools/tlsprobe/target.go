package tlsprobe

import (
	"net"
	"net/url"
	"strconv"
	"strings"

	"chit/internal/core"
)

// DefaultPort is where a TLS service listens when the user did not say.
const DefaultPort = 443

// target is a validated host and port to probe.
type target struct {
	host string
	port int
}

func (t target) address() string { return net.JoinHostPort(t.host, strconv.Itoa(t.port)) }

// parseTarget accepts what a tech will actually paste: a bare name, host:port,
// a whole https:// URL, or an IPv6 literal with or without brackets.
func parseTarget(raw string) (target, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return target{}, core.Errorf(core.CodeInvalidInput,
			"Type the server to probe, for example mail.example.com.")
	}

	if i := strings.Index(text, "://"); i >= 0 {
		scheme := strings.ToLower(text[:i])
		if scheme != "http" && scheme != "https" {
			return target{}, core.Errorf(core.CodeInvalidInput,
				"CHIT can only probe a plain TLS service. Drop the %s:// and type just the server name.", scheme)
		}
		u, err := url.Parse(text)
		if err != nil || u.Hostname() == "" {
			return target{}, badTarget(raw)
		}
		text = u.Hostname()
		if p := u.Port(); p != "" {
			text = net.JoinHostPort(u.Hostname(), p)
		}
	}

	host, portText, err := net.SplitHostPort(text)
	if err != nil {
		// No port at all, or a bare IPv6 literal, which SplitHostPort reads as
		// a pile of colons. Either way the whole string is the host.
		if strings.ContainsAny(text, " \t") {
			return target{}, badTarget(raw)
		}
		return target{host: text, port: DefaultPort}, nil
	}
	if host == "" {
		return target{}, badTarget(raw)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return target{}, core.Errorf(core.CodeInvalidInput,
			"%q does not name a port between 1 and 65535.", raw)
	}
	return target{host: host, port: port}, nil
}

func badTarget(raw string) error {
	return core.Errorf(core.CodeInvalidInput,
		"%q is not a server name or an IP address. Try mail.example.com or 192.168.1.10.", raw)
}
