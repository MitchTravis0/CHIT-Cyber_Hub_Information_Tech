package totp

import (
	"net/url"
	"strconv"
	"strings"

	"chit/internal/core"
)

// parsedAccount is everything an otpauth link, or a hand-typed secret, carries.
type parsedAccount struct {
	Issuer    string
	Label     string
	Secret    string
	Digits    int
	Period    int
	Algorithm string
}

// parseURI reads an otpauth:// link, the text every service shows behind its
// "can't scan the code?" link. Anything the link leaves out gets the default
// every authenticator uses.
func parseURI(raw string) (parsedAccount, error) {
	var out parsedAccount

	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(u.Scheme, "otpauth") {
		return out, core.Errorf(core.CodeInvalidInput, msgNotOtpauth)
	}
	switch strings.ToLower(u.Host) {
	case "totp":
	case "hotp":
		return out, core.Errorf(core.CodeInvalidInput, msgHOTP)
	default:
		return out, core.Errorf(core.CodeInvalidInput, msgNotOtpauth)
	}

	query := u.Query()
	out.Secret = strings.TrimSpace(query.Get("secret"))
	if out.Secret == "" {
		return out, core.Errorf(core.CodeInvalidInput, msgNoSecretInURI)
	}

	// The path is "/Issuer:account". The issuer query parameter wins when both
	// are present, because the label prefix is only a convention.
	name := strings.TrimPrefix(u.Path, "/")
	out.Label = strings.TrimSpace(name)
	if colon := strings.Index(name, ":"); colon >= 0 {
		out.Issuer = strings.TrimSpace(name[:colon])
		out.Label = strings.TrimSpace(name[colon+1:])
	}
	if fromQuery := strings.TrimSpace(query.Get("issuer")); fromQuery != "" {
		out.Issuer = fromQuery
	}

	out.Digits, err = numberIn(query.Get("digits"), defaultDigits, minDigits, maxDigits, msgDigits)
	if err != nil {
		return parsedAccount{}, err
	}
	out.Period, err = numberIn(query.Get("period"), defaultPeriod, minPeriod, maxPeriod, msgPeriod)
	if err != nil {
		return parsedAccount{}, err
	}
	out.Algorithm, err = normalizeAlgorithm(query.Get("algorithm"))
	if err != nil {
		return parsedAccount{}, err
	}
	return out, nil
}

// numberIn reads an optional numeric link parameter. An empty one takes the
// default; anything else has to land inside the range, and the message quotes
// what was actually written rather than a number it was not.
func numberIn(text string, fallback, low, high int, message string) (int, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(trimmed)
	if err != nil || n < low || n > high {
		return 0, core.Errorf(core.CodeInvalidInput, message, trimmed)
	}
	return n, nil
}
