package permute

import "encoding/base64"

var (
	b64Std = base64.StdEncoding.WithPadding(base64.NoPadding)
	b64URL = base64.URLEncoding.WithPadding(base64.NoPadding)
)

func base64StdVariants(target []byte) []Variant {
	return bitLevelVariants("base64", target, 6, func(b []byte) string {
		return b64Std.EncodeToString(b)
	})
}

func base64URLVariants(target []byte) []Variant {
	return bitLevelVariants("base64url", target, 6, func(b []byte) string {
		return b64URL.EncodeToString(b)
	})
}
