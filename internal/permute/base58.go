package permute

import "math/big"

// base58Alphabet is the Bitcoin / IPFS base58 alphabet.
const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

var big58 = big.NewInt(58)

// EncodeBase58 encodes b using the Bitcoin base58 alphabet. Each leading zero
// byte becomes a leading '1'.
func EncodeBase58(b []byte) string {
	// Count leading zero bytes; each maps to a single '1'.
	zeros := 0
	for zeros < len(b) && b[zeros] == 0 {
		zeros++
	}

	num := new(big.Int).SetBytes(b)
	mod := new(big.Int)
	var rev []byte
	for num.Sign() > 0 {
		num.DivMod(num, big58, mod)
		rev = append(rev, base58Alphabet[mod.Int64()])
	}

	out := make([]byte, 0, zeros+len(rev))
	for i := 0; i < zeros; i++ {
		out = append(out, base58Alphabet[0])
	}
	for i := len(rev) - 1; i >= 0; i-- {
		out = append(out, rev[i])
	}
	return string(out)
}

// base58Variants returns the single direct encoding of the full target. base58
// is positional over the whole input, so a plaintext substring does not appear
// at a predictable place inside the base58 of a larger blob; only whole-value
// matches are meaningful.
func base58Variants(target []byte) []Variant {
	if len(target) == 0 {
		return nil
	}
	return []Variant{{Encoding: "base58", Offset: 0, Pattern: EncodeBase58(target)}}
}
