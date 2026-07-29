package pubip

import (
	"encoding/json"
	"errors"
	"net/netip"
	"strconv"
	"strings"
)

// errProvider means "this service did not give a usable answer, try the next
// one". It never reaches a user: Lookup turns a run of these into the one
// message written for a tech.
var errProvider = errors.New("provider did not return a usable answer")

// parseIPInfo reads the ipinfo.io body. Fields it does not know about are
// ignored rather than treated as an error.
func parseIPInfo(body []byte) (Info, error) {
	var found struct {
		IP       string `json:"ip"`
		City     string `json:"city"`
		Region   string `json:"region"`
		Country  string `json:"country"`
		Loc      string `json:"loc"`
		Org      string `json:"org"`
		Timezone string `json:"timezone"`
		Bogon    bool   `json:"bogon"`
	}
	if err := json.Unmarshal(body, &found); err != nil {
		return Info{}, errProvider
	}
	// A bogon answer means the request never left a private network, so the
	// address in it is not this site's public one.
	if found.Bogon {
		return Info{}, errProvider
	}
	if _, err := netip.ParseAddr(found.IP); err != nil {
		return Info{}, errProvider
	}

	info := Info{
		IPv4:        found.IP,
		City:        found.City,
		Region:      found.Region,
		Country:     found.Country,
		CountryName: countryName(found.Country),
		Timezone:    found.Timezone,
	}
	info.Latitude, info.Longitude = parseLoc(found.Loc)
	info.ASN, info.ISP = splitOrg(found.Org)
	return info, nil
}

// parseIPWho reads the ipwho.is body, which nests the ISP and the time zone.
func parseIPWho(body []byte) (Info, error) {
	var found struct {
		IP          string  `json:"ip"`
		Success     *bool   `json:"success"`
		City        string  `json:"city"`
		Region      string  `json:"region"`
		Country     string  `json:"country"`
		CountryCode string  `json:"country_code"`
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
		Connection  struct {
			ASN int    `json:"asn"`
			Org string `json:"org"`
			ISP string `json:"isp"`
		} `json:"connection"`
		Timezone struct {
			ID string `json:"id"`
		} `json:"timezone"`
	}
	if err := json.Unmarshal(body, &found); err != nil {
		return Info{}, errProvider
	}
	if found.Success != nil && !*found.Success {
		return Info{}, errProvider
	}
	if _, err := netip.ParseAddr(found.IP); err != nil {
		return Info{}, errProvider
	}

	isp := found.Connection.ISP
	if isp == "" {
		isp = found.Connection.Org
	}
	asn := ""
	if found.Connection.ASN != 0 {
		asn = "AS" + strconv.Itoa(found.Connection.ASN)
	}

	return Info{
		IPv4:        found.IP,
		ISP:         isp,
		ASN:         asn,
		City:        found.City,
		Region:      found.Region,
		Country:     found.CountryCode,
		CountryName: found.Country,
		Timezone:    found.Timezone.ID,
		Latitude:    found.Latitude,
		Longitude:   found.Longitude,
	}, nil
}

// parseCloudflareTrace reads the key=value lines Cloudflare's trace endpoint
// returns. It knows only the address and the country, so an answer from here
// is flagged partial and explains itself.
func parseCloudflareTrace(body []byte) (Info, error) {
	values := map[string]string{}
	for _, line := range strings.Split(string(body), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		values[key] = value
	}
	ip := values["ip"]
	if _, err := netip.ParseAddr(ip); err != nil {
		return Info{}, errProvider
	}

	return Info{
		IPv4:        ip,
		Country:     values["loc"],
		CountryName: countryName(values["loc"]),
		Partial:     true,
		Note:        "Only the address could be looked up: the services that name the ISP and location did not answer. The address itself is correct.",
	}, nil
}

// parseLoc reads ipinfo.io's "lat,lon" pair. Anything unreadable leaves both
// at zero, which the UI treats as "no location" rather than the Atlantic.
func parseLoc(loc string) (float64, float64) {
	latText, lonText, ok := strings.Cut(loc, ",")
	if !ok {
		return 0, 0
	}
	lat, err := strconv.ParseFloat(strings.TrimSpace(latText), 64)
	if err != nil {
		return 0, 0
	}
	lon, err := strconv.ParseFloat(strings.TrimSpace(lonText), 64)
	if err != nil {
		return 0, 0
	}
	return lat, lon
}

// splitOrg pulls the AS number off the front of ipinfo.io's org string
// ("AS12345 Example Communications Ltd").
func splitOrg(org string) (asn string, isp string) {
	first, rest, ok := strings.Cut(strings.TrimSpace(org), " ")
	if !ok || !isASNumber(first) {
		return "", strings.TrimSpace(org)
	}
	return first, strings.TrimSpace(rest)
}

func isASNumber(word string) bool {
	if !strings.HasPrefix(word, "AS") || len(word) < 3 {
		return false
	}
	for _, r := range word[2:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// countryNames covers the codes a UK-based field tech sees. Anything else
// shows its ISO code, which is a display nicety rather than data.
var countryNames = map[string]string{
	"GB": "United Kingdom", "US": "United States", "IE": "Ireland", "CA": "Canada",
	"AU": "Australia", "NZ": "New Zealand", "DE": "Germany", "FR": "France",
	"ES": "Spain", "IT": "Italy", "NL": "Netherlands", "BE": "Belgium",
	"PL": "Poland", "SE": "Sweden", "NO": "Norway", "DK": "Denmark",
	"FI": "Finland", "PT": "Portugal", "CH": "Switzerland", "AT": "Austria",
	"IN": "India", "ZA": "South Africa", "SG": "Singapore", "JP": "Japan",
}

func countryName(code string) string {
	if name, ok := countryNames[code]; ok {
		return name
	}
	return code
}
