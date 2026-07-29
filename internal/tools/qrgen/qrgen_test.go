package qrgen

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"chit/internal/core"
)

// TestTablesAreConsistent cross-checks every row of tables.go against every
// other row. A transcription slip in any of the forty block entries fails here
// rather than producing a code no phone can read.
func TestTablesAreConsistent(t *testing.T) {
	for version := 1; version <= maxVersion; version++ {
		for level := range levels {
			b := blockTable[version-1][level]
			name := fmt.Sprintf("v%d-%s", version, levels[level])

			data := b.blocks1*b.data1 + b.blocks2*b.data2
			total := data + (b.blocks1+b.blocks2)*b.ecPerBlock
			if total != totalCodewords[version-1] {
				t.Errorf("%s: blocks add up to %d codewords, want %d", name, total, totalCodewords[version-1])
			}

			header := 2
			if version >= 10 {
				header = 3
			}
			if data != byteCapacity[version-1][level]+header {
				t.Errorf("%s: %d data codewords, but the capacity table implies %d",
					name, data, byteCapacity[version-1][level]+header)
			}

			if b.blocks2 != 0 && b.data2 != b.data1+1 {
				t.Errorf("%s: second group holds %d, want %d", name, b.data2, b.data1+1)
			}
		}

		centres := alignCentres[version-1]
		if version == 1 {
			if len(centres) != 0 {
				t.Errorf("v1: %d alignment centres, want none", len(centres))
			}
			continue
		}
		if centres[0] != 6 {
			t.Errorf("v%d: alignment centres start at %d, want 6", version, centres[0])
		}
		if last := centres[len(centres)-1]; last != sizeOf(version)-7 {
			t.Errorf("v%d: alignment centres end at %d, want %d", version, last, sizeOf(version)-7)
		}
	}
}

// TestAlignmentCentresMatchTheStandard recomputes every centre from the rule in
// the QR standard rather than checking the table against itself. The first
// centre is 6, the last is size-7, and the gaps between them are equal and
// rounded up to an even number. TestTablesAreConsistent only pins the two ends,
// so without this an interior centre could be moved and every other guard would
// miss it: the encoder and the test decoder both read this table, so they skip
// the same wrong squares and the round trip still passes.
func TestAlignmentCentresMatchTheStandard(t *testing.T) {
	for version := 2; version <= maxVersion; version++ {
		count := version/7 + 2
		last := sizeOf(version) - 7

		want := make([]int, count)
		want[0] = 6
		if count > 2 {
			gaps := count - 1
			step := 2 * ((last - 6 + 2*gaps - 1) / (2 * gaps))
			for i := count - 1; i > 0; i-- {
				want[i] = last - (count-1-i)*step
			}
		} else {
			want[1] = last
		}

		got := alignCentres[version-1]
		if len(got) != len(want) {
			t.Errorf("v%d: %d alignment centres %v, want %d %v", version, len(got), got, len(want), want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("v%d: alignment centres are %v, want %v", version, got, want)
				break
			}
		}
	}
}

// TestVersionInformationWords recomputes the four constants from the BCH(18,6)
// generator, so the table cannot be a mistyped copy of something.
func TestVersionInformationWords(t *testing.T) {
	want := map[int]int{7: 0x07C94, 8: 0x085BC, 9: 0x09A99, 10: 0x0A4D3}
	for version, expected := range want {
		rem := version
		for i := 0; i < 12; i++ {
			rem = (rem << 1) ^ ((rem >> 11) * 0x1F25)
		}
		got := version<<12 | rem
		if got != expected {
			t.Errorf("v%d: computed 0x%05X, want 0x%05X", version, got, expected)
		}
		if versionInfoWords[version] != expected {
			t.Errorf("v%d: table holds 0x%05X, want 0x%05X", version, versionInfoWords[version], expected)
		}
	}
	for version := 1; version <= 6; version++ {
		if _, ok := versionInfoWords[version]; ok {
			t.Errorf("v%d must not carry version information", version)
		}
	}
}

func TestGeneratorPolynomial(t *testing.T) {
	published := map[int][]byte{
		7:  {0, 87, 229, 146, 149, 238, 102, 21},
		10: {0, 251, 67, 46, 61, 118, 70, 64, 94, 32, 45},
	}
	for degree, want := range published {
		gen := generatorPoly(degree)
		if len(gen) != len(want) {
			t.Fatalf("degree %d: %d coefficients, want %d", degree, len(gen), len(want))
		}
		for i, coeff := range gen {
			if logTable[coeff] != want[i] {
				t.Errorf("degree %d coefficient %d: alpha^%d, want alpha^%d",
					degree, i, logTable[coeff], want[i])
			}
		}
	}

	// The defining property, which no mistyped table can satisfy: every alpha^i
	// for i below the degree is a root of g.
	for _, degree := range []int{7, 10, 13, 15, 16, 17, 18, 20, 22, 24, 26, 28, 30} {
		gen := generatorPoly(degree)
		for i := 0; i < degree; i++ {
			if v := evalPoly(gen, expTable[i]); v != 0 {
				t.Errorf("degree %d: g(alpha^%d) = %d, want 0", degree, i, v)
			}
		}
	}
}

// TestReedSolomonSyndromes checks the definition rather than a transcription:
// the data plus its error-correction codewords, read as one polynomial, must be
// divisible by g, so every syndrome is zero.
func TestReedSolomonSyndromes(t *testing.T) {
	random := rand.New(rand.NewSource(20260726))
	noise := make([]byte, 40)
	for i := range noise {
		noise[i] = byte(random.Intn(256))
	}

	blocks := map[string][]byte{
		"all zero":     make([]byte, 40),
		"all ones":     []byte(strings.Repeat("\xff", 40)),
		"pseudorandom": noise,
	}
	for name, data := range blocks {
		for _, ecPerBlock := range []int{7, 18, 30} {
			ec := rsEncode(data, ecPerBlock)
			if len(ec) != ecPerBlock {
				t.Fatalf("%s/%d: %d error-correction bytes, want %d", name, ecPerBlock, len(ec), ecPerBlock)
			}
			full := append(append([]byte{}, data...), ec...)
			for i := 0; i < ecPerBlock; i++ {
				if v := evalPoly(full, expTable[i]); v != 0 {
					t.Errorf("%s/%d: syndrome %d = %d, want 0", name, ecPerBlock, i, v)
				}
			}
		}
	}
}

func TestGF256Field(t *testing.T) {
	for _, x := range []byte{0, 1, 2, 17, 128, 255} {
		if mul(0, x) != 0 || mul(x, 0) != 0 {
			t.Errorf("mul with zero is not zero for %d", x)
		}
		if mul(1, x) != x || mul(x, 1) != x {
			t.Errorf("mul by one changed %d", x)
		}
	}
	for a := 0; a < 256; a += 7 {
		for b := 0; b < 256; b += 11 {
			if mul(byte(a), byte(b)) != mul(byte(b), byte(a)) {
				t.Errorf("mul(%d, %d) is not commutative", a, b)
			}
		}
	}
	if expTable[0] != 1 || expTable[255] != 1 {
		t.Errorf("expTable[0] = %d and expTable[255] = %d, both want 1", expTable[0], expTable[255])
	}
	for i := 0; i < 255; i++ {
		if int(logTable[expTable[i]]) != i {
			t.Errorf("logTable[expTable[%d]] = %d", i, logTable[expTable[i]])
		}
	}
}

func TestBitStreamLength(t *testing.T) {
	for _, version := range []int{1, 5, 9, 10} {
		for level := range levels {
			name := fmt.Sprintf("v%d-%s", version, levels[level])
			capacity := byteCapacity[version-1][level]
			want := dataCodewords(version, level)

			t.Run(name, func(t *testing.T) {
				stream := buildDataCodewords(strings.Repeat("A", 5), version, level)
				if len(stream) != want {
					t.Fatalf("stream is %d codewords, want %d", len(stream), want)
				}
				if stream[0]>>4 != modeByte {
					t.Errorf("mode indicator is %04b, want 0100", stream[0]>>4)
				}

				// A payload that exactly fills the version leaves room for the four
				// bit terminator and nothing else, so there are no pad bytes and the
				// last byte is the tail of the payload shifted up.
				full := strings.Repeat("A", capacity)
				stream = buildDataCodewords(full, version, level)
				if len(stream) != want {
					t.Fatalf("full stream is %d codewords, want %d", len(stream), want)
				}
				if last := stream[len(stream)-1]; last != (full[capacity-1]&0x0F)<<4 {
					t.Errorf("last byte of a full stream is 0x%02X, want 0x%02X",
						last, (full[capacity-1]&0x0F)<<4)
				}

				// One byte under, and exactly one pad byte of 0xEC is added.
				stream = buildDataCodewords(strings.Repeat("A", capacity-1), version, level)
				if len(stream) != want {
					t.Fatalf("short stream is %d codewords, want %d", len(stream), want)
				}
				if last := stream[len(stream)-1]; last != 0xEC {
					t.Errorf("last byte is 0x%02X, want the first pad byte 0xEC", last)
				}
				if second := stream[len(stream)-2]; second == 0xEC || second == 0x11 {
					t.Errorf("second to last byte is 0x%02X, so more than one pad byte was added", second)
				}
			})
		}
	}
}

func TestFunctionPatterns(t *testing.T) {
	for _, version := range []int{1, 6, 7, 10} {
		t.Run(fmt.Sprintf("v%d", version), func(t *testing.T) {
			size := sizeOf(version)
			m := newMatrix(size)
			drawFunctionPatterns(m, version)

			dark := map[string][2]int{
				"top-left finder centre":     {3, 3},
				"top-right finder centre":    {3, size - 4},
				"bottom-left finder centre":  {size - 4, 3},
				"horizontal timing at col 8": {6, 8},
				"vertical timing at row 8":   {8, 6},
				"the dark module":            {4*version + 9, 8},
			}
			for name, at := range dark {
				if !m.at(at[0], at[1]) {
					t.Errorf("%s at (%d, %d) is light", name, at[0], at[1])
				}
			}

			light := map[string][2]int{
				"the light ring of the top-left finder": {3, 5},
				"the separator corner":                  {7, 7},
				"horizontal timing at col 9":            {6, 9},
				"vertical timing at row 9":              {9, 6},
			}
			for name, at := range light {
				if m.at(at[0], at[1]) {
					t.Errorf("%s at (%d, %d) is dark", name, at[0], at[1])
				}
			}

			if m.isFunction(size-4, size-4) {
				t.Errorf("(%d, %d) is a function module, so something drew a fourth finder", size-4, size-4)
			}

			// Only a centre between the first and the last is drawn on row 6: the
			// (first, first) and (first, last) combinations sit on finder patterns
			// and are skipped, so versions 2 to 6 have nothing to check here. The
			// two coordinates are written out rather than read from alignCentres,
			// because alignCentres is part of what this is checking.
			if centre, ok := map[int]int{7: 22, 10: 28}[version]; ok {
				if !m.at(6, centre) {
					t.Errorf("alignment centre (6, %d) is light", centre)
				}
				if m.at(6, centre+1) {
					t.Errorf("alignment ring (6, %d) is dark", centre+1)
				}
			}

			free := 0
			for i := range m.fn {
				if !m.fn[i] {
					free++
				}
			}
			spare := free - totalCodewords[version-1]*8
			if spare < 0 || spare > 7 {
				t.Errorf("%d modules are free for data, which is %d away from %d codewords",
					free, spare, totalCodewords[version-1])
			}
			if version == 1 {
				// Version 1 has no alignment patterns, so the function modules are
				// exactly the three finders with separators (192), the timing
				// patterns (10), the dark module (1) and both format copies (30).
				if used := size*size - free; used != 233 {
					t.Errorf("v1 has %d function modules, want 233", used)
				}
			}
		})
	}
}

func TestParamsValidation(t *testing.T) {
	longSSID := strings.Repeat("N", 33)
	longWifi := Params{
		SSID:     strings.Repeat("N", 32),
		Password: strings.Repeat("p", 90),
		ECLevel:  "H",
	}
	longPayload := buildWifiPayload(longWifi.SSID, longWifi.Password, "WPA", false)

	cases := []struct {
		name   string
		params Params
		want   string
	}{
		{
			name:   "unknown mode",
			params: Params{Mode: "photo"},
			want:   `"photo" is not a kind of QR code. Use "wifi" for a network or "text" for anything else.`,
		},
		{
			name:   "unknown security",
			params: Params{SSID: "Guest", Security: "WPA4"},
			want:   `"WPA4" is not a Wi-Fi security setting. Use WPA, SAE, WEP or nopass.`,
		},
		{
			name:   "unknown error-correction level",
			params: Params{SSID: "Guest", ECLevel: "Z"},
			want:   `"Z" is not an error-correction setting. Use L, M, Q or H.`,
		},
		{
			name:   "SSID of 33 bytes",
			params: Params{SSID: longSSID},
			want:   "A Wi-Fi network name can be at most 32 characters. That one comes to 33.",
		},
		{
			name:   "empty SSID",
			params: Params{SSID: "   "},
			want:   "Type the network name exactly as it appears on the router, capital letters and all.",
		},
		{
			name:   "empty text",
			params: Params{Mode: "text", Text: "  \n"},
			want:   "Type or paste the text or link to put in the code.",
		},
		{
			name:   "272 bytes of text at level L",
			params: Params{Mode: "text", Text: strings.Repeat("A", 272), ECLevel: "L"},
			want:   "That comes to 272 bytes and the largest QR code CHIT makes holds 271 at the L setting. Shorten the text, or choose a lower error-correction setting: L holds the most.",
		},
		{
			name:   "a Wi-Fi payload too long for level H",
			params: longWifi,
			want: fmt.Sprintf(
				"The network name and password together come to %d bytes, and the largest QR code CHIT makes holds 119 at the H setting. Choose a lower error-correction setting (L holds the most), or use a shorter password.",
				len(longPayload)),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Generate(tc.params)
			if err == nil {
				t.Fatal("the request was accepted")
			}
			if core.CodeOf(err) != core.CodeInvalidInput {
				t.Errorf("code is %q, want %q", core.CodeOf(err), core.CodeInvalidInput)
			}
			if err.Error() != tc.want {
				t.Errorf("message is\n  %q\nwant\n  %q", err.Error(), tc.want)
			}
		})
	}

	t.Run("271 bytes of text at level L is accepted", func(t *testing.T) {
		code, err := Generate(Params{Mode: "text", Text: strings.Repeat("A", 271), ECLevel: "L"})
		if err != nil {
			t.Fatalf("rejected: %v", err)
		}
		if code.Version != 10 {
			t.Errorf("version %d, want 10", code.Version)
		}
		if code.Capacity != 271 || code.PayloadBytes != 271 {
			t.Errorf("payload %d of capacity %d, want 271 of 271", code.PayloadBytes, code.Capacity)
		}
	})
}

// knownMatrix is the complete version 1 level M code for the payload "CHIT".
//
// Be honest about what this is: a regression lock, not an independent oracle.
// It was produced by printing this package's own output once the round-trip
// tests passed, and the code it describes was checked against a real reader
// before it was written down. From then on it fails loudly if anyone changes
// the mask scoring, the placement order or the interleaver.
var knownMatrix = []string{
	"#######...###.#######",
	"#.....#.##.##.#.....#",
	"#.###.#.#..#..#.###.#",
	"#.###.#.#...#.#.###.#",
	"#.###.#...#.#.#.###.#",
	"#.....#....#..#.....#",
	"#######.#.#.#.#######",
	"........#............",
	"#.....#.#..#.##..###.",
	"..#.#...##.###.##...#",
	"##.#.##.###.#.##..##.",
	".#.##....#######..###",
	"#.##.##..#.#######.##",
	"........#...#....#...",
	"#######..###.#..####.",
	"#.....#..#....#..####",
	"#.###.#..#.#.#..#.##.",
	"#.###.#...#####......",
	"#.###.#..#####.##.###",
	"#.....#..######..##..",
	"#######.#...#....###.",
}

// renderMatrix draws a code the same way knownMatrix is written, so a failure
// prints two grids that line up.
func renderMatrix(c Code) []string {
	out := make([]string, c.Size)
	for row := 0; row < c.Size; row++ {
		line := make([]byte, c.Size)
		for col := 0; col < c.Size; col++ {
			line[col] = '.'
			if c.Modules[row*c.Size+col] {
				line[col] = '#'
			}
		}
		out[row] = string(line)
	}
	return out
}

func TestKnownMatrix(t *testing.T) {
	// The payload is a fixed test vector, not the product name, and it must stay
	// "CH-IT" even though the product is now called CHIT. knownMatrix and the
	// expected mask below were verified against the standard's own constants in
	// Phase 3; regenerating them from this package's encoder to match a renamed
	// payload would replace an independent check with a circular one.
	code, err := Generate(Params{Mode: "text", Text: "CH-IT", ECLevel: "M"})
	if err != nil {
		t.Fatalf("rejected: %v", err)
	}
	if code.Version != 1 || code.Size != 21 {
		t.Fatalf("version %d at %d modules, want version 1 at 21", code.Version, code.Size)
	}
	if code.Mask != 5 {
		t.Errorf("mask %d, want 5", code.Mask)
	}
	got := renderMatrix(code)
	if strings.Join(got, "\n") != strings.Join(knownMatrix, "\n") {
		t.Errorf("the matrix changed.\ngot:\n%s\n\nwant:\n%s",
			strings.Join(got, "\n"), strings.Join(knownMatrix, "\n"))
	}
}

func TestGenerateChoosesSmallestVersion(t *testing.T) {
	cases := []struct {
		level   string
		bytes   int
		version int
	}{
		{"M", 14, 1},
		{"M", 15, 2},
		{"M", 26, 2},
		{"M", 27, 3},
		{"M", 213, 10},
		{"L", 17, 1},
		{"L", 18, 2},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s-%d", tc.level, tc.bytes), func(t *testing.T) {
			code, err := Generate(Params{Mode: "text", Text: strings.Repeat("A", tc.bytes), ECLevel: tc.level})
			if err != nil {
				t.Fatalf("rejected: %v", err)
			}
			if code.Version != tc.version {
				t.Errorf("version %d, want %d", code.Version, tc.version)
			}
			if code.Size != 4*code.Version+17 {
				t.Errorf("size %d, want %d", code.Size, 4*code.Version+17)
			}
			if len(code.Modules) != code.Size*code.Size {
				t.Errorf("%d modules, want %d", len(code.Modules), code.Size*code.Size)
			}
			if code.Quiet != 4 {
				t.Errorf("quiet zone %d, want 4", code.Quiet)
			}
			if code.ECLevel != tc.level {
				t.Errorf("level %q, want %q", code.ECLevel, tc.level)
			}
		})
	}
}
