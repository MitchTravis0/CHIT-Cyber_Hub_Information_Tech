package qrgen

// GF(256) arithmetic and Reed-Solomon encoding. The field is GF(2^8) with the
// primitive polynomial 0x11D and generator element alpha = 2, which is what the
// QR standard uses.

// expTable[i] is alpha^i. It is doubled to 512 entries so a multiplication can
// add two exponents without a modulo.
var expTable [512]byte

// logTable[v] is the exponent i for which alpha^i == v. logTable[0] is unused.
var logTable [256]byte

func init() {
	x := 1
	for i := 0; i < 255; i++ {
		expTable[i] = byte(x)
		logTable[byte(x)] = byte(i)
		x <<= 1
		if x > 255 {
			x ^= 0x11D
		}
	}
	for i := 255; i < 512; i++ {
		expTable[i] = expTable[i-255]
	}
}

// mul multiplies two field elements.
func mul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return expTable[int(logTable[a])+int(logTable[b])]
}

// generatorPoly builds g(x) = (x + alpha^0)(x + alpha^1)...(x + alpha^(n-1)),
// coefficients most significant first, so the result has n+1 of them and starts
// with 1. Subtraction is XOR in this field, so (x - a) and (x + a) are the same.
func generatorPoly(n int) []byte {
	gen := []byte{1}
	for i := 0; i < n; i++ {
		next := make([]byte, len(gen)+1)
		for j, c := range gen {
			next[j] ^= c
			next[j+1] ^= mul(c, expTable[i])
		}
		gen = next
	}
	return gen
}

// evalPoly evaluates a polynomial with coefficients most significant first at x.
func evalPoly(coeffs []byte, x byte) byte {
	var out byte
	for _, c := range coeffs {
		out = mul(out, x) ^ c
	}
	return out
}

// rsEncode returns the n error-correction codewords for one block: the
// remainder of the message polynomial shifted left by n, modulo g(x).
func rsEncode(data []byte, n int) []byte {
	gen := generatorPoly(n)
	rem := make([]byte, n)
	for _, b := range data {
		factor := b ^ rem[0]
		copy(rem, rem[1:])
		rem[len(rem)-1] = 0
		for i, g := range gen[1:] {
			rem[i] ^= mul(g, factor)
		}
	}
	return rem
}
