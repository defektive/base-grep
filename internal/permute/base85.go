package permute

// base85 encodings map each 4-byte block to 5 output characters by treating the
// block as a big-endian uint32 and writing it in base 85 (most-significant digit
// first). Unlike base64/base32 this is positional, not bit-repacking: every one
// of the 5 characters depends on all 4 input bytes. Alignment therefore works at
// whole-block granularity only — a stable run is the encoding of the 4-byte
// blocks that fall entirely within the target.
//
// We deliberately do not emit the Ascii85 "z" abbreviation for all-zero blocks,
// so every block is exactly 5 characters and our slice indices stay valid.

const (
	ascii85Alphabet = "!\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstu"
	// Z85 (ZeroMQ / RFC 32) alphabet.
	z85Alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ.-:+=^!/*?&<>()[]{}@%$#"
)

// encodeBase85Blocks encodes b (whose length must be a multiple of 4) into
// fixed-width 5-character-per-block output using the given alphabet.
func encodeBase85Blocks(b []byte, alphabet string) string {
	out := make([]byte, 0, len(b)/4*5)
	for i := 0; i+4 <= len(b); i += 4 {
		n := uint32(b[i])<<24 | uint32(b[i+1])<<16 | uint32(b[i+2])<<8 | uint32(b[i+3])
		var chunk [5]byte
		for j := 4; j >= 0; j-- {
			chunk[j] = alphabet[n%85]
			n /= 85
		}
		out = append(out, chunk[:]...)
	}
	return string(out)
}

// blockVariants generates whole-block alignment variants for a 4-byte/5-char
// block encoding.
func blockVariants(name string, target []byte, alphabet string) []Variant {
	const inBytes, outChars = 4, 5
	L := len(target)
	if L == 0 {
		return nil
	}

	var out []Variant
	for s := 0; s < inBytes; s++ {
		total := s + L
		paddedLen := ceilDiv(total, inBytes) * inBytes
		buf := make([]byte, paddedLen)
		copy(buf[s:], target)

		encoded := encodeBase85Blocks(buf, alphabet)

		startBlock := ceilDiv(s, inBytes) // first block with no prefix bytes
		endBlock := (s + L) / inBytes     // past last all-target block
		if endBlock <= startBlock {
			continue
		}
		start, end := startBlock*outChars, endBlock*outChars
		if end > len(encoded) {
			continue
		}
		out = append(out, Variant{Encoding: name, Offset: s, Pattern: encoded[start:end]})
	}
	return dedupe(out)
}

func ascii85Variants(target []byte) []Variant {
	return blockVariants("ascii85", target, ascii85Alphabet)
}

func z85Variants(target []byte) []Variant {
	return blockVariants("z85", target, z85Alphabet)
}
