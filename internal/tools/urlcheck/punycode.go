package urlcheck

import "strings"

// The RFC 3492 parameter values for the Punycode variant (RFC 3492 section 5).
const (
	pyBase        = 36
	pyTMin        = 1
	pyTMax        = 26
	pySkew        = 38
	pyDamp        = 700
	pyInitialBias = 72
	pyInitialN    = 128
	pyDelimiter   = '-'
	// pyMaxInt guards the arithmetic below. A label is attacker-supplied, so a
	// crafted one must fail rather than wrap around into a plausible address.
	pyMaxInt = 1<<31 - 1
)

// DecodeHost turns every xn-- label in host back into the characters it stands
// for. A label that starts with xn-- but is not valid punycode is left exactly
// as it was, because a host that merely looks like punycode is still a host.
func DecodeHost(host string) string {
	labels := strings.Split(host, ".")
	for i, label := range labels {
		if !strings.HasPrefix(strings.ToLower(label), "xn--") {
			continue
		}
		if decoded, ok := decodeLabel(label[4:]); ok {
			labels[i] = decoded
		}
	}
	return strings.Join(labels, ".")
}

// decodeLabel implements the RFC 3492 decoding procedure for one label, with the
// xn-- prefix already removed. ok is false when the input is not valid punycode.
func decodeLabel(s string) (out string, ok bool) {
	if s == "" {
		return "", false
	}

	var runes []rune
	start := 0
	if d := strings.LastIndexByte(s, pyDelimiter); d >= 0 {
		for i := 0; i < d; i++ {
			if s[i] >= 0x80 {
				return "", false
			}
			runes = append(runes, rune(s[i]))
		}
		start = d + 1
	}

	n := pyInitialN
	i := 0
	bias := pyInitialBias

	for pos := start; pos < len(s); {
		oldi := i
		w := 1
		for k := pyBase; ; k += pyBase {
			if pos >= len(s) {
				return "", false
			}
			digit, valid := pyDigit(s[pos])
			if !valid {
				return "", false
			}
			pos++
			if digit > (pyMaxInt-i)/w {
				return "", false
			}
			i += digit * w

			t := k - bias
			if k <= bias {
				t = pyTMin
			} else if k >= bias+pyTMax {
				t = pyTMax
			}
			if digit < t {
				break
			}
			if w > pyMaxInt/(pyBase-t) {
				return "", false
			}
			w *= pyBase - t
		}

		points := len(runes) + 1
		bias = adapt(i-oldi, points, oldi == 0)
		if i/points > pyMaxInt-n {
			return "", false
		}
		n += i / points
		i %= points
		if n > 0x10FFFF || (n >= 0xD800 && n <= 0xDFFF) {
			return "", false
		}

		runes = append(runes, 0)
		copy(runes[i+1:], runes[i:])
		runes[i] = rune(n)
		i++
	}
	return string(runes), true
}

// pyDigit turns one encoded character into its base-36 value. Case is ignored on
// the letters, as RFC 3492 section 5 requires.
func pyDigit(c byte) (int, bool) {
	switch {
	case c >= 'a' && c <= 'z':
		return int(c - 'a'), true
	case c >= 'A' && c <= 'Z':
		return int(c - 'A'), true
	case c >= '0' && c <= '9':
		return int(c-'0') + 26, true
	}
	return 0, false
}

// adapt is the bias adaptation from RFC 3492 section 6.1.
func adapt(delta, numPoints int, firstTime bool) int {
	if firstTime {
		delta /= pyDamp
	} else {
		delta /= 2
	}
	delta += delta / numPoints
	k := 0
	for delta > ((pyBase-pyTMin)*pyTMax)/2 {
		delta /= pyBase - pyTMin
		k += pyBase
	}
	return k + (pyBase-pyTMin+1)*delta/(delta+pySkew)
}
