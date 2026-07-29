package tracert

import (
	"fmt"
	"math"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

// Both parsers are token based and never match an English word. Windows and
// Linux translate their own messages ("Request timed out." is French on a
// French Windows), so a parser keyed on words breaks on a customer machine.
// Every decision below is made on digits, punctuation and address shapes only.

// unixTime matches a bare millisecond count, which traceroute always follows
// with a separate "ms" token.
var unixTime = regexp.MustCompile(`^\d+(\.\d+)?$`)

// tracertTime matches a Windows millisecond count, including the "<1" form for
// anything under a millisecond. A dotted address never matches: it has more
// than one dot group.
var tracertTime = regexp.MustCompile(`^<?\d+(\.\d+)?$`)

func newHop(number int) Hop {
	return Hop{Number: number, TimesMS: []float64{}, AlsoSeen: []string{}}
}

// hopNumber accepts only a canonical hop number, which is what separates a hop
// line from a header, a blank line or a trailer.
func hopNumber(field string) (int, bool) {
	if len(field) < 1 || len(field) > 3 {
		return 0, false
	}
	if len(field) > 1 && field[0] == '0' {
		return 0, false
	}
	for i := 0; i < len(field); i++ {
		if field[i] < '0' || field[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(field)
	if err != nil {
		return 0, false
	}
	return n, true
}

// parseUnixHop reads one line of GNU or BSD traceroute output. It returns
// false for anything that is not a hop line.
func parseUnixHop(line string) (Hop, bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return Hop{}, false
	}
	number, ok := hopNumber(fields[0])
	if !ok {
		return Hop{}, false
	}

	hop := newHop(number)
	pendingName := ""
	for i := 1; i < len(fields); i++ {
		field := fields[i]
		switch {
		case field == "*":
			hop.Lost++
		case i+1 < len(fields) && fields[i+1] == "ms" && unixTime.MatchString(field):
			if ms, err := strconv.ParseFloat(field, 64); err == nil {
				hop.TimesMS = append(hop.TimesMS, ms)
			}
			i++
		case strings.HasPrefix(field, "!"):
			hop.Note = annotationNote(field)
		default:
			if addr, ok := parenAddress(field); ok {
				addAddress(&hop, addr, pendingName)
				pendingName = ""
			} else if _, err := netip.ParseAddr(field); err == nil {
				addAddress(&hop, field, "")
				pendingName = ""
			} else {
				pendingName = field
			}
		}
	}

	setStats(&hop)
	return hop, true
}

// parseTracertHop reads one line of Windows tracert output. It returns false
// for anything that is not a hop line.
func parseTracertHop(line string) (Hop, bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return Hop{}, false
	}
	number, ok := hopNumber(fields[0])
	if !ok {
		return Hop{}, false
	}

	hop := newHop(number)
	for _, field := range fields[1:] {
		if field == "*" {
			hop.Lost++
			continue
		}
		if !tracertTime.MatchString(field) {
			continue
		}
		if ms, err := strconv.ParseFloat(strings.TrimPrefix(field, "<"), 64); err == nil {
			hop.TimesMS = append(hop.TimesMS, ms)
		}
	}

	last := fields[len(fields)-1]
	if addr, ok := bracketAddress(last); ok {
		hop.IP = addr
		if len(fields) > 2 {
			before := fields[len(fields)-2]
			if before != "*" && !tracertTime.MatchString(before) {
				hop.Hostname = before
			}
		}
	} else if _, err := netip.ParseAddr(last); err == nil {
		hop.IP = last
	}

	setStats(&hop)
	return hop, true
}

// parenAddress unwraps the "(192.0.2.1)" form traceroute prints after a name.
func parenAddress(field string) (string, bool) {
	if len(field) < 3 || field[0] != '(' || field[len(field)-1] != ')' {
		return "", false
	}
	inner := field[1 : len(field)-1]
	if _, err := netip.ParseAddr(inner); err != nil {
		return "", false
	}
	return inner, true
}

// bracketAddress unwraps the "[192.0.2.1]" form tracert prints after a name.
func bracketAddress(field string) (string, bool) {
	if len(field) < 3 || field[0] != '[' || field[len(field)-1] != ']' {
		return "", false
	}
	inner := field[1 : len(field)-1]
	if _, err := netip.ParseAddr(inner); err != nil {
		return "", false
	}
	return inner, true
}

// addAddress records the first address on the line as the hop, and any later
// one as an extra router. The name of an extra address is dropped: one row
// carries one name. A repeat of the address already recorded is not an extra
// router, it is traceroute printing the address in the name column because
// the reverse lookup found nothing.
func addAddress(hop *Hop, ip, name string) {
	if hop.IP == "" {
		hop.IP = ip
		hop.Hostname = name
		return
	}
	if ip == hop.IP {
		return
	}
	for _, seen := range hop.AlsoSeen {
		if seen == ip {
			return
		}
	}
	hop.AlsoSeen = append(hop.AlsoSeen, ip)
}

func setStats(hop *Hop) {
	hop.BestMS, hop.AvgMS, hop.WorstMS = hopStats(hop.TimesMS)
}

// hopStats reduces the probe times to the three numbers the UI shows, rounded
// to two decimals so a table of them lines up.
func hopStats(times []float64) (best, avg, worst float64) {
	if len(times) == 0 {
		return 0, 0, 0
	}
	best, worst = times[0], times[0]
	total := 0.0
	for _, t := range times {
		if t < best {
			best = t
		}
		if t > worst {
			worst = t
		}
		total += t
	}
	return round2(best), round2(total / float64(len(times))), round2(worst)
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// annotationNote turns a traceroute annotation into something a tech can act
// on. The letters come from the traceroute manual page.
func annotationNote(tag string) string {
	switch tag {
	case "!H":
		return "the router said it cannot reach that host"
	case "!N":
		return "the router said it cannot reach that network"
	case "!P":
		return "the router said it cannot reach that protocol"
	case "!X", "!A":
		return "the router refused to pass the traffic, which usually means a firewall rule"
	case "!S":
		return "the source route failed"
	case "!F":
		return "the packet was too big for the link and needs fragmenting"
	}
	return fmt.Sprintf("the router reported a problem (%s)", tag)
}
