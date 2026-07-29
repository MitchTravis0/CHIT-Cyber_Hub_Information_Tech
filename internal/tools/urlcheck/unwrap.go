package urlcheck

import (
	"net/url"
	"strings"
)

// maxUnwraps stops a URL that has been crafted to wrap itself. Two layers is
// already unusual in real mail.
const maxUnwraps = 5

// unwrapAll strips known mail-security and search-engine wrappers off a URL,
// repeatedly, because one wrapper can contain another. It makes no network
// calls. notes carries the informational findings that detection produced (the
// Proofpoint v3 and Mimecast cases), which cannot be unwrapped. It is named
// unwrapAll rather than Unwrap because Unwrap is the type of one of its steps.
func unwrapAll(raw string) (out string, steps []Unwrap, notes []Finding) {
	steps = []Unwrap{}
	current := raw

	for i := 0; i < maxUnwraps; i++ {
		u, err := url.Parse(current)
		if err != nil {
			return current, steps, notes
		}
		inner, wrapper, note := unwrapOnce(u)
		if note != nil {
			notes = append(notes, *note)
		}
		if wrapper == "" {
			return current, steps, notes
		}
		// An attacker chooses what sits in that parameter, so anything that is
		// not an absolute web address is left alone rather than followed.
		if !absoluteWeb(inner) {
			return current, steps, notes
		}
		next, err := NormalizeInput(inner)
		if err != nil {
			return current, steps, notes
		}
		steps = append(steps, Unwrap{Wrapper: wrapper, From: current, To: next})
		current = next
	}
	return current, steps, notes
}

// unwrapOnce applies the first wrapper rule that matches u. An empty wrapper
// means nothing matched, or that what matched cannot be decoded at all, in
// which case note explains why.
func unwrapOnce(u *url.URL) (inner, wrapper string, note *Finding) {
	host := cleanHost(u.Hostname())
	path := u.Path

	switch {
	case host == "safelinks.protection.outlook.com" || strings.HasSuffix(host, ".safelinks.protection.outlook.com"):
		// Query().Get percent-decodes once, which is exactly the one layer that
		// Safe Links applies.
		return u.Query().Get("url"), "Microsoft Defender Safe Links", nil

	case host == "urldefense.proofpoint.com" || host == "urldefense.com":
		if strings.HasPrefix(path, "/v2/url") {
			value := u.Query().Get("u")
			if value == "" {
				return "", "", nil
			}
			decoded, err := url.QueryUnescape(strings.NewReplacer("-", "%", "_", "/").Replace(value))
			if err != nil {
				return "", "", nil
			}
			return decoded, "Proofpoint URL Defense", nil
		}
		if strings.HasPrefix(path, "/v3/") {
			finding := findingURLDefenseV3()
			return "", "", &finding
		}
		return "", "", nil

	case isGoogleRedirect(host, path):
		value := u.Query().Get("q")
		if value == "" {
			value = u.Query().Get("url")
		}
		return value, "Google redirect", nil

	case strings.HasSuffix(host, ".mimecast.com"):
		finding := findingMimecast()
		return "", "", &finding

	case host == "linkprotect.cudasvc.com" && path == "/url":
		return u.Query().Get("a"), "Barracuda Link Protection", nil
	}
	return "", "", nil
}

// isGoogleRedirect matches google.com and the country variants, which are
// google plus one or two more labels, on their /url redirector only.
func isGoogleRedirect(host, path string) bool {
	if path != "/url" {
		return false
	}
	labels := strings.Split(strings.TrimPrefix(host, "www."), ".")
	return len(labels) >= 2 && len(labels) <= 3 && labels[0] == "google"
}

// absoluteWeb reports whether value is an address that can actually be
// followed, so a relative or javascript: parameter is never unwrapped into.
func absoluteWeb(value string) bool {
	if value == "" {
		return false
	}
	u, err := url.Parse(value)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
