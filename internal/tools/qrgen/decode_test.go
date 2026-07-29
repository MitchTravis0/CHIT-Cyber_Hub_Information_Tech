package qrgen

import (
	"fmt"
	"math/bits"
	"strings"
	"testing"
)

// decodeForTest reads a finished matrix back into the string that was encoded.
// It is the oracle for the encoder: without it, nothing in this package proves
// that the bits ended up where a phone will look for them. It is compiled only
// into the tests and is never reachable from the app.
//
// It takes only Modules and Size from the Code. The version comes from the
// size, and the level and the mask come from the format information, because
// those are the values it exists to recover independently.
func decodeForTest(t *testing.T, c Code) string {
	t.Helper()

	size := c.Size
	if size < 21 || (size-17)%4 != 0 {
		t.Fatalf("a %d module matrix is not a QR code", size)
	}
	version := (size - 17) / 4
	if len(c.Modules) != size*size {
		t.Fatalf("%d modules for a %d by %d matrix", len(c.Modules), size, size)
	}
	read := func(row, col int) bool { return c.Modules[row*size+col] }

	// Both copies of the format information, then the closest of the 32 valid
	// values by Hamming distance, which is what a reader's BCH correction does.
	first, second := 0, 0
	for i := 0; i < 15; i++ {
		row1, col1, row2, col2 := formatPositions(size, i)
		if read(row1, col1) {
			first |= 1 << i
		}
		if read(row2, col2) {
			second |= 1 << i
		}
	}
	if first != second {
		t.Errorf("the two format copies disagree: 0x%04X and 0x%04X", first, second)
	}
	level, mask, distance := 0, 0, 16
	for candidateLevel := range levels {
		for candidateMask := 0; candidateMask < 8; candidateMask++ {
			d := bits.OnesCount(uint(formatInfo(candidateLevel, candidateMask) ^ first))
			if d < distance {
				level, mask, distance = candidateLevel, candidateMask, d
			}
		}
	}
	if distance > 3 {
		t.Fatalf("format information 0x%04X is %d bits from anything valid", first, distance)
	}
	if levels[level] != c.ECLevel {
		t.Errorf("recovered level %q, but the code says %q", levels[level], c.ECLevel)
	}
	if mask != c.Mask {
		t.Errorf("recovered mask %d, but the code says %d", mask, c.Mask)
	}

	// The function map comes from the version alone. An error in it would make
	// the round trip fail rather than pass, and TestFunctionPatterns pins the
	// positions absolutely.
	patterns := newMatrix(size)
	drawFunctionPatterns(patterns, version)

	stream := make([]bool, 0, totalCodewords[version-1]*8)
	for right := size - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		for vert := 0; vert < size; vert++ {
			for j := 0; j < 2; j++ {
				col := right - j
				upward := ((right + 1) & 2) == 0
				row := vert
				if upward {
					row = size - 1 - vert
				}
				if patterns.isFunction(row, col) {
					continue
				}
				value := read(row, col)
				if maskCondition(mask, row, col) {
					value = !value
				}
				stream = append(stream, value)
			}
		}
	}

	total := totalCodewords[version-1]
	if len(stream) < total*8 {
		t.Fatalf("only %d data modules, need %d", len(stream), total*8)
	}
	codewords := make([]byte, total)
	for i := 0; i < total*8; i++ {
		if stream[i] {
			codewords[i/8] |= 1 << (7 - i%8)
		}
	}

	// De-interleave into the blocks the version and level call for.
	info := blockTable[version-1][level]
	sizes := make([]int, 0, info.blocks1+info.blocks2)
	for i := 0; i < info.blocks1; i++ {
		sizes = append(sizes, info.data1)
	}
	for i := 0; i < info.blocks2; i++ {
		sizes = append(sizes, info.data2)
	}
	data := make([][]byte, len(sizes))
	ec := make([][]byte, len(sizes))
	pos := 0
	for i := 0; i < max(info.data1, info.data2); i++ {
		for b, n := range sizes {
			if i < n {
				data[b] = append(data[b], codewords[pos])
				pos++
			}
		}
	}
	for i := 0; i < info.ecPerBlock; i++ {
		for b := range ec {
			ec[b] = append(ec[b], codewords[pos])
			pos++
		}
	}

	// A full Berlekamp-Massey decoder would be code with one caller. Asserting
	// every syndrome is zero proves the error-correction codewords are correct
	// for the data, which is the property that matters here: the matrix is not
	// damaged, so there is nothing to correct.
	for b := range data {
		full := append(append([]byte{}, data[b]...), ec[b]...)
		for i := 0; i < info.ecPerBlock; i++ {
			if v := evalPoly(full, expTable[i]); v != 0 {
				t.Errorf("block %d: syndrome %d = %d, want 0", b, i, v)
			}
		}
	}

	var payload []byte
	for _, block := range data {
		payload = append(payload, block...)
	}

	at := 0
	take := func(n int) int {
		out := 0
		for i := 0; i < n; i++ {
			out <<= 1
			if payload[at/8]>>(7-at%8)&1 == 1 {
				out |= 1
			}
			at++
		}
		return out
	}
	if mode := take(4); mode != modeByte {
		t.Fatalf("mode indicator is %04b, want 0100", mode)
	}
	count := take(countBits(version))
	if count*8 > len(payload)*8-at {
		t.Fatalf("the header claims %d bytes, more than the code holds", count)
	}
	out := make([]byte, count)
	for i := range out {
		out[i] = byte(take(8))
	}
	return string(out)
}

// repeatTo builds a payload of exactly n bytes out of a repeating pattern, so a
// capacity case is easy to read in a failure message.
func repeatTo(n int) string {
	const pattern = "CHIT-0123456789-abcdefghijklmnopqrstuvwxyz-"
	return strings.Repeat(pattern, n/len(pattern)+1)[:n]
}

func TestRoundTripPayloads(t *testing.T) {
	capacityFor := map[string]int{"L": 271, "M": 213, "Q": 151, "H": 119}

	for _, level := range levels {
		payloads := map[string]string{
			"one byte":            "A",
			"the realistic case":  `WIFI:T:WPA;S:Guest-WiFi;P:Welcome2026;H:false;;`,
			"every escape":        `WIFI:T:WPA;S:Acme\;Corp;P:p@ss\:word\,really;H:true;;`,
			"a URL":               "https://helpdesk.example.com/new?from=qr&site=head-office",
			"100 bytes":           repeatTo(100),
			"the level's maximum": repeatTo(capacityFor[level]),
		}
		for name, payload := range payloads {
			t.Run(fmt.Sprintf("%s/%s", level, name), func(t *testing.T) {
				code, err := Generate(Params{Mode: "text", Text: payload, ECLevel: level})
				if err != nil {
					t.Fatalf("rejected: %v", err)
				}
				if got := decodeForTest(t, code); got != payload {
					t.Errorf("decoded\n  %q\nwant\n  %q", got, payload)
				}
			})
		}
	}
}

func TestRoundTripEveryVersion(t *testing.T) {
	for version := 1; version <= maxVersion; version++ {
		for level, name := range levels {
			t.Run(fmt.Sprintf("v%d-%s", version, name), func(t *testing.T) {
				payload := repeatTo(byteCapacity[version-1][level])
				code, err := Generate(Params{Mode: "text", Text: payload, ECLevel: name})
				if err != nil {
					t.Fatalf("rejected: %v", err)
				}
				if code.Version != version {
					t.Fatalf("version %d, want %d", code.Version, version)
				}
				if got := decodeForTest(t, code); got != payload {
					t.Errorf("decoded\n  %q\nwant\n  %q", got, payload)
				}
			})
		}
	}
}

// TestRoundTripMultiBlock is the case a broken interleaver fails. A single
// block code round-trips even when the interleave is wrong, so these two, which
// have two groups of different block lengths, are the ones that matter.
func TestRoundTripMultiBlock(t *testing.T) {
	cases := []struct {
		version int
		level   string
	}{
		{5, "Q"},
		{10, "Q"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("v%d-%s", tc.version, tc.level), func(t *testing.T) {
			level, _ := levelIndex(tc.level)
			info := blockTable[tc.version-1][level]
			if info.blocks2 == 0 {
				t.Fatalf("v%d-%s has only one group, so it proves nothing", tc.version, tc.level)
			}
			payload := repeatTo(byteCapacity[tc.version-1][level])
			code, err := Generate(Params{Mode: "text", Text: payload, ECLevel: tc.level})
			if err != nil {
				t.Fatalf("rejected: %v", err)
			}
			if code.Version != tc.version {
				t.Fatalf("version %d, want %d", code.Version, tc.version)
			}
			if got := decodeForTest(t, code); got != payload {
				t.Errorf("decoded\n  %q\nwant\n  %q", got, payload)
			}
		})
	}
}

func TestRoundTripNonASCII(t *testing.T) {
	code, err := Generate(Params{SSID: "Café-Gäste", Password: "Grüße2026"})
	if err != nil {
		t.Fatalf("rejected: %v", err)
	}
	want := `WIFI:T:WPA;S:Café-Gäste;P:Grüße2026;H:false;;`
	if code.Payload != want {
		t.Fatalf("payload is %q, want %q", code.Payload, want)
	}
	if got := decodeForTest(t, code); got != want {
		t.Errorf("decoded\n  %q\nwant\n  %q", got, want)
	}
}
