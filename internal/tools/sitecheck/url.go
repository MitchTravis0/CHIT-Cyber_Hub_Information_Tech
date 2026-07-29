package sitecheck

import (
	"net/url"
	"strings"

	"chit/internal/core"
)

// NormalizeURL turns what a tech typed into a URL this tool can request. A
// missing scheme becomes https because that is what a browser tries first.
func NormalizeURL(raw string) (string, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return "", core.Errorf(core.CodeInvalidInput,
			"Enter an address to check, for example https://example.com.")
	}
	if !strings.Contains(text, "://") {
		text = "https://" + text
	}
	u, err := url.Parse(text)
	if err != nil {
		return "", notAnAddress(raw)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", notAnAddress(raw)
	}
	if u.Host == "" {
		return "", notAnAddress(raw)
	}
	u.Scheme = scheme
	u.Host = strings.ToLower(u.Host)
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String(), nil
}

func notAnAddress(raw string) error {
	return core.Errorf(core.CodeInvalidInput,
		"\"%s\" is not an address this can check. Use something like https://example.com or 192.168.1.10:8080.",
		strings.TrimSpace(raw))
}
