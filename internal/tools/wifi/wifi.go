// Package wifi reports what this machine's wireless adapters are connected to
// right now: network, band, channel, negotiated rate and signal. The shipped
// Network Adapter Info tool answers the addressing question and says nothing
// about the radio, which is what "the Wi-Fi is bad in the corner office" is
// actually about.
//
// Everything here is read only: nothing connects, disconnects or changes a
// setting, and no saved network password is read.
package wifi

import (
	"runtime"

	"chit/internal/core"
)

// Ids used in Report.Unsupported.
const (
	FieldSignalDBm = "signalDbm"
	FieldBSSID     = "bssid"
	FieldSSID      = "ssid"
	FieldWidth     = "width"
	FieldSecurity  = "security"
	FieldRate      = "rate"
)

// commandTimeout caps iw and netsh. system_profiler gets a longer budget.
const (
	commandTimeout         = 10 // seconds
	profilerCommandTimeout = 15 // seconds
)

// Link is one wireless interface.
type Link struct {
	Interface string `json:"interface"`
	Connected bool   `json:"connected"`
	SSID      string `json:"ssid"`
	// BSSID is the access point's MAC, lowercase with colons, or "".
	BSSID string `json:"bssid"`
	// Band is "2.4 GHz", "5 GHz", "6 GHz" or "" when the frequency is unknown.
	Band         string `json:"band"`
	Channel      int    `json:"channel"`
	FrequencyMHz int    `json:"frequencyMhz"`
	WidthMHz     int    `json:"widthMhz"`
	// SignalDBm is zero when this operating system reports only a percentage.
	SignalDBm int `json:"signalDbm"`
	// SignalPercent is filled whenever there is any signal figure at all,
	// derived from dBm where only dBm was given, so a Linux reading and a
	// Windows reading sit on the same scale.
	SignalPercent int     `json:"signalPercent"`
	RxMbps        float64 `json:"rxMbps"`
	TxMbps        float64 `json:"txMbps"`
	Security      string  `json:"security"`
	// Reading is the plain sentence about the signal, never empty when there is
	// a signal figure.
	Reading string `json:"reading"`
	// Source is where CHIT read it: "iw", "netsh wlan", "system_profiler".
	Source string `json:"source"`
}

type Report struct {
	OS    string `json:"os"`
	Links []Link `json:"links"`
	// Unsupported lists field ids this operating system would not report, so the
	// card can say "not reported on Windows" instead of showing a blank.
	Unsupported []string `json:"unsupported"`
	// Note always has a sentence in it.
	Note string `json:"note"`
}

// List reads whatever this operating system will say about the wireless.
func List() (Report, error) {
	r := Report{
		OS: runtime.GOOS,
		// Both slices are initialised so they marshal as [] rather than null.
		Links:       []Link{},
		Unsupported: []string{},
	}

	collect(&r)

	for i := range r.Links {
		link := &r.Links[i]
		if !link.Connected {
			continue
		}
		if link.SignalDBm != 0 {
			link.SignalPercent = percentFromDBm(link.SignalDBm)
			link.Reading = readingFromDBm(link.SignalDBm)
		} else if link.SignalPercent > 0 {
			link.Reading = readingFromPercent(link.SignalPercent)
		}
		if link.Band == "" && link.FrequencyMHz > 0 {
			link.Band = bandFor(link.FrequencyMHz)
		}
		if link.Channel == 0 && link.FrequencyMHz > 0 {
			link.Channel = channelFor(link.FrequencyMHz)
		}
		if link.FrequencyMHz == 0 && link.Channel > 0 {
			link.FrequencyMHz = frequencyFor(link.Channel)
		}
	}

	if r.Note == "" {
		return r, core.Errorf(core.CodeInternal,
			"Could not read this machine's wireless details. Try Refresh, and if it keeps happening this computer is refusing something CHIT normally gets without admin rights.")
	}
	return r, nil
}

// The per-OS notes, in one place so a test can pin every branch.
const (
	noteNoAdapter = "This machine has no wireless adapter, or it is switched off."
	noteLinux     = "Signal is in dBm, where a number closer to zero is stronger. Linux does not report which security the connection is using, so that field is left out."
	noteNoIw      = "This machine does not have the iw command, so CHIT cannot read the wireless details. On most distributions it is in a package called iw or wireless-tools."
	noteWindows   = "Windows reports the signal as a percentage rather than in dBm, so the percentage here is the real figure and no dBm number is shown. If this page is empty on a machine that is connected, the most likely reason is that Windows is not in English: CHIT reads netsh's output by name and those names are translated."
	noteDarwin    = "macOS reports one rate rather than separate send and receive rates, so both are shown the same."
	noteDarwinNo  = "macOS will not tell an app the name of the network it is on unless the app has been given location access, so the network name is missing. Everything else on this page is still real."
	noteOther     = "CHIT does not know how to read wireless details on this operating system."
)

// noteFor picks the sentence for this operating system and what it managed to
// read. A tech who does not read the note will take an empty page for a broken
// adapter.
func noteFor(os string, links int, ssid bool) string {
	if links == 0 {
		switch os {
		case "linux", "windows", "darwin":
			return noteNoAdapter
		}
		return noteOther
	}
	switch os {
	case "linux":
		return noteLinux
	case "windows":
		return noteWindows
	case "darwin":
		if ssid {
			return noteDarwin
		}
		return noteDarwinNo
	}
	return noteOther
}

// markUnsupported records a field this operating system would not report, once.
func (r *Report) markUnsupported(fields ...string) {
	for _, f := range fields {
		found := false
		for _, existing := range r.Unsupported {
			if existing == f {
				found = true
				break
			}
		}
		if !found {
			r.Unsupported = append(r.Unsupported, f)
		}
	}
}
