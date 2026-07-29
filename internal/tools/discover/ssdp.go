package discover

import (
	"net/url"
	"strconv"
	"strings"
)

// ssdpSearch is the M-SEARCH request. The blank line at the end is part of the
// request and a responder ignores one without it.
//
// MX is the number of seconds a responder may wait before answering, so it must
// stay below the listening window.
const ssdpSearch = "M-SEARCH * HTTP/1.1\r\n" +
	"HOST: 239.255.255.250:1900\r\n" +
	"MAN: \"ssdp:discover\"\r\n" +
	"MX: 2\r\n" +
	"ST: ssdp:all\r\n" +
	"\r\n"

// devicesFromSSDP reads one SSDP reply. Both a search response and an
// unsolicited NOTIFY announcement are accepted: a device that announces itself
// while CHIT happens to be listening is just as useful as one that answers.
func devicesFromSSDP(payload []byte, src, adapter string) []Device {
	text := string(payload)
	line, rest, ok := strings.Cut(text, "\n")
	if !ok {
		return nil
	}
	start := strings.ToUpper(strings.TrimSpace(line))
	if !strings.HasPrefix(start, "HTTP/1.1 200") && !strings.HasPrefix(start, "NOTIFY ") {
		return nil
	}

	headers := map[string]string{}
	for _, raw := range strings.Split(rest, "\n") {
		key, value, ok := strings.Cut(raw, ":")
		if !ok {
			continue
		}
		headers[strings.ToUpper(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}

	host, port := hostAndPort(headers["LOCATION"])
	service := headers["ST"]
	if service == "" {
		service = headers["NT"]
	}

	return []Device{newDevice(Device{
		Protocol: ProtocolSSDP,
		IP:       src,
		Name:     nameFromUSN(headers["USN"]),
		Service:  service,
		Host:     host,
		Port:     port,
		Details:  headers["SERVER"],
		Adapter:  adapter,
	})}
}

// hostAndPort reads the LOCATION header, which is the URL of the device's own
// description document. CHIT never fetches it: this tool only listens to what
// devices volunteer.
func hostAndPort(location string) (string, int) {
	if location == "" {
		return "", 0
	}
	u, err := url.Parse(location)
	if err != nil || u.Hostname() == "" {
		return "", 0
	}
	if p := u.Port(); p != "" {
		port, err := strconv.Atoi(p)
		if err != nil {
			return u.Hostname(), 0
		}
		return u.Hostname(), port
	}
	if u.Scheme == "https" {
		return u.Hostname(), 443
	}
	return u.Hostname(), 80
}

// nameFromUSN pulls a readable name out of the unique service name where there
// is one. A USN is normally "uuid:<guid>::<type>", which has no name in it, so
// this is empty far more often than not and the UI says so.
func nameFromUSN(usn string) string {
	if usn == "" {
		return ""
	}
	name, _, _ := strings.Cut(usn, "::")
	if strings.HasPrefix(strings.ToLower(name), "uuid:") {
		return ""
	}
	return strings.TrimSpace(name)
}
