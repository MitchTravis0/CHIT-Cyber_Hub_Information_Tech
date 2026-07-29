package discover

import (
	"encoding/binary"
	"encoding/hex"
	"reflect"
	"strings"
	"testing"
)

// Every fixture below was built by a python3 script using struct and manual
// label encoding, not by this package's own encoder. An encoder and a decoder
// written by the same hand can be self-consistently wrong; these cannot agree
// with a mistake they do not share.
const (
	// A realistic IPP printer response: PTR, SRV, TXT and A, with the instance
	// name written out in full and the record owners as real names.
	mdnsPrinterHex = "000084000000000400000000045f697070045f746370056c6f63616c00000c00010000007800241242726f7468657220484c2d4c323335304457045f697070045f746370056c6f63616c001242726f7468657220484c2d4c323335304457045f697070045f746370056c6f63616c000021000100000078001d0000000002770f425257303031313232333334343535056c6f63616c001242726f7468657220484c2d4c323335304457045f697070045f746370056c6f63616c000010000100000078004b1574793d42726f7468657220484c2d4c3233353044572370726f647563743d2842726f7468657220484c2d4c32333530445720736572696573291072703d64756572717865737a353039300f425257303031313232333334343535056c6f63616c0000010001000000780004c0a8013f"
	// An AirPlay response whose SRV target is a compression pointer back to the
	// A record's owner, which is what real responders send.
	mdnsPointerHex = "000084000000000200000000086170706c652d7476056c6f63616c00000100010000007800040a0000070b4c6976696e6720526f6f6d085f616972706c6179045f746370056c6f63616c0000210001000000780008000000001b58c00c"
	// A name whose pointer points at itself: the shape used to hang a parser.
	mdnsLoopHex = "000084000000000100000000c00c0001000100000078000401020304"
	// A pointer that points forwards past the end of the message.
	mdnsForwardHex = "000084000000000100000000c0280001000100000078000401020304"
	// A pointer that points forwards to an offset that is INSIDE the message.
	// The one above is rejected by a bounds check alone, so on its own it does
	// not prove that forward pointers are refused.
	mdnsForwardInsideHex = "000084000000000100000000c01e00010001000000780004010203040000056c61746572056c6f63616c00"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad fixture: %v", err)
	}
	return b
}

func TestEncodeName(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantHex string
		wantErr bool
	}{
		// The expected bytes came out of the python script, not out of this
		// function.
		{"a service type", "_ipp._tcp.local.", "045f697070045f746370056c6f63616c00", false},
		{"without the trailing dot", "_ipp._tcp.local", "045f697070045f746370056c6f63616c00", false},
		{"the root", ".", "00", false},
		{"empty", "", "00", false},
		{"a label of 63", strings.Repeat("a", 63) + ".local.", "", false},
		{"a label of 64", strings.Repeat("a", 64) + ".local.", "", true},
		{"a name over 255 bytes", strings.TrimSuffix(strings.Repeat(strings.Repeat("a", 63)+".", 5), ".") + ".", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := encodeName(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("encodeName(%q) = %x, want an error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("encodeName(%q) errored: %v", tt.in, err)
			}
			if tt.wantHex != "" && hex.EncodeToString(got) != tt.wantHex {
				t.Errorf("encodeName(%q) = %x, want %s", tt.in, got, tt.wantHex)
			}
		})
	}
}

func TestDecodeName(t *testing.T) {
	printer := mustHex(t, mdnsPrinterHex)

	t.Run("a plain name", func(t *testing.T) {
		got, next, err := decodeName(printer, headerBytes)
		if err != nil {
			t.Fatalf("decodeName errored: %v", err)
		}
		if got != "_ipp._tcp.local" {
			t.Errorf("name = %q, want _ipp._tcp.local", got)
		}
		if next != headerBytes+17 {
			t.Errorf("next = %d, want %d", next, headerBytes+17)
		}
	})

	t.Run("a compression pointer", func(t *testing.T) {
		// The AirPlay fixture's SRV target is a pointer back to the A record's
		// owner at offset 12.
		pointer := mustHex(t, mdnsPointerHex)
		got, _, err := decodeName(pointer, len(pointer)-2)
		if err != nil {
			t.Fatalf("decodeName errored: %v", err)
		}
		if got != "apple-tv.local" {
			t.Errorf("name = %q, want apple-tv.local", got)
		}
	})

	t.Run("a pointer that points at itself is refused", func(t *testing.T) {
		loop := mustHex(t, mdnsLoopHex)
		if _, _, err := decodeName(loop, headerBytes); err != errPointerLoop {
			t.Errorf("err = %v, want %v", err, errPointerLoop)
		}
	})

	t.Run("a forward pointer is refused", func(t *testing.T) {
		forward := mustHex(t, mdnsForwardHex)
		if _, _, err := decodeName(forward, headerBytes); err != errPointerLoop {
			t.Errorf("err = %v, want %v", err, errPointerLoop)
		}
	})

	t.Run("a forward pointer inside the message is refused", func(t *testing.T) {
		inside := mustHex(t, mdnsForwardInsideHex)
		target := 30
		if target >= len(inside) {
			t.Fatalf("the fixture's target %d is not inside a %d byte message", target, len(inside))
		}
		if _, _, err := decodeName(inside, headerBytes); err != errPointerLoop {
			t.Errorf("err = %v, want %v", err, errPointerLoop)
		}
	})

	t.Run("a long chain of pointers is refused", func(t *testing.T) {
		// Every hop is strictly backwards, so no single pointer is illegal and
		// only the bound on the chain length stops this. Without it a hostile
		// packet can make the parser walk thousands of hops per name.
		chain := longPointerChain(maxPointers + 8)
		if _, _, err := decodeName(chain.msg, chain.start); err != errPointerLoop {
			t.Errorf("err = %v, want %v for a chain of %d hops", err, errPointerLoop, maxPointers+8)
		}

		// A chain just inside the bound must still work, or the bound is simply
		// rejecting everything.
		short := longPointerChain(4)
		if got, _, err := decodeName(short.msg, short.start); err != nil || got != "end.local" {
			t.Errorf("a 4 hop chain gave (%q, %v), want end.local", got, err)
		}
	})

	t.Run("a truncated name is refused", func(t *testing.T) {
		if _, _, err := decodeName(printer[:headerBytes+4], headerBytes); err != errShort {
			t.Errorf("err = %v, want %v", err, errShort)
		}
	})

	t.Run("a reserved label type is refused", func(t *testing.T) {
		// Top bits 10 and 01 are reserved and have never been assigned.
		for _, top := range []byte{0x80, 0x40} {
			b := append([]byte{}, printer...)
			b[headerBytes] = top
			if _, _, err := decodeName(b, headerBytes); err != errBadName {
				t.Errorf("label type %#x: err = %v, want %v", top, err, errBadName)
			}
		}
	})

	t.Run("reading past the end is refused", func(t *testing.T) {
		if _, _, err := decodeName(printer, len(printer)); err != errShort {
			t.Errorf("err = %v, want %v", err, errShort)
		}
	})
}

func TestDecodeMessage(t *testing.T) {
	printer := mustHex(t, mdnsPrinterHex)
	msg, err := decodeMessage(printer)
	if err != nil {
		t.Fatalf("decodeMessage errored: %v", err)
	}
	if len(msg.records) != 4 {
		t.Fatalf("got %d records, want 4", len(msg.records))
	}

	wantTypes := []uint16{typePTR, typeSRV, typeTXT, typeA}
	for i, r := range msg.records {
		if r.rtype != wantTypes[i] {
			t.Errorf("record %d has type %d, want %d", i, r.rtype, wantTypes[i])
		}
	}

	if got := msg.records[0].name; got != "_ipp._tcp.local" {
		t.Errorf("PTR owner = %q", got)
	}
	instance, err := parsePTR(printer, msg.records[0])
	if err != nil {
		t.Fatalf("parsePTR errored: %v", err)
	}
	if instance != "Brother HL-L2350DW._ipp._tcp.local" {
		t.Errorf("PTR target = %q", instance)
	}

	service, err := parseSRV(printer, msg.records[1])
	if err != nil {
		t.Fatalf("parseSRV errored: %v", err)
	}
	if service.port != 631 {
		t.Errorf("SRV port = %d, want 631", service.port)
	}
	if service.target != "BRW001122334455.local" {
		t.Errorf("SRV target = %q", service.target)
	}

	txt, err := parseTXT(msg.records[2].data)
	if err != nil {
		t.Fatalf("parseTXT errored: %v", err)
	}
	want := []string{
		"ty=Brother HL-L2350DW",
		"product=(Brother HL-L2350DW series)",
		"rp=duerqxesz5090",
	}
	if !reflect.DeepEqual(txt, want) {
		t.Errorf("TXT = %v, want %v", txt, want)
	}

	if got := len(msg.records[3].data); got != 4 {
		t.Fatalf("A record data is %d bytes, want 4", got)
	}
	if ip := msg.records[3].data; ip[0] != 192 || ip[1] != 168 || ip[2] != 1 || ip[3] != 63 {
		t.Errorf("A record = %v, want 192.168.1.63", ip)
	}
}

func TestDecodeMessageFollowsACompressionPointerInSRV(t *testing.T) {
	pointer := mustHex(t, mdnsPointerHex)
	msg, err := decodeMessage(pointer)
	if err != nil {
		t.Fatalf("decodeMessage errored: %v", err)
	}
	if len(msg.records) != 2 {
		t.Fatalf("got %d records, want 2", len(msg.records))
	}
	service, err := parseSRV(pointer, msg.records[1])
	if err != nil {
		t.Fatalf("parseSRV errored: %v", err)
	}
	if service.target != "apple-tv.local" {
		t.Errorf("SRV target = %q, want apple-tv.local: the compression pointer was not followed", service.target)
	}
	if service.port != 7000 {
		t.Errorf("SRV port = %d, want 7000", service.port)
	}
}

// TestDecodeMessageTruncated is the test that stops a hostile or broken packet
// crashing the tool: every prefix of a real message must produce an error or a
// partial result, and none may panic.
func TestDecodeMessageTruncated(t *testing.T) {
	for _, fixture := range []string{mdnsPrinterHex, mdnsPointerHex, mdnsLoopHex, mdnsForwardHex} {
		full := mustHex(t, fixture)
		for n := 0; n <= len(full); n++ {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("decodeMessage panicked on a %d byte prefix: %v", n, r)
					}
				}()
				_, _ = decodeMessage(full[:n])
			}()
		}
	}
}

// TestDecodeMessageCorrupted goes further than truncation: every single byte of
// a real message flipped, one at a time, must still not panic.
func TestDecodeMessageCorrupted(t *testing.T) {
	full := mustHex(t, mdnsPrinterHex)
	for i := range full {
		for _, value := range []byte{0x00, 0xFF, 0xC0} {
			b := append([]byte{}, full...)
			b[i] = value
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("decodeMessage panicked with byte %d set to %#x: %v", i, value, r)
					}
				}()
				msg, _ := decodeMessage(b)
				for _, rec := range msg.records {
					_, _ = parseSRV(b, rec)
					_, _ = parseTXT(rec.data)
					_, _ = parsePTR(b, rec)
				}
			}()
		}
	}
}

func TestParseTXT(t *testing.T) {
	tests := []struct {
		name    string
		in      []byte
		want    []string
		wantErr bool
	}{
		{"several strings", []byte{2, 'a', 'b', 3, 'c', 'd', 'e'}, []string{"ab", "cde"}, false},
		{"one string", []byte{2, 'a', 'b'}, []string{"ab"}, false},
		// A single zero-length string is how a device says "I have no data",
		// and it must not become an empty string in the list.
		{"a single empty string", []byte{0}, nil, false},
		{"empty data", nil, nil, false},
		{"a length that runs off the end", []byte{5, 'a', 'b'}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTXT(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseTXT(%v) = %v, want an error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseTXT errored: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseTXT(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseSRVRejectsShortData(t *testing.T) {
	printer := mustHex(t, mdnsPrinterHex)
	if _, err := parseSRV(printer, resource{data: []byte{0, 0, 0}}); err != errBadRecordData {
		t.Errorf("err = %v, want %v", err, errBadRecordData)
	}
}

// longPointerChain builds a message holding one real name followed by hops
// pointers, each pointing at the one before it. Every hop is backwards, so the
// chain is legal hop by hop and only the length bound refuses it.
type pointerChain struct {
	msg   []byte
	start int
}

func longPointerChain(hops int) pointerChain {
	msg := make([]byte, headerBytes)
	binary.BigEndian.PutUint16(msg[2:], 0x8400)

	nameAt := len(msg)
	msg = append(msg, 3, 'e', 'n', 'd', 5, 'l', 'o', 'c', 'a', 'l', 0)

	previous := nameAt
	start := 0
	for i := 0; i < hops; i++ {
		start = len(msg)
		msg = binary.BigEndian.AppendUint16(msg, 0xC000|uint16(previous))
		previous = start
	}
	return pointerChain{msg: msg, start: start}
}
