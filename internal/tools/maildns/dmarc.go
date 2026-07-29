package maildns

import (
	"strconv"
	"strings"
)

type DMARC struct {
	Found  bool   `json:"found"`
	Record string `json:"record"`
	Count  int    `json:"count"`
	// Policy is the p tag, lower case: "none", "quarantine", "reject" or "".
	Policy string `json:"policy"`
	// Subdomain is the sp tag, "" when not set.
	Subdomain string `json:"subdomain"`
	// Pct is the pct tag, 100 when not set.
	Pct int `json:"pct"`
	// RUA and RUF are the reporting addresses, [] when not set.
	RUA []string `json:"rua"`
	RUF []string `json:"ruf"`
	// Verdict is one sentence about what this policy does.
	Verdict string `json:"verdict"`
}

// findDMARC picks the DMARC record out of the TXT records at
// _dmarc.<domain>. More than one makes DMARC fail entirely, so the count is
// reported rather than the first one silently winning.
func findDMARC(txts []string) DMARC {
	out := DMARC{Pct: 100, RUA: []string{}, RUF: []string{}}
	for _, txt := range txts {
		trimmed := strings.TrimSpace(txt)
		if !hasDMARCPrefix(trimmed) {
			continue
		}
		out.Count++
		if !out.Found {
			out.Found = true
			out.Record = trimmed
		}
	}
	if !out.Found {
		return out
	}
	parseDMARC(&out)
	return out
}

func hasDMARCPrefix(txt string) bool {
	const tag = "v=dmarc1"
	if len(txt) < len(tag) || !strings.EqualFold(txt[:len(tag)], tag) {
		return false
	}
	// The version tag is followed by a semicolon or nothing at all.
	rest := strings.TrimSpace(txt[len(tag):])
	return rest == "" || strings.HasPrefix(rest, ";")
}

func parseDMARC(d *DMARC) {
	for _, part := range strings.Split(d.Record, ";") {
		name, value, ok := splitTag(part)
		if !ok {
			continue
		}
		switch name {
		case "p":
			d.Policy = strings.ToLower(value)
		case "sp":
			d.Subdomain = strings.ToLower(value)
		case "pct":
			if n, err := strconv.Atoi(value); err == nil {
				// A percentage outside 0 to 100 is malformed. Clamping rather
				// than rejecting keeps the rest of the record readable, and the
				// clamped value is what a receiving server would apply anyway.
				if n < 0 {
					n = 0
				}
				if n > 100 {
					n = 100
				}
				d.Pct = n
			}
		case "rua":
			d.RUA = splitAddresses(value)
		case "ruf":
			d.RUF = splitAddresses(value)
		}
	}
}

// splitTag reads one "name=value" pair, tolerating whitespace on either side of
// the equals sign, which real records have.
func splitTag(part string) (string, string, bool) {
	i := strings.Index(part, "=")
	if i < 0 {
		return "", "", false
	}
	name := strings.ToLower(strings.TrimSpace(part[:i]))
	value := strings.TrimSpace(part[i+1:])
	if name == "" {
		return "", "", false
	}
	return name, value, true
}

func splitAddresses(value string) []string {
	out := []string{}
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
