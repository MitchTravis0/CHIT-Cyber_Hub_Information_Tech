// Package qrgen turns a Wi-Fi network or a piece of text into a QR code matrix.
// The encoder is written from scratch here, in byte mode at versions 1 to 10,
// because there is no QR library in this project and none may be added. The
// frontend draws the matrix; nothing about a request is kept anywhere.
package qrgen

import (
	"strings"

	"chit/internal/core"
)

const (
	modeWifi = "wifi"
	modeText = "text"

	securityNone = "nopass"

	// maxSSIDBytes is the 802.11 limit on a network name.
	maxSSIDBytes = 32

	// quietZone is the light margin, in modules, the frontend must draw around
	// the code. A camera cannot find the pattern without it.
	quietZone = 4
)

// securities are the four values the T: field may take.
var securities = [4]string{"WPA", "SAE", "WEP", securityNone}

// Params is one request from the UI. Every zero value means "use the default",
// so a request with only Mode and the mode's own fields filled in is valid.
type Params struct {
	// Mode is "wifi" or "text". Empty means "wifi".
	Mode string `json:"mode"`
	// Text is used in text mode and ignored in wifi mode.
	Text string `json:"text"`
	// SSID is the network name, exactly as the router advertises it.
	SSID string `json:"ssid"`
	// Password is ignored when Security is "nopass".
	Password string `json:"password"`
	// Security is "WPA", "SAE", "WEP" or "nopass". Empty means "WPA".
	Security string `json:"security"`
	// Hidden marks a network that does not broadcast its name.
	Hidden bool `json:"hidden"`
	// ECLevel is "L", "M", "Q" or "H". Empty means "M".
	ECLevel string `json:"ecLevel"`
}

// Code is a finished QR code. Modules is the matrix the frontend draws.
type Code struct {
	// Size is the width and height of the matrix in modules, 4*Version+17.
	Size int `json:"size"`
	// Version is 1 to 10.
	Version int `json:"version"`
	// ECLevel is "L", "M", "Q" or "H" as actually used.
	ECLevel string `json:"ecLevel"`
	// Mask is the data mask pattern chosen, 0 to 7. Shown for diagnosis only.
	Mask int `json:"mask"`
	// Modules is Size*Size entries, row-major: index row*Size+col. True is a
	// dark module. Flat rather than nested so a nil inner slice cannot reach
	// the UI as JSON null.
	Modules []bool `json:"modules"`
	// Quiet is the quiet zone width in modules the frontend must draw. Always 4.
	Quiet int `json:"quiet"`
	// Payload is the exact string that was encoded, so the UI can show it.
	Payload string `json:"payload"`
	// PayloadBytes is len([]byte(Payload)).
	PayloadBytes int `json:"payloadBytes"`
	// Capacity is the most bytes this version and level could have held, so the
	// UI can say how close to the limit the code is.
	Capacity int `json:"capacity"`
}

// Generate validates the request, builds the payload, and encodes it as the
// smallest QR code that will hold it at the chosen error-correction level.
func Generate(p Params) (Code, error) {
	mode := p.Mode
	if mode == "" {
		mode = modeWifi
	}
	if mode != modeWifi && mode != modeText {
		return Code{}, core.Errorf(core.CodeInvalidInput,
			`"%s" is not a kind of QR code. Use "wifi" for a network or "text" for anything else.`, p.Mode)
	}

	security := p.Security
	if security == "" {
		security = securities[0]
	}
	if !known(securities[:], security) {
		return Code{}, core.Errorf(core.CodeInvalidInput,
			`"%s" is not a Wi-Fi security setting. Use WPA, SAE, WEP or nopass.`, p.Security)
	}

	ecLevel := p.ECLevel
	if ecLevel == "" {
		ecLevel = "M"
	}
	level, ok := levelIndex(ecLevel)
	if !ok {
		return Code{}, core.Errorf(core.CodeInvalidInput,
			`"%s" is not an error-correction setting. Use L, M, Q or H.`, p.ECLevel)
	}

	payload, err := payloadFor(mode, security, p)
	if err != nil {
		return Code{}, err
	}

	n := len(payload)
	version := 0
	for v := 1; v <= maxVersion; v++ {
		if byteCapacity[v-1][level] >= n {
			version = v
			break
		}
	}
	if version == 0 {
		return Code{}, tooLongError(mode, n, byteCapacity[maxVersion-1][level], ecLevel)
	}

	return encode(payload, version, level, ecLevel), nil
}

// payloadFor builds the string that goes in the code, and is the only place a
// user's own text is checked.
func payloadFor(mode, security string, p Params) (string, error) {
	if mode == modeText {
		return buildTextPayload(p.Text)
	}
	if strings.TrimSpace(p.SSID) == "" {
		return "", core.Errorf(core.CodeInvalidInput,
			"Type the network name exactly as it appears on the router, capital letters and all.")
	}
	if len(p.SSID) > maxSSIDBytes {
		return "", core.Errorf(core.CodeInvalidInput,
			"A Wi-Fi network name can be at most %d characters. That one comes to %d.",
			maxSSIDBytes, len(p.SSID))
	}
	return buildWifiPayload(p.SSID, p.Password, security, p.Hidden), nil
}

// tooLongError explains the byte limit in the terms of whichever mode the tech
// is working in.
func tooLongError(mode string, n, capacity int, ecLevel string) error {
	if mode == modeText {
		return core.Errorf(core.CodeInvalidInput,
			"That comes to %d bytes and the largest QR code CHIT makes holds %d at the %s setting. Shorten the text, or choose a lower error-correction setting: L holds the most.",
			n, capacity, ecLevel)
	}
	return core.Errorf(core.CodeInvalidInput,
		"The network name and password together come to %d bytes, and the largest QR code CHIT makes holds %d at the %s setting. Choose a lower error-correction setting (L holds the most), or use a shorter password.",
		n, capacity, ecLevel)
}

// encode runs the whole pipeline for a payload already known to fit. Nothing
// here can fail: the version was chosen because the payload fits and every
// table lookup is in range.
func encode(payload string, version, level int, ecLevel string) Code {
	codewords := interleave(buildDataCodewords(payload, version, level), version, level)

	size := sizeOf(version)
	m := newMatrix(size)
	drawFunctionPatterns(m, version)
	placeData(m, codewords)
	mask := chooseMask(m, level)

	return Code{
		Size:         size,
		Version:      version,
		ECLevel:      ecLevel,
		Mask:         mask,
		Modules:      m.modules,
		Quiet:        quietZone,
		Payload:      payload,
		PayloadBytes: len(payload),
		Capacity:     byteCapacity[version-1][level],
	}
}

func known(allowed []string, value string) bool {
	for _, name := range allowed {
		if name == value {
			return true
		}
	}
	return false
}
