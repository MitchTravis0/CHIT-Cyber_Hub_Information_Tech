package wifi

import (
	"strconv"
	"strings"
)

// bandFor names the band a frequency is in. The 5 GHz range runs up to the
// boundary of the 6 GHz one so there is no gap a real channel could fall into.
func bandFor(mhz int) string {
	switch {
	case mhz >= 2400 && mhz <= 2500:
		return "2.4 GHz"
	case mhz >= 4900 && mhz <= 5924:
		return "5 GHz"
	case mhz >= 5925 && mhz <= 7125:
		return "6 GHz"
	}
	return ""
}

// channelFor converts a frequency to a channel number, or 0. Channel 14 does not
// follow the 5 MHz rule the rest of 2.4 GHz does, which is why it is a case
// rather than arithmetic.
func channelFor(mhz int) int {
	switch {
	case mhz == 2484:
		return 14
	case mhz >= 2412 && mhz <= 2472:
		if (mhz-2407)%5 != 0 {
			return 0
		}
		return (mhz - 2407) / 5
	case mhz >= 4900 && mhz <= 5924:
		if (mhz-5000)%5 != 0 {
			return 0
		}
		return (mhz - 5000) / 5
	case mhz >= 5925 && mhz <= 7125:
		if (mhz-5950)%5 != 0 {
			return 0
		}
		return (mhz - 5950) / 5
	}
	return 0
}

// frequencyFor converts a 2.4 GHz or 5 GHz channel number back to a frequency,
// for the operating systems that report only the channel.
func frequencyFor(channel int) int {
	switch {
	case channel == 14:
		return 2484
	case channel >= 1 && channel <= 13:
		return 2407 + channel*5
	case channel >= 32 && channel <= 177:
		return 5000 + channel*5
	}
	return 0
}

// percentFromDBm is Microsoft's mapping, so a Linux reading in dBm and a Windows
// reading in percent sit on the same scale rather than being two units the user
// has to convert in their head.
func percentFromDBm(dbm int) int {
	pct := 2 * (dbm + 100)
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}

// readingFromDBm is the plain sentence about the signal.
func readingFromDBm(dbm int) string {
	switch {
	case dbm >= -50:
		return "Excellent. This is right next to the access point."
	case dbm >= -60:
		return "Good. Normal for the same room as the access point."
	case dbm >= -67:
		return "Usable. This is about the edge of what voice and video calls need."
	case dbm >= -75:
		return "Weak. Expect drops and slow transfers at this desk."
	}
	return "Very weak. Move closer, or the site needs another access point here."
}

// readingFromPercent is the same ladder read from the other unit. It is defined
// as the inverse of percentFromDBm rather than as a second table, so the two
// cannot drift apart: a hand-written second table is exactly what would let them.
func readingFromPercent(pct int) string {
	return readingFromDBm(pct/2 - 100)
}

// parseIwDev reads "iw dev" and returns the names of the managed interfaces. A
// P2P or monitor pseudo-interface is not a card a tech wants to see, and the
// names come from iw's own output, never from user input.
func parseIwDev(out string) []string {
	var names []string
	current := ""
	managed := false

	flush := func() {
		if current != "" && managed {
			names = append(names, current)
		}
		current, managed = "", false
	}

	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "Interface "):
			flush()
			current = strings.TrimSpace(strings.TrimPrefix(trimmed, "Interface "))
		case strings.HasPrefix(trimmed, "phy#"), strings.HasPrefix(trimmed, "Unnamed/"):
			flush()
		case strings.HasPrefix(trimmed, "type "):
			if strings.TrimSpace(strings.TrimPrefix(trimmed, "type ")) == "managed" {
				managed = true
			}
		}
	}
	flush()
	return names
}

// parseIwLink reads "iw dev <name> link". "Not connected." yields a link with
// Connected false and nothing else, which is what the page shows as a card
// rather than a row of blanks.
func parseIwLink(out string) Link {
	link := Link{Source: "iw"}

	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "Connected to "):
			link.Connected = true
			rest := strings.TrimPrefix(line, "Connected to ")
			bssid, _, _ := strings.Cut(rest, " ")
			link.BSSID = strings.ToLower(strings.TrimSpace(bssid))
		case strings.HasPrefix(line, "SSID:"):
			link.SSID = strings.TrimSpace(strings.TrimPrefix(line, "SSID:"))
		case strings.HasPrefix(line, "freq:"):
			link.FrequencyMHz = roundedMHz(strings.TrimPrefix(line, "freq:"))
		case strings.HasPrefix(line, "signal:"):
			link.SignalDBm = leadingInt(strings.TrimPrefix(line, "signal:"))
		case strings.HasPrefix(line, "rx bitrate:"):
			link.RxMbps = leadingFloat(strings.TrimPrefix(line, "rx bitrate:"))
		case strings.HasPrefix(line, "tx bitrate:"):
			link.TxMbps = leadingFloat(strings.TrimPrefix(line, "tx bitrate:"))
		}
	}
	return link
}

// parseIwInfo reads the channel line of "iw dev <name> info", which is where the
// channel width comes from: the link output does not carry it.
//
//	channel 3 (2422 MHz), width: 20 MHz, center1: 2422 MHz
func parseIwInfo(out string) (channel, mhz, width int) {
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(raw)
		if !strings.HasPrefix(line, "channel ") {
			continue
		}
		channel = leadingInt(strings.TrimPrefix(line, "channel "))
		if open := strings.Index(line, "("); open >= 0 {
			mhz = leadingInt(line[open+1:])
		}
		if at := strings.Index(line, "width:"); at >= 0 {
			width = leadingInt(line[at+len("width:"):])
		}
		return channel, mhz, width
	}
	return 0, 0, 0
}

// netshKeys are the field names CHIT reads out of "netsh wlan show interfaces".
// They are English, which is a real limitation: on a localised Windows none of
// them matches and the tool reports no links rather than wrong ones. The note
// says so.
func parseNetshInterfaces(out string) []Link {
	var links []Link
	block := map[string]string{}

	flush := func() {
		if len(block) == 0 {
			return
		}
		if link, ok := linkFromNetsh(block); ok {
			links = append(links, link)
		}
		block = map[string]string{}
	}

	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		// A BSSID holds colons of its own, so only the first one splits the line.
		if key == "Name" {
			flush()
		}
		if key != "" {
			block[key] = value
		}
	}
	flush()
	return links
}

func linkFromNetsh(block map[string]string) (Link, bool) {
	name := block["Name"]
	if name == "" {
		return Link{}, false
	}

	link := Link{
		Interface: name,
		Connected: strings.EqualFold(block["State"], "connected"),
		SSID:      block["SSID"],
		BSSID:     strings.ToLower(block["BSSID"]),
		Security:  block["Authentication"],
		Band:      block["Band"],
		Source:    "netsh wlan",
	}
	if radio := block["Radio type"]; radio != "" && link.Security != "" {
		link.Security += " (" + radio + ")"
	}
	if channel := block["Channel"]; channel != "" {
		link.Channel = leadingInt(channel)
	}
	link.RxMbps = leadingFloat(block["Receive rate (Mbps)"])
	link.TxMbps = leadingFloat(block["Transmit rate (Mbps)"])
	link.SignalPercent = leadingInt(block["Signal"])
	return link, true
}

// parseAirportChannel reads the macOS form "3 (2GHz, 20MHz)", which carries the
// channel, the band and the width in one string.
func parseAirportChannel(text string) (channel int, band string, width int) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, "", 0
	}
	channel = leadingInt(text)

	open := strings.Index(text, "(")
	if open < 0 {
		return channel, "", 0
	}
	inner := text[open+1:]
	if close := strings.Index(inner, ")"); close >= 0 {
		inner = inner[:close]
	}
	for _, part := range strings.Split(inner, ",") {
		part = strings.TrimSpace(part)
		switch {
		case strings.EqualFold(part, "2GHz"):
			band = "2.4 GHz"
		case strings.EqualFold(part, "5GHz"):
			band = "5 GHz"
		case strings.EqualFold(part, "6GHz"):
			band = "6 GHz"
		case strings.HasSuffix(strings.ToUpper(part), "MHZ"):
			width = leadingInt(part)
		}
	}
	return channel, band, width
}

// parseSignalNoise reads the macOS form "-39 dBm / -92 dBm" and keeps the first
// number, which is the signal.
func parseSignalNoise(text string) int {
	return leadingInt(strings.TrimSpace(text))
}

// leadingInt reads the first integer in a string, sign included, or 0.
func leadingInt(text string) int {
	start, end := -1, -1
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c >= '0' && c <= '9' {
			if start < 0 {
				start = i
				if i > 0 && text[i-1] == '-' {
					start = i - 1
				}
			}
			end = i + 1
			continue
		}
		if start >= 0 {
			break
		}
	}
	if start < 0 {
		return 0
	}
	n, err := strconv.Atoi(text[start:end])
	if err != nil {
		return 0
	}
	return n
}

// leadingFloat reads the first number in a string, decimals included, or 0.
func leadingFloat(text string) float64 {
	start, end := -1, -1
	for i := 0; i < len(text); i++ {
		c := text[i]
		if (c >= '0' && c <= '9') || (c == '.' && start >= 0) {
			if start < 0 {
				start = i
				if i > 0 && text[i-1] == '-' {
					start = i - 1
				}
			}
			end = i + 1
			continue
		}
		if start >= 0 {
			break
		}
	}
	if start < 0 {
		return 0
	}
	n, err := strconv.ParseFloat(strings.TrimSuffix(text[start:end], "."), 64)
	if err != nil {
		return 0
	}
	return n
}

// roundedMHz reads "2422.0" as 2422: iw prints the frequency as a float.
func roundedMHz(text string) int {
	return int(leadingFloat(text) + 0.5)
}
