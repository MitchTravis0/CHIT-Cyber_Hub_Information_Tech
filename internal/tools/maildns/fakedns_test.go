package maildns

import (
	"encoding/binary"
	"net"
	"strings"
	"sync"
	"testing"
)

// A minimal DNS responder, test-only, keyed by name so one fake can serve the
// domain, its _dmarc record and its _domainkey selectors at once.
//
// internal/tools/dnscmp has a similar test-only responder keyed by record type.
// Neither imports the other: they are test scaffolding in two packages with two
// different shapes, and a shared test helper would have to live in a non-test
// package that ships in the binary.

const (
	typeMX  = 15
	typeTXT = 16
)

type record struct {
	typ  uint16
	data []byte
}

// zone maps a fully qualified name (no trailing dot) to its records by type.
type zone map[string]map[uint16][]record

func mxRecord(pref uint16, host string) record {
	data := make([]byte, 2)
	binary.BigEndian.PutUint16(data, pref)
	return record{typ: typeMX, data: append(data, encodeName(host)...)}
}

// nullMXRecord is the RFC 7505 "this domain accepts no email": preference 0 and
// the root as the host.
func nullMXRecord() record {
	return record{typ: typeMX, data: []byte{0, 0, 0}}
}

// txtRecord splits at 255 bytes, which is the wire limit for one TXT string and
// is how a long SPF or DKIM record really arrives.
func txtRecord(text string) record {
	data := make([]byte, 0, len(text)+4)
	for len(text) > 255 {
		data = append(data, 255)
		data = append(data, text[:255]...)
		text = text[255:]
	}
	data = append(data, byte(len(text)))
	return record{typ: typeTXT, data: append(data, text...)}
}

func encodeName(name string) []byte {
	out := make([]byte, 0, len(name)+2)
	for _, label := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		if label == "" {
			continue
		}
		out = append(out, byte(len(label)))
		out = append(out, label...)
	}
	return append(out, 0)
}

type fakeDNS struct {
	conn   *net.UDPConn
	mu     sync.Mutex
	zone   zone
	asked  int
	silent bool
}

func startFakeDNS(t *testing.T, z zone) *fakeDNS {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	f := &fakeDNS{conn: conn, zone: z}
	go f.serve()
	t.Cleanup(func() { conn.Close() })
	return f
}

func startSilentDNS(t *testing.T) *fakeDNS {
	t.Helper()
	f := startFakeDNS(t, zone{})
	f.mu.Lock()
	f.silent = true
	f.mu.Unlock()
	return f
}

func (f *fakeDNS) addr() string { return f.conn.LocalAddr().String() }

func (f *fakeDNS) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.asked
}

func (f *fakeDNS) serve() {
	buf := make([]byte, 1024)
	for {
		n, from, err := f.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		f.mu.Lock()
		f.asked++
		silent := f.silent
		z := f.zone
		f.mu.Unlock()
		if silent || n < 12 {
			continue
		}
		if reply := buildReply(buf[:n], z); reply != nil {
			_, _ = f.conn.WriteToUDP(reply, from)
		}
	}
}

// buildReply echoes the question and appends the records for that name and type.
// A name the zone does not hold gets NXDOMAIN, which is what a real server sends
// and what separates "no such name" from "no record of that type".
func buildReply(query []byte, z zone) []byte {
	name, qEnd, qType, ok := readQuestion(query)
	if !ok {
		return nil
	}

	out := make([]byte, 0, 1024)
	out = append(out, query[:qEnd]...)
	binary.BigEndian.PutUint16(out[2:], 0x8180) // response, recursion available
	binary.BigEndian.PutUint16(out[6:], 0)
	// The query header is echoed wholesale, so these two have to be cleared.
	// This machine's resolv.conf carries "options edns0", which makes Go send an
	// OPT record and set ARCOUNT to 1; a reply claiming an additional record it
	// does not contain is malformed and Go rejects it.
	binary.BigEndian.PutUint16(out[8:], 0)
	binary.BigEndian.PutUint16(out[10:], 0)

	byType, known := z[name]
	if !known {
		binary.BigEndian.PutUint16(out[2:], 0x8183) // NXDOMAIN
		return out
	}

	records := byType[qType]
	for _, r := range records {
		out = append(out, 0xC0, 0x0C) // pointer to the question name at offset 12
		out = binary.BigEndian.AppendUint16(out, r.typ)
		out = binary.BigEndian.AppendUint16(out, 1)  // class IN
		out = binary.BigEndian.AppendUint32(out, 60) // ttl
		out = binary.BigEndian.AppendUint16(out, uint16(len(r.data)))
		out = append(out, r.data...)
	}
	binary.BigEndian.PutUint16(out[6:], uint16(len(records)))
	return out
}

func readQuestion(query []byte) (name string, end int, qType uint16, ok bool) {
	var labels []string
	i := 12
	for i < len(query) && query[i] != 0 {
		length := int(query[i])
		if i+1+length > len(query) {
			return "", 0, 0, false
		}
		labels = append(labels, string(query[i+1:i+1+length]))
		i += length + 1
	}
	if i+5 > len(query) {
		return "", 0, 0, false
	}
	return strings.ToLower(strings.Join(labels, ".")), i + 5, binary.BigEndian.Uint16(query[i+1:]), true
}
