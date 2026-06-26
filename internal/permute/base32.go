package permute

import "encoding/base32"

var (
	b32Std = base32.StdEncoding.WithPadding(base32.NoPadding)
	b32Hex = base32.HexEncoding.WithPadding(base32.NoPadding)
)

func base32StdVariants(target []byte) []Variant {
	return bitLevelVariants("base32", target, 5, func(b []byte) string {
		return b32Std.EncodeToString(b)
	})
}

func base32HexVariants(target []byte) []Variant {
	return bitLevelVariants("base32hex", target, 5, func(b []byte) string {
		return b32Hex.EncodeToString(b)
	})
}
