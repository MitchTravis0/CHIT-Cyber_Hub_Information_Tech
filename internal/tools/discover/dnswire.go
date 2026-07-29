package discover

import (
	"encoding/binary"
	"errors"
	"strings"
)

// A minimal DNS message reader, enough for the mDNS responses this tool asks
// for. Go's standard library has no exported DNS wire parser and
// golang.org/x/net/dns/dnsmessage is an indirect dependency of Wails that CHIT
// must not make direct, so this is written out.
//
// Everything here is defensive: the packets arrive from anything on the local
// network, so a malformed or hostile one must produce an error, never a panic
// and never a loop.

// Record types, from the IANA registry. Only the ones this tool reads.
const (
	typeA    = 1
	typePTR  = 12
	typeTXT  = 16
	typeAAAA = 28
	typeSRV  = 33
)

const (
	headerBytes = 12
	// maxLabelBytes and maxNameBytes are the DNS limits. A name past either is
	// malformed.
	maxLabelBytes = 63
	maxNameBytes  = 255
	// maxPointers bounds a compression chain. Any legal name needs far fewer,
	// and the bound is what stops a pointer loop hanging the parser.
	maxPointers = 32
)

var (
	errShort         = errors.New("message is shorter than it claims")
	errBadName       = errors.New("name is malformed")
	errPointerLoop   = errors.New("name compression pointer loops or goes forwards")
	errBadRecordData = errors.New("record data is malformed")
)

// resource is one parsed record.
type resource struct {
	name  string
	rtype uint16
	// data is the raw rdata, sliced out of the message.
	data []byte
	// off is where data starts in the message, needed because a name inside
	// rdata may compress against anything earlier in the message.
	off int
}

// message is a parsed DNS message. Only the record sections matter here: mDNS
// responders put the useful records in answers and additionals, and which one
// they choose varies by implementation, so both are read into one list.
type message struct {
	records []resource
}

// decodeMessage reads a whole DNS message. A truncated or malformed message
// yields an error and whatever was read before the problem, so a partially
// readable packet still contributes what it had.
func decodeMessage(b []byte) (message, error) {
	var msg message
	if len(b) < headerBytes {
		return msg, errShort
	}

	counts := [4]int{
		int(binary.BigEndian.Uint16(b[4:])),  // questions
		int(binary.BigEndian.Uint16(b[6:])),  // answers
		int(binary.BigEndian.Uint16(b[8:])),  // authority
		int(binary.BigEndian.Uint16(b[10:])), // additional
	}

	off := headerBytes
	for i := 0; i < counts[0]; i++ {
		_, next, err := decodeName(b, off)
		if err != nil {
			return msg, err
		}
		// A question is the name plus type and class.
		if next+4 > len(b) {
			return msg, errShort
		}
		off = next + 4
	}

	for section := 1; section < 4; section++ {
		for i := 0; i < counts[section]; i++ {
			name, next, err := decodeName(b, off)
			if err != nil {
				return msg, err
			}
			// type, class, ttl, rdlength
			if next+10 > len(b) {
				return msg, errShort
			}
			rtype := binary.BigEndian.Uint16(b[next:])
			length := int(binary.BigEndian.Uint16(b[next+8:]))
			start := next + 10
			if start+length > len(b) {
				return msg, errShort
			}
			msg.records = append(msg.records, resource{
				name:  name,
				rtype: rtype,
				data:  b[start : start+length],
				off:   start,
			})
			off = start + length
		}
	}
	return msg, nil
}

// decodeName reads a domain name at off, following compression pointers, and
// returns the name and the offset just past the name as it was written (which
// is not where a pointer led).
func decodeName(b []byte, off int) (string, int, error) {
	var labels []string
	total := 0
	pointers := 0
	next := -1
	limit := off

	for {
		if limit >= len(b) {
			return "", 0, errShort
		}
		length := int(b[limit])

		switch {
		case length == 0:
			if next < 0 {
				next = limit + 1
			}
			return strings.Join(labels, "."), next, nil

		case length&0xC0 == 0xC0:
			// A two byte pointer to somewhere earlier in the message.
			if limit+1 >= len(b) {
				return "", 0, errShort
			}
			target := int(binary.BigEndian.Uint16(b[limit:]) & 0x3FFF)
			if next < 0 {
				next = limit + 2
			}
			// Only a backwards pointer is legal. Forwards or self-referential
			// pointers are how a hostile packet tries to hang a parser.
			if target >= limit {
				return "", 0, errPointerLoop
			}
			pointers++
			if pointers > maxPointers {
				return "", 0, errPointerLoop
			}
			limit = target

		default:
			// This also covers the two reserved label types (top bits 01 and
			// 10). Every byte they use is in 64..191, and all of those are
			// already longer than a label may be, so a separate branch for them
			// would be code no input can reach.
			if length > maxLabelBytes {
				return "", 0, errBadName
			}
			if limit+1+length > len(b) {
				return "", 0, errShort
			}
			total += length + 1
			if total > maxNameBytes {
				return "", 0, errBadName
			}
			labels = append(labels, string(b[limit+1:limit+1+length]))
			limit += 1 + length
		}
	}
}

// encodeName writes a name in wire form. Used only for the questions this tool
// asks, which are constants in this package.
func encodeName(name string) ([]byte, error) {
	out := make([]byte, 0, len(name)+2)
	total := 0
	for _, label := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		if label == "" {
			continue
		}
		if len(label) > maxLabelBytes {
			return nil, errBadName
		}
		total += len(label) + 1
		if total > maxNameBytes {
			return nil, errBadName
		}
		out = append(out, byte(len(label)))
		out = append(out, label...)
	}
	return append(out, 0), nil
}

// srv is the contents of an SRV record.
type srv struct {
	port   int
	target string
}

func parseSRV(msg []byte, r resource) (srv, error) {
	if len(r.data) < 6 {
		return srv{}, errBadRecordData
	}
	// The target is a name and may compress against the whole message, so it is
	// decoded from the message rather than from the record data alone.
	target, _, err := decodeName(msg, r.off+6)
	if err != nil {
		return srv{}, err
	}
	return srv{port: int(binary.BigEndian.Uint16(r.data[4:])), target: target}, nil
}

// parseTXT reads the length-prefixed strings in a TXT record.
func parseTXT(data []byte) ([]string, error) {
	var out []string
	for i := 0; i < len(data); {
		length := int(data[i])
		if i+1+length > len(data) {
			return out, errBadRecordData
		}
		if length > 0 {
			out = append(out, string(data[i+1:i+1+length]))
		}
		i += 1 + length
	}
	return out, nil
}

func parsePTR(msg []byte, r resource) (string, error) {
	name, _, err := decodeName(msg, r.off)
	return name, err
}
