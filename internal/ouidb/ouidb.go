// Package ouidb answers "who made this device?" for a MAC address, using an
// embedded copy of the IEEE registry. It is read-only and safe for concurrent
// use; the database is decoded on first lookup, not at process start.
package ouidb

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"io"
	"strings"
	"sync"
)

//go:embed data/oui.txt.gz
var packed []byte

// Meta describes the embedded database so the UI can show how stale it is.
type Meta struct {
	Source  string `json:"source"`
	Fetched string `json:"fetched"`
	Records int    `json:"records"`
}

var (
	once sync.Once
	meta Meta
	// IEEE hands out 24-bit (MA-L), 28-bit (MA-M) and 36-bit (MA-S) blocks, so
	// the same OUI can be shared by many small vendors. Longest prefix wins.
	m24 map[uint32]string
	m28 map[uint32]string
	m36 map[uint64]string
)

func load() {
	zr, err := gzip.NewReader(bytes.NewReader(packed))
	if err != nil {
		panic("ouidb: embedded database is corrupt: " + err.Error())
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		panic("ouidb: embedded database is corrupt: " + err.Error())
	}

	// Vendor names are sliced out of this one string, so the maps hold no
	// per-entry allocations.
	blob := string(raw)

	m24 = make(map[uint32]string, 40000)
	m28 = make(map[uint32]string, 7000)
	m36 = make(map[uint64]string, 12000)

	for len(blob) > 0 {
		line := blob
		if nl := strings.IndexByte(blob, '\n'); nl >= 0 {
			line, blob = blob[:nl], blob[nl+1:]
		} else {
			blob = ""
		}
		if line == "" {
			continue
		}
		if line[0] == '#' {
			readMeta(line)
			continue
		}
		tab := strings.IndexByte(line, '\t')
		if tab < 0 {
			continue
		}
		key, ok := hexValue(line[:tab])
		if !ok {
			continue
		}
		vendor := line[tab+1:]
		switch tab {
		case 6:
			m24[uint32(key)] = vendor
		case 7:
			m28[uint32(key)] = vendor
		case 9:
			m36[key] = vendor
		}
	}
	meta.Records = len(m24) + len(m28) + len(m36)
}

func readMeta(line string) {
	switch {
	case strings.HasPrefix(line, "# source: "):
		meta.Source = line[len("# source: "):]
	case strings.HasPrefix(line, "# fetched: "):
		meta.Fetched = line[len("# fetched: "):]
	}
}

func hexValue(s string) (uint64, bool) {
	var v uint64
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			v = v<<4 | uint64(c-'0')
		case c >= 'A' && c <= 'F':
			v = v<<4 | uint64(c-'A'+10)
		case c >= 'a' && c <= 'f':
			v = v<<4 | uint64(c-'a'+10)
		default:
			return 0, false
		}
	}
	return v, true
}

// Metadata reports where the embedded database came from and when.
func Metadata() Meta {
	once.Do(load)
	return meta
}

// ParseMAC accepts "AA:BB:CC:DD:EE:FF", "aa-bb-cc-dd-ee-ff", "aabb.ccdd.eeff",
// bare hex and any mix of those separators. Only 48-bit addresses are accepted.
func ParseMAC(mac string) ([6]byte, bool) {
	var out [6]byte
	n := 0
	for i := 0; i < len(mac); i++ {
		c := mac[i]
		switch {
		case c == ':' || c == '-' || c == '.' || c == ' ' || c == '\t' || c == '\r' || c == '\n':
			continue
		case c >= '0' && c <= '9':
			c -= '0'
		case c >= 'A' && c <= 'F':
			c -= 'A' - 10
		case c >= 'a' && c <= 'f':
			c -= 'a' - 10
		default:
			return out, false
		}
		if n >= 12 {
			return out, false
		}
		if n%2 == 0 {
			out[n/2] = c << 4
		} else {
			out[n/2] |= c
		}
		n++
	}
	if n != 12 {
		return out, false
	}
	return out, true
}

const hexDigits = "0123456789ABCDEF"

// Normalize returns the canonical "AA:BB:CC:DD:EE:FF" form.
func Normalize(mac string) (string, bool) {
	b, ok := ParseMAC(mac)
	if !ok {
		return "", false
	}
	return format(b), true
}

func format(b [6]byte) string {
	var sb strings.Builder
	sb.Grow(17)
	for i, v := range b {
		if i > 0 {
			sb.WriteByte(':')
		}
		sb.WriteByte(hexDigits[v>>4])
		sb.WriteByte(hexDigits[v&0x0f])
	}
	return sb.String()
}

// LookupBytes is the hot path used by the scanner, which already has raw bytes.
func LookupBytes(b [6]byte) (string, bool) {
	once.Do(load)
	v := uint64(b[0])<<40 | uint64(b[1])<<32 | uint64(b[2])<<24 |
		uint64(b[3])<<16 | uint64(b[4])<<8 | uint64(b[5])
	if vendor, ok := m36[v>>12]; ok {
		return vendor, true
	}
	if vendor, ok := m28[uint32(v>>20)]; ok {
		return vendor, true
	}
	vendor, ok := m24[uint32(v>>24)]
	return vendor, ok
}

// Lookup returns the manufacturer registered for a MAC address. ok is false for
// an unparseable address and for one whose prefix is not in the registry
// (randomized phone addresses land here, see Describe).
func Lookup(mac string) (string, bool) {
	b, ok := ParseMAC(mac)
	if !ok {
		return "", false
	}
	return LookupBytes(b)
}
