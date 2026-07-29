package qrgen

// Function patterns, data placement, masking and the format and version
// information. Throughout, (row, col) is row from the top and column from the
// left, both zero based.

// matrix is a code under construction: the modules plus a parallel map marking
// the ones that belong to a function pattern and must never be masked or
// overwritten by data.
type matrix struct {
	size    int
	modules []bool
	fn      []bool
}

func newMatrix(size int) *matrix {
	return &matrix{
		size:    size,
		modules: make([]bool, size*size),
		fn:      make([]bool, size*size),
	}
}

func (m *matrix) at(row, col int) bool { return m.modules[row*m.size+col] }

func (m *matrix) set(row, col int, dark bool) { m.modules[row*m.size+col] = dark }

func (m *matrix) isFunction(row, col int) bool { return m.fn[row*m.size+col] }

// setFunction writes a module and marks it as part of a function pattern.
func (m *matrix) setFunction(row, col int, dark bool) {
	m.modules[row*m.size+col] = dark
	m.fn[row*m.size+col] = true
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// drawFinder draws one 7 by 7 finder pattern with its top-left corner at
// (top, left), plus the light separator around it. The separator falls out of
// the same distance test: a module one step outside the 7 by 7 is at distance 4
// from the centre, and only distances 0, 1 and 3 are dark.
func drawFinder(m *matrix, top, left int) {
	for r := -1; r <= 7; r++ {
		for c := -1; c <= 7; c++ {
			row, col := top+r, left+c
			if row < 0 || row >= m.size || col < 0 || col >= m.size {
				continue
			}
			d := max(abs(r-3), abs(c-3))
			m.setFunction(row, col, d == 0 || d == 1 || d == 3)
		}
	}
}

// drawAlignment draws every alignment pattern for a version, skipping the three
// positions that would land on a finder pattern.
func drawAlignment(m *matrix, version int) {
	centres := alignCentres[version-1]
	if len(centres) == 0 {
		return
	}
	first, last := centres[0], centres[len(centres)-1]
	for _, rowCentre := range centres {
		for _, colCentre := range centres {
			onFinder := (rowCentre == first && colCentre == first) ||
				(rowCentre == first && colCentre == last) ||
				(rowCentre == last && colCentre == first)
			if onFinder {
				continue
			}
			for r := -2; r <= 2; r++ {
				for c := -2; c <= 2; c++ {
					d := max(abs(r), abs(c))
					m.setFunction(rowCentre+r, colCentre+c, d == 0 || d == 2)
				}
			}
		}
	}
}

// formatPositions returns the two placements of format information bit i: the
// copy around the top-left finder and the copy split between the bottom-left
// and the top-right.
func formatPositions(size, i int) (row1, col1, row2, col2 int) {
	switch {
	case i < 6:
		row1, col1 = i, 8
	case i == 6:
		row1, col1 = 7, 8
	case i == 7:
		row1, col1 = 8, 8
	case i == 8:
		row1, col1 = 8, 7
	default:
		row1, col1 = 8, 14-i
	}
	if i < 8 {
		row2, col2 = 8, size-1-i
	} else {
		row2, col2 = size-15+i, 8
	}
	return row1, col1, row2, col2
}

// reserveFormat marks the format information modules as function modules so the
// data placement skips them. The bits themselves are written once the mask is
// chosen.
func reserveFormat(m *matrix) {
	for i := 0; i < 15; i++ {
		row1, col1, row2, col2 := formatPositions(m.size, i)
		m.setFunction(row1, col1, false)
		m.setFunction(row2, col2, false)
	}
}

// levelBits is the two bit error-correction level field, indexed by the level's
// column in the tables (L, M, Q, H). The order is not the obvious one.
var levelBits = [4]int{1, 0, 3, 2}

// formatInfo is the 15 bit format information for a level and mask: five data
// bits, ten BCH(15,5) check bits, the whole thing XORed with 0x5412 so that
// level M with mask 0 does not come out all zero.
func formatInfo(level, mask int) int {
	data := levelBits[level]<<3 | mask
	rem := data
	for i := 0; i < 10; i++ {
		rem = (rem << 1) ^ ((rem >> 9) * 0x537)
	}
	return (data<<10 | rem) ^ 0x5412
}

// writeFormat writes both copies of the format information.
func writeFormat(m *matrix, bits int) {
	for i := 0; i < 15; i++ {
		dark := (bits>>i)&1 == 1
		row1, col1, row2, col2 := formatPositions(m.size, i)
		m.setFunction(row1, col1, dark)
		m.setFunction(row2, col2, dark)
	}
}

// drawVersionInfo writes the two 3 by 6 version information blocks. Versions 1
// to 6 do not carry them.
func drawVersionInfo(m *matrix, version int) {
	word, ok := versionInfoWords[version]
	if !ok {
		return
	}
	for i := 0; i < 18; i++ {
		dark := (word>>i)&1 == 1
		b := i / 3
		m.setFunction(b, m.size-11+i%3, dark)
		m.setFunction(m.size-11+i%3, b, dark)
	}
}

// drawFunctionPatterns lays down everything that is not data, in the order
// section 10.6 of the spec gives.
func drawFunctionPatterns(m *matrix, version int) {
	size := m.size
	drawFinder(m, 0, 0)
	drawFinder(m, 0, size-7)
	drawFinder(m, size-7, 0)

	for i := 8; i < size-8; i++ {
		dark := i%2 == 0
		m.setFunction(6, i, dark)
		m.setFunction(i, 6, dark)
	}

	drawAlignment(m, version)
	reserveFormat(m)
	// The dark module has no meaning, it is simply mandated. It goes in after the
	// format reservation so nothing can leave it light.
	m.setFunction(4*version+9, 8, true)
	drawVersionInfo(m, version)
}

// placeData walks the zig-zag and writes the codeword stream into every module
// that is not a function module. Any spare modules at the end stay light.
func placeData(m *matrix, codewords []byte) {
	size := m.size
	totalBits := len(codewords) * 8
	placed := 0
	for right := size - 1; right >= 1; right -= 2 {
		if right == 6 {
			// Column 6 is the vertical timing pattern. It is skipped entirely, not
			// shifted, so the pair here is (5, 4) and the walk continues at 3.
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
				if !m.isFunction(row, col) && placed < totalBits {
					m.set(row, col, (codewords[placed/8]>>(7-placed%8))&1 == 1)
					placed++
				}
			}
		}
	}
}

// maskCondition reports whether the data module at (row, col) is inverted by
// the given mask pattern. All divisions are integer divisions.
func maskCondition(mask, row, col int) bool {
	switch mask {
	case 0:
		return (row+col)%2 == 0
	case 1:
		return row%2 == 0
	case 2:
		return col%3 == 0
	case 3:
		return (row+col)%3 == 0
	case 4:
		return (row/2+col/3)%2 == 0
	case 5:
		return (row*col)%2+(row*col)%3 == 0
	case 6:
		return ((row*col)%2+(row*col)%3)%2 == 0
	default:
		return ((row+col)%2+(row*col)%3)%2 == 0
	}
}

// applyMask inverts every data module the mask selects. Applying the same mask
// twice restores the matrix, because it is an XOR.
func applyMask(m *matrix, mask int) {
	for row := 0; row < m.size; row++ {
		for col := 0; col < m.size; col++ {
			if !m.isFunction(row, col) && maskCondition(mask, row, col) {
				m.set(row, col, !m.at(row, col))
			}
		}
	}
}

// finderLike are the two 11 module sequences penalty rule 3 looks for.
var finderLike = [2][11]bool{
	{true, false, true, true, true, false, true, false, false, false, false},
	{false, false, false, false, true, false, true, true, true, false, true},
}

// penalty scores a masked matrix with the four rules. The lowest score wins.
func penalty(m *matrix) int {
	size := m.size
	score := 0

	// Rule 1: runs of five or more of the same colour, rows and columns both.
	for i := 0; i < size; i++ {
		score += runPenalty(m, i, true) + runPenalty(m, i, false)
	}

	// Rule 2: every 2 by 2 square of one colour, overlapping squares included.
	for row := 0; row < size-1; row++ {
		for col := 0; col < size-1; col++ {
			v := m.at(row, col)
			if m.at(row, col+1) == v && m.at(row+1, col) == v && m.at(row+1, col+1) == v {
				score += 3
			}
		}
	}

	// Rule 3: finder-like sequences, rows and columns, overlaps counted.
	for i := 0; i < size; i++ {
		score += finderLikePenalty(m, i, true) + finderLikePenalty(m, i, false)
	}

	// Rule 4: how far the dark proportion is from half.
	dark := 0
	for _, v := range m.modules {
		if v {
			dark++
		}
	}
	percent := dark * 100 / (size * size)
	score += 10 * (abs(percent-50) / 5)

	return score
}

// lineAt reads module i of a row (byRow) or of a column.
func lineAt(m *matrix, index, i int, byRow bool) bool {
	if byRow {
		return m.at(index, i)
	}
	return m.at(i, index)
}

func runPenalty(m *matrix, index int, byRow bool) int {
	score, run := 0, 1
	for i := 1; i < m.size; i++ {
		if lineAt(m, index, i, byRow) == lineAt(m, index, i-1, byRow) {
			run++
			continue
		}
		if run >= 5 {
			score += 3 + (run - 5)
		}
		run = 1
	}
	if run >= 5 {
		score += 3 + (run - 5)
	}
	return score
}

func finderLikePenalty(m *matrix, index int, byRow bool) int {
	score := 0
	for start := 0; start+11 <= m.size; start++ {
		for _, want := range finderLike {
			hit := true
			for i, v := range want {
				if lineAt(m, index, start+i, byRow) != v {
					hit = false
					break
				}
			}
			if hit {
				score += 40
			}
		}
	}
	return score
}

// chooseMask scores all eight masks with their own format information written
// in, because the format modules sit next to the finder patterns and change
// three of the four penalties. The lowest score wins, ties go to the lower mask
// number. The winner is left applied and written.
func chooseMask(m *matrix, level int) int {
	best, bestScore := 0, -1
	for mask := 0; mask < 8; mask++ {
		writeFormat(m, formatInfo(level, mask))
		applyMask(m, mask)
		score := penalty(m)
		applyMask(m, mask)
		if bestScore < 0 || score < bestScore {
			best, bestScore = mask, score
		}
	}
	writeFormat(m, formatInfo(level, best))
	applyMask(m, best)
	return best
}
