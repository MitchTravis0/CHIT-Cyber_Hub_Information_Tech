package qrgen

// The bit stream, the padding and the block interleave. Byte mode only.

// modeByte is the four bit mode indicator for byte mode.
const modeByte = 0b0100

// padBytes alternate to fill a data stream that the payload does not fill. The
// two values are fixed by the standard, they are not arbitrary filler.
var padBytes = [2]byte{0xEC, 0x11}

// bitBuffer accumulates bits most significant first into whole bytes.
type bitBuffer struct {
	bytes []byte
	n     int
}

// appendBits writes the low `bits` bits of value, most significant first.
func (b *bitBuffer) appendBits(value, bits int) {
	for i := bits - 1; i >= 0; i-- {
		if b.n%8 == 0 {
			b.bytes = append(b.bytes, 0)
		}
		if (value>>i)&1 == 1 {
			b.bytes[b.n/8] |= 1 << (7 - b.n%8)
		}
		b.n++
	}
}

// buildDataCodewords turns a payload into exactly the version and level's data
// codeword count of bytes: header, payload, terminator, byte padding, pad bytes.
func buildDataCodewords(payload string, version, level int) []byte {
	total := dataCodewords(version, level)
	capacity := total * 8

	var b bitBuffer
	data := []byte(payload)
	b.appendBits(modeByte, 4)
	b.appendBits(len(data), countBits(version))
	for _, c := range data {
		b.appendBits(int(c), 8)
	}

	terminator := 4
	if capacity-b.n < terminator {
		terminator = capacity - b.n
	}
	b.appendBits(0, terminator)
	if b.n%8 != 0 {
		b.appendBits(0, 8-b.n%8)
	}
	for i := 0; len(b.bytes) < total; i++ {
		b.bytes = append(b.bytes, padBytes[i%2])
		b.n += 8
	}
	return b.bytes
}

// interleave splits the data codewords into Reed-Solomon blocks, encodes each
// one, and returns the final codeword stream: interleaved data followed by
// interleaved error correction.
func interleave(data []byte, version, level int) []byte {
	info := blockTable[version-1][level]

	blocks := make([][]byte, 0, info.blocks1+info.blocks2)
	pos := 0
	for i := 0; i < info.blocks1; i++ {
		blocks = append(blocks, data[pos:pos+info.data1])
		pos += info.data1
	}
	for i := 0; i < info.blocks2; i++ {
		blocks = append(blocks, data[pos:pos+info.data2])
		pos += info.data2
	}

	ec := make([][]byte, len(blocks))
	for i, block := range blocks {
		ec[i] = rsEncode(block, info.ecPerBlock)
	}

	out := make([]byte, 0, totalCodewords[version-1])
	longest := max(info.data1, info.data2)
	for i := 0; i < longest; i++ {
		for _, block := range blocks {
			if i < len(block) {
				out = append(out, block[i])
			}
		}
	}
	for i := 0; i < info.ecPerBlock; i++ {
		for _, block := range ec {
			out = append(out, block[i])
		}
	}
	return out
}
