package portscan

import (
	"slices"
	"strings"
	"unicode"

	"chit/internal/core"
)

// parsePorts reads the spec a tech types: "443", "80,443", "1-1024" or
// "80, 8000-8100". The result is sorted and de-duplicated. This is the only
// implementation of this parsing in the repo; the UI sends the raw string.
func parsePorts(spec string) ([]int, error) {
	tokens := strings.FieldsFunc(spec, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})
	if len(tokens) == 0 {
		return nil, core.Errorf(core.CodeInvalidInput,
			"Enter at least one port to check, for example 80,443 or 1-1024.")
	}

	var ports []int
	for _, token := range tokens {
		low, high, err := parseToken(token)
		if err != nil {
			return nil, err
		}
		for port := low; port <= high; port++ {
			ports = append(ports, port)
		}
	}

	slices.Sort(ports)
	return slices.Compact(ports), nil
}

// parseToken reads one "N" or "N-M" token and returns the inclusive bounds.
func parseToken(token string) (int, int, error) {
	lowText, highText, isRange := strings.Cut(token, "-")
	if !isRange {
		port, err := portNumber(lowText, token)
		return port, port, err
	}

	low, err := portNumber(lowText, token)
	if err != nil {
		return 0, 0, err
	}
	high, err := portNumber(highText, token)
	if err != nil {
		return 0, 0, err
	}
	if high < low {
		return 0, 0, core.Errorf(core.CodeInvalidInput,
			"The range %s runs backwards. Put the smaller port first, for example 80-443.", token)
	}
	return low, high, nil
}

// portNumber reads one number out of a token. A part made only of digits and a
// sign was meant to be a number, so it gets the message about the port range;
// anything else was never a port at all and gets the message about the format.
func portNumber(part, token string) (int, error) {
	if n, ok := parsePort(part); ok {
		return n, nil
	}
	if numeric(part) {
		return 0, core.Errorf(core.CodeInvalidInput,
			"Ports run from 1 to 65535, so %s is not one.", part)
	}
	return 0, core.Errorf(core.CodeInvalidInput,
		"%q is not a port or a port range. Use a number like 443, a list like 80,443, or a range like 1-1024.", token)
}

// parsePort takes the canonical decimal form only. strconv.Atoi would accept
// "+80" and "080", neither of which is what a tech meant to type, and the same
// reasoning already governs parseOctet in internal/netscan.
func parsePort(s string) (int, bool) {
	if s == "" || len(s) > 5 || (len(s) > 1 && s[0] == '0') {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
	}
	if n < 1 || n > 65535 {
		return 0, false
	}
	return n, true
}

func numeric(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if (s[i] < '0' || s[i] > '9') && s[i] != '+' {
			return false
		}
	}
	return true
}
