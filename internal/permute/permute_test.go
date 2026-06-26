package permute

import (
	"encoding/base32"
	"encoding/base64"
	"math/big"
	"regexp"
	"strings"
	"testing"
)

// embedAt builds a buffer with `prefix` filler bytes, then target, then `suffix`
// filler bytes. Filler is deliberately non-zero, arbitrary data so the test
// proves the stable pattern is independent of surrounding bytes.
func embedAt(prefix int, target []byte, suffix int) []byte {
	buf := make([]byte, 0, prefix+len(target)+suffix)
	for i := 0; i < prefix; i++ {
		buf = append(buf, byte(0x90+i))
	}
	buf = append(buf, target...)
	for i := 0; i < suffix; i++ {
		buf = append(buf, byte(0xA0+i))
	}
	return buf
}

// assertEmbedded checks that every variant's pattern actually appears in the
// full encoding of the target embedded at that variant's alignment offset. This
// is the core correctness property of the permutation logic.
func assertEmbedded(t *testing.T, target []byte, variants []Variant, realEnc func([]byte) string, block int) {
	t.Helper()
	for _, v := range variants {
		// Try a few suffix lengths so we exercise the trailing boundary too.
		found := false
		var encs []string
		for suffix := 0; suffix <= block; suffix++ {
			full := realEnc(embedAt(v.Offset, target, suffix))
			encs = append(encs, full)
			if strings.Contains(full, v.Pattern) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s off=%d: pattern %q not found embedded at offset %d (encodings: %v)",
				v.Encoding, v.Offset, v.Pattern, v.Offset, encs)
		}
	}
}

func TestBase64KnownVector(t *testing.T) {
	// "Hello" -> base64 "SGVsbG8=" ; raw stable middle at offset 0 is the run of
	// chars fully determined by the 5 input bytes: floor(40/6)=6 chars.
	got := base64StdVariants([]byte("Hello"))
	var off0 string
	for _, v := range got {
		if v.Offset == 0 {
			off0 = v.Pattern
		}
	}
	if want := "SGVsbG"; off0 != want {
		t.Fatalf("base64 offset0 = %q, want %q", off0, want)
	}
}

func TestBitLevelEmbedded(t *testing.T) {
	targets := []string{"a", "ab", "secret", "the password is hunter2", "x\x00y\xffz"}
	b64 := base64.StdEncoding.WithPadding(base64.NoPadding)
	b64u := base64.URLEncoding.WithPadding(base64.NoPadding)
	b32 := base32.StdEncoding.WithPadding(base32.NoPadding)
	b32h := base32.HexEncoding.WithPadding(base32.NoPadding)

	for _, ts := range targets {
		target := []byte(ts)
		assertEmbedded(t, target, base64StdVariants(target), b64.EncodeToString, 3)
		assertEmbedded(t, target, base64URLVariants(target), b64u.EncodeToString, 3)
		assertEmbedded(t, target, base32StdVariants(target), b32.EncodeToString, 5)
		assertEmbedded(t, target, base32HexVariants(target), b32h.EncodeToString, 5)
	}
}

func TestBase85Embedded(t *testing.T) {
	targets := []string{"ab", "secret", "alignment matters here", "\x01\x02\x03\x04\x05"}
	for _, ts := range targets {
		target := []byte(ts)
		// Real encoder pads to a 4-byte boundary; interior blocks are unaffected.
		encA := func(b []byte) string {
			padded := make([]byte, ceilDiv(len(b), 4)*4)
			copy(padded, b)
			return encodeBase85Blocks(padded, ascii85Alphabet)
		}
		encZ := func(b []byte) string {
			padded := make([]byte, ceilDiv(len(b), 4)*4)
			copy(padded, b)
			return encodeBase85Blocks(padded, z85Alphabet)
		}
		assertEmbedded(t, target, ascii85Variants(target), encA, 4)
		assertEmbedded(t, target, z85Variants(target), encZ, 4)
	}
}

func TestBase85AlphabetsDiffer(t *testing.T) {
	target := []byte("secret")
	a := ascii85Variants(target)
	z := z85Variants(target)
	if len(a) == 0 || len(z) == 0 {
		t.Fatal("expected variants for both alphabets")
	}
	if a[0].Pattern == z[0].Pattern {
		t.Errorf("ascii85 and z85 produced identical pattern %q", a[0].Pattern)
	}
}

// decodeBase58 is a test-only inverse used to verify the encoder.
func decodeBase58(s string) []byte {
	num := new(big.Int)
	radix := big.NewInt(58)
	for _, c := range s {
		idx := strings.IndexRune(base58Alphabet, c)
		if idx < 0 {
			return nil
		}
		num.Mul(num, radix)
		num.Add(num, big.NewInt(int64(idx)))
	}
	dec := num.Bytes()
	// Restore leading zero bytes.
	zeros := 0
	for zeros < len(s) && s[zeros] == base58Alphabet[0] {
		zeros++
	}
	return append(make([]byte, zeros), dec...)
}

func TestBase58RoundTrip(t *testing.T) {
	cases := []string{"", "a", "hello", "the password is hunter2", "\x00\x00abc", "\x00"}
	for _, c := range cases {
		enc := EncodeBase58([]byte(c))
		dec := decodeBase58(enc)
		if string(dec) != c {
			t.Errorf("base58 round-trip %q: encoded %q decoded %q", c, enc, dec)
		}
	}
}

func TestBase58KnownVector(t *testing.T) {
	// Well-known: base58("Hello World!") == "2NEpo7TZRRrLZSi2U"
	if got := EncodeBase58([]byte("Hello World!")); got != "2NEpo7TZRRrLZSi2U" {
		t.Errorf("base58(Hello World!) = %q", got)
	}
}

func TestRegexpAlternation(t *testing.T) {
	target := []byte("the password is hunter2")
	variants := Generate(target)
	re := RegexpAlternation(variants)

	// Must compile as a Go (RE2/ripgrep-family) regexp.
	rx, err := regexp.Compile(re)
	if err != nil {
		t.Fatalf("generated regexp does not compile: %v\n%s", err, re)
	}

	// It must match real encoded data for several alignments.
	b64 := base64.StdEncoding.EncodeToString(embedAt(1, target, 2))
	if !rx.MatchString(b64) {
		t.Errorf("regexp did not match base64 of embedded target:\n%s", b64)
	}

	// Metacharacters from the base85 alphabets must be escaped, never raw.
	if strings.Contains(re, "(|)") || strings.HasPrefix(re, "*") {
		t.Errorf("regexp looks unescaped: %s", re)
	}
}

func TestRegexpAlternationEmpty(t *testing.T) {
	if got := RegexpAlternation(nil); got != "" {
		t.Errorf("RegexpAlternation(nil) = %q, want empty", got)
	}
}

func TestRegexpAlternationEscapesMeta(t *testing.T) {
	// z85 / ascii85 patterns are rich in regex metacharacters; a literal pattern
	// containing them must round-trip through QuoteMeta so it matches itself.
	v := []Variant{{Encoding: "z85", Offset: 0, Pattern: "a.b*c+(d)"}}
	rx := regexp.MustCompile(RegexpAlternation(v))
	if !rx.MatchString("xx a.b*c+(d) yy") {
		t.Error("escaped pattern failed to match its literal text")
	}
	if rx.MatchString("aXbYcZ d ") {
		t.Error("metacharacters were treated as regex operators")
	}
}

func TestGenerateForFilter(t *testing.T) {
	target := []byte("secret")
	only := GenerateFor(target, []string{"base64"})
	if len(only) == 0 {
		t.Fatal("expected base64 variants")
	}
	for _, v := range only {
		if v.Encoding != "base64" {
			t.Errorf("filter leaked encoding %q", v.Encoding)
		}
	}
	if got := GenerateFor(target, nil); len(got) <= len(only) {
		t.Errorf("all-encodings (%d) should exceed base64-only (%d)", len(got), len(only))
	}
}

func TestGenerateEmptyTarget(t *testing.T) {
	if got := Generate(nil); got != nil {
		t.Errorf("Generate(nil) = %v, want nil", got)
	}
	if got := Generate([]byte{}); got != nil {
		t.Errorf("Generate(empty) = %v, want nil", got)
	}
}

func TestNoEmptyOrDuplicatePatterns(t *testing.T) {
	seen := map[string]map[string]bool{}
	for _, v := range Generate([]byte("the password is hunter2")) {
		if v.Pattern == "" {
			t.Errorf("%s off=%d produced empty pattern", v.Encoding, v.Offset)
		}
		if seen[v.Encoding] == nil {
			seen[v.Encoding] = map[string]bool{}
		}
		if seen[v.Encoding][v.Pattern] {
			t.Errorf("%s produced duplicate pattern %q", v.Encoding, v.Pattern)
		}
		seen[v.Encoding][v.Pattern] = true
	}
}
