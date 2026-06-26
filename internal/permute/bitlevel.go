package permute

// bitLevelVariants generates alignment variants for pure bit-repacking
// encodings (RFC 4648 base64 and base32), where each output character encodes a
// fixed number of consecutive bits of the input.
//
// For a target placed at byte offset s inside a larger stream, the input bits
// occupied by the target are [s*8, s*8+L*8). An output character is "stable"
// (fully determined by the target) only if its bit range lies entirely within
// that interval. Characters that straddle the prefix or the trailing data are
// dropped.
//
//	startChar = ceil(prefixBits / bitsPerChar)   -- first char with no prefix bits
//	endChar   = floor((prefixBits + L*8) / bitsPerChar) -- past last all-target char
//
// enc must produce the raw (un-padded) encoding of its input using the desired
// alphabet. The byte values of the prefix/suffix padding are irrelevant because
// the characters they influence are exactly the ones we discard.
func bitLevelVariants(name string, target []byte, bitsPerChar int, enc func([]byte) string) []Variant {
	L := len(target)
	if L == 0 {
		return nil
	}

	// Bytes per "clean" block: the smallest run of input bytes that lands on a
	// character boundary. base64: lcm(8,6)/8 = 3. base32: lcm(8,5)/8 = 5.
	blockBytes := lcm(8, bitsPerChar) / 8

	var out []Variant
	for s := 0; s < blockBytes; s++ {
		// prefix (s bytes) + target, padded up to a whole number of blocks so
		// the encoder never introduces padding inside our slice window.
		total := s + L
		paddedLen := ceilDiv(total, blockBytes) * blockBytes
		buf := make([]byte, paddedLen)
		copy(buf[s:], target)

		encoded := enc(buf)

		start := ceilDiv(s*8, bitsPerChar)
		end := (s*8 + L*8) / bitsPerChar
		if end <= start || end > len(encoded) {
			continue
		}
		out = append(out, Variant{Encoding: name, Offset: s, Pattern: encoded[start:end]})
	}
	return dedupe(out)
}

func lcm(a, b int) int { return a / gcd(a, b) * b }

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
