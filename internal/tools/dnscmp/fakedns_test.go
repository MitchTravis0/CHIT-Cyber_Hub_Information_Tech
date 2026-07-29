package dnscmp

import (
	"encoding/binary"
	"net"
	"strings"
	"sync"
	"testing"
)

// A minimal DNS responder, test-only. It writes just enough of the wire format
// to answer the six record types this tool asks for.
//
// The Phase 8 Device Discovery tool has a real DNS message parser in
// internal/tools/discover. It is deliberately not imported here: that one reads
// responses and this needs to write them, it belongs to another tool, and the
// fifteen lines below are cheaper than the coupling.

const (
	typeA     = 1
	typeCNAME = 5
	typeMX    = 15
	typeTXT   = 16
	typeNS    = 2
	typeAAAA  = 28
)

// record is one answer the fake will give for one query type. owner is the name
// the record belongs to; empty means the name that was asked about, which is
// written as a compression pointer the way a real server does.
type record struct {
	typ   uint16
	owner string
	data  []byte
}

// ownedBy is used after a CNAME: the address records in the reply belong to the
// alias target, not to the name that was asked about, and Go's resolver will not
// follow the chain unless they say so.
func (r record) ownedBy(name string) record {
	r.owner = name
	return r
}

func aRecord(ip string) record {
	return record{typ: typeA, data: net.ParseIP(ip).To4()}
}

func aaaaRecord(ip string) record {
	return record{typ: typeAAAA, data: net.ParseIP(ip).To16()}
}

func nameRecord(typ uint16, name string) record {
	return record{typ: typ, data: encodeName(name)}
}

func mxRecord(pref uint16, host string) record {
	data := make([]byte, 2)
	binary.BigEndian.PutUint16(data, pref)
	return record{typ: typeMX, data: append(data, encodeName(host)...)}
}

func txtRecord(text string) record {
	return record{typ: typeTXT, data: append([]byte{byte(len(text))}, text...)}
}

// nullMXRecord is the RFC 7505 "this domain accepts no email": preference 0 and
// the root as the host.
func nullMXRecord() record {
	return record{typ: typeMX, data: []byte{0, 0, 0}}
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
	conn *net.UDPConn
	mu   sync.Mutex
	// answers maps a query type to the records to return. A type that is absent
	// gets an empty NOERROR answer, which is how a real server says "no record
	// of that type".
	answers map[uint16][]record
	asked   int
	// cname, when set, is the alias this name carries. A real server puts the
	// CNAME record into the A and AAAA answers as well as the CNAME answer, and
	// Go's LookupCNAME reads the canonical name out of the address lookup, so a
	// fake that only answers the CNAME query is not realistic enough to test
	// against.
	cname string
	// silent makes the server read the query and never reply.
	silent bool
}

func startFakeDNS(t *testing.T, answers map[uint16][]record) *fakeDNS {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	f := &fakeDNS{conn: conn, answers: answers}
	go f.serve()
	t.Cleanup(func() { conn.Close() })
	return f
}

// startAliasDNS is a fake for a name that is an alias for another. The CNAME
// record appears in the address answers as well, the way a real server sends it.
func startAliasDNS(t *testing.T, cname string, answers map[uint16][]record) *fakeDNS {
	t.Helper()
	f := startFakeDNS(t, answers)
	f.mu.Lock()
	f.cname = cname
	f.mu.Unlock()
	return f
}

func startSilentDNS(t *testing.T) *fakeDNS {
	t.Helper()
	f := startFakeDNS(t, nil)
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
	buf := make([]byte, 512)
	for {
		n, from, err := f.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		f.mu.Lock()
		f.asked++
		silent := f.silent
		answers := f.answers
		cname := f.cname
		f.mu.Unlock()
		if silent || n < 12 {
			continue
		}
		if reply := buildReply(buf[:n], answers, cname); reply != nil {
			_, _ = f.conn.WriteToUDP(reply, from)
		}
	}
}

// buildReply echoes the question and appends the records for its type, using a
// compression pointer back to the question name the way a real server does.
func buildReply(query []byte, answers map[uint16][]record, cname string) []byte {
	// Walk the question name to find where the type and class sit.
	i := 12
	for i < len(query) && query[i] != 0 {
		i += int(query[i]) + 1
	}
	if i+5 > len(query) {
		return nil
	}
	qEnd := i + 5
	qType := binary.BigEndian.Uint16(query[i+1:])

	out := make([]byte, 0, 512)
	out = append(out, query[:qEnd]...)
	binary.BigEndian.PutUint16(out[2:], 0x8180) // response, recursion available
	binary.BigEndian.PutUint16(out[6:], 0)      // answer count, filled in below
	// The query header is echoed wholesale, so these two have to be cleared.
	// This machine's resolv.conf carries "options edns0", which makes Go send an
	// OPT record and set ARCOUNT to 1; a reply claiming an additional record it
	// does not contain is malformed, and Go rejects it about half the time.
	binary.BigEndian.PutUint16(out[8:], 0)  // authority count
	binary.BigEndian.PutUint16(out[10:], 0) // additional count

	records := answers[qType]
	// A name that is an alias carries its CNAME record in the address answers as
	// well as in the CNAME answer, and the addresses that follow belong to the
	// target. Go's LookupCNAME reads the canonical name out of the address
	// lookup, so a fake that gets this wrong makes a correct tool look broken.
	if cname != "" && (qType == typeA || qType == typeAAAA) {
		owned := make([]record, 0, len(records)+1)
		owned = append(owned, nameRecord(typeCNAME, cname))
		for _, r := range records {
			owned = append(owned, r.ownedBy(cname))
		}
		records = owned
	}
	for _, r := range records {
		if r.owner == "" {
			out = append(out, 0xC0, 0x0C) // pointer to the question name at offset 12
		} else {
			out = append(out, encodeName(r.owner)...)
		}
		out = binary.BigEndian.AppendUint16(out, r.typ)
		out = binary.BigEndian.AppendUint16(out, 1)  // class IN
		out = binary.BigEndian.AppendUint32(out, 60) // ttl
		out = binary.BigEndian.AppendUint16(out, uint16(len(r.data)))
		out = append(out, r.data...)
	}
	binary.BigEndian.PutUint16(out[6:], uint16(len(records)))
	return out
}
