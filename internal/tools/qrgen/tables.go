package qrgen

// Everything in this file is data lifted from the QR standard for versions 1 to
// 10 only, which is the whole range this tool supports. TestTablesAreConsistent
// cross-checks every row against the others, so a transcription slip fails the
// build rather than producing a code no phone can read.

// maxVersion is the largest code this tool makes. Version 10 holds 271 bytes at
// level L, which covers every Wi-Fi payload and every URL a tech will paste.
const maxVersion = 10

// levels is the error-correction letter for each level index, in the order the
// tables below use: L, M, Q, H.
var levels = [4]string{"L", "M", "Q", "H"}

// levelIndex turns an error-correction letter into its column in the tables.
func levelIndex(level string) (int, bool) {
	for i, name := range levels {
		if name == level {
			return i, true
		}
	}
	return 0, false
}

// byteCapacity is the most payload bytes a version holds at each level, indexed
// [version-1][level]. It equals the version's data codeword count minus 2 for
// versions 1 to 9 and minus 3 for version 10, because the header is 12 bits
// below version 10 and 20 bits at version 10.
var byteCapacity = [maxVersion][4]int{
	{17, 14, 11, 7},
	{32, 26, 20, 14},
	{53, 42, 32, 24},
	{78, 62, 46, 34},
	{106, 84, 60, 44},
	{134, 106, 74, 58},
	{154, 122, 86, 64},
	{192, 152, 108, 84},
	{230, 180, 130, 98},
	{271, 213, 151, 119},
}

// totalCodewords is data plus error correction for each version.
var totalCodewords = [maxVersion]int{26, 44, 70, 100, 134, 172, 196, 242, 292, 346}

// blockInfo is the Reed-Solomon block layout for one version and level. Group 2
// blocks always hold exactly one more data codeword than group 1 blocks, and
// blocks2 of 0 means there is no second group.
type blockInfo struct {
	ecPerBlock int
	blocks1    int
	data1      int
	blocks2    int
	data2      int
}

// blockTable is indexed [version-1][level].
var blockTable = [maxVersion][4]blockInfo{
	{{7, 1, 19, 0, 0}, {10, 1, 16, 0, 0}, {13, 1, 13, 0, 0}, {17, 1, 9, 0, 0}},
	{{10, 1, 34, 0, 0}, {16, 1, 28, 0, 0}, {22, 1, 22, 0, 0}, {28, 1, 16, 0, 0}},
	{{15, 1, 55, 0, 0}, {26, 1, 44, 0, 0}, {18, 2, 17, 0, 0}, {22, 2, 13, 0, 0}},
	{{20, 1, 80, 0, 0}, {18, 2, 32, 0, 0}, {26, 2, 24, 0, 0}, {16, 4, 9, 0, 0}},
	{{26, 1, 108, 0, 0}, {24, 2, 43, 0, 0}, {18, 2, 15, 2, 16}, {22, 2, 11, 2, 12}},
	{{18, 2, 68, 0, 0}, {16, 4, 27, 0, 0}, {24, 4, 19, 0, 0}, {28, 4, 15, 0, 0}},
	{{20, 2, 78, 0, 0}, {18, 4, 31, 0, 0}, {18, 2, 14, 4, 15}, {26, 4, 13, 1, 14}},
	{{24, 2, 97, 0, 0}, {22, 2, 38, 2, 39}, {22, 4, 18, 2, 19}, {26, 4, 14, 2, 15}},
	{{30, 2, 116, 0, 0}, {22, 3, 36, 2, 37}, {20, 4, 16, 4, 17}, {24, 4, 12, 4, 13}},
	{{18, 2, 68, 2, 69}, {26, 4, 43, 1, 44}, {24, 6, 19, 2, 20}, {28, 6, 15, 2, 16}},
}

// alignCentres lists the alignment pattern centre coordinates for each version,
// indexed [version-1]. Version 1 has none.
var alignCentres = [maxVersion][]int{
	nil,
	{6, 18},
	{6, 22},
	{6, 26},
	{6, 30},
	{6, 34},
	{6, 22, 38},
	{6, 24, 42},
	{6, 26, 46},
	{6, 28, 50},
}

// versionInfoWords holds the 18 bit version information for versions 7 to 10,
// already carrying its BCH(18,6) check bits. Four constants beat a BCH encoder
// with four callers, and TestVersionInformationWords recomputes all four.
var versionInfoWords = map[int]int{
	7:  0x07C94,
	8:  0x085BC,
	9:  0x09A99,
	10: 0x0A4D3,
}

// sizeOf is the width and height of a version's matrix in modules.
func sizeOf(version int) int { return 4*version + 17 }

// dataCodewords is how many of a version and level's codewords carry payload.
func dataCodewords(version, level int) int {
	b := blockTable[version-1][level]
	return b.blocks1*b.data1 + b.blocks2*b.data2
}

// countBits is the width of the character count indicator. The boundary between
// 8 and 16 bits falls inside our supported range, so it cannot be hard coded: a
// version 10 code with an 8 bit count is not readable by anything.
func countBits(version int) int {
	if version >= 10 {
		return 16
	}
	return 8
}
