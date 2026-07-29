package renamer

import "unicode"

// naturalLess orders names the way a person reads them. Plain string ordering
// puts IMG_10.jpg before IMG_9.jpg, which silently gives the wrong sequence
// numbers in the commonest job this tool does.
func naturalLess(a, b string) bool {
	ra, rb := []rune(a), []rune(b)
	i, j := 0, 0
	for i < len(ra) && j < len(rb) {
		if isDigit(ra[i]) && isDigit(rb[j]) {
			startA, startB := i, j
			for i < len(ra) && isDigit(ra[i]) {
				i++
			}
			for j < len(rb) && isDigit(rb[j]) {
				j++
			}
			da, db := trimLeadingZeros(ra[startA:i]), trimLeadingZeros(rb[startB:j])
			if len(da) != len(db) {
				return len(da) < len(db)
			}
			for k := range da {
				if da[k] != db[k] {
					return da[k] < db[k]
				}
			}
			// Same number, so the run written with fewer characters comes first
			// and file1 sorts before file01.
			if i-startA != j-startB {
				return i-startA < j-startB
			}
			continue
		}
		if ra[i] != rb[j] {
			la, lb := unicode.ToLower(ra[i]), unicode.ToLower(rb[j])
			if la != lb {
				return la < lb
			}
			// Same letter in different capitals. Ordering by the raw rune keeps
			// the comparison total, which sort.SliceStable needs.
			return ra[i] < rb[j]
		}
		i++
		j++
	}
	return len(ra)-i < len(rb)-j
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func trimLeadingZeros(digits []rune) []rune {
	k := 0
	for k < len(digits)-1 && digits[k] == '0' {
		k++
	}
	return digits[k:]
}
