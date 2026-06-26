// Package permute generates the set of encoded "patterns" that a plaintext
// string can produce when it appears, at an arbitrary byte alignment, inside a
// larger blob that has been encoded with a given base-N scheme.
//
// The central idea is alignment. Block-based encodings (base64, base32, base85)
// pack a fixed number of input bytes into a fixed number of output characters.
// When the bytes you are searching for begin part-way through one of those
// blocks, the output characters shift. To reliably find a plaintext substring
// inside encoded data you therefore have to search for every alignment variant,
// keeping only the run of output characters that is fully determined by your
// target bytes (the "stable" middle).
//
// base58 is not block-based: it is a big-integer positional encoding where every
// output character depends on the whole input, so only the direct encoding of
// the full string can be produced.
package permute

import (
	"regexp"
	"sort"
	"strings"
)

// Variant is a single searchable pattern derived from the target string.
type Variant struct {
	// Encoding identifies the scheme/alphabet, e.g. "base64", "base64url".
	Encoding string
	// Offset is the byte alignment (0..blockSize-1) the pattern corresponds to.
	// For non-block encodings it is always 0.
	Offset int
	// Pattern is the literal substring to search for in encoded data.
	Pattern string
}

// generator produces the variants for one encoding scheme.
type generator func(target []byte) []Variant

// registry is the ordered list of every supported encoding generator.
var registry = []struct {
	name string
	gen  generator
}{
	{"base64", base64StdVariants},
	{"base64url", base64URLVariants},
	{"base32", base32StdVariants},
	{"base32hex", base32HexVariants},
	{"ascii85", ascii85Variants},
	{"z85", z85Variants},
	{"base58", base58Variants},
}

// Encodings returns the names of all supported encodings, in a stable order.
func Encodings() []string {
	names := make([]string, len(registry))
	for i, e := range registry {
		names[i] = e.name
	}
	return names
}

// Generate returns every alignment variant for target across all encodings.
// Patterns are de-duplicated within each encoding. Empty patterns (which occur
// for very short targets at some offsets) are dropped.
func Generate(target []byte) []Variant {
	return GenerateFor(target, nil)
}

// GenerateFor is like Generate but restricted to the named encodings. A nil or
// empty list means "all encodings". Unknown names are ignored.
func GenerateFor(target []byte, encodings []string) []Variant {
	if len(target) == 0 {
		return nil
	}

	want := map[string]bool{}
	for _, e := range encodings {
		want[e] = true
	}

	var out []Variant
	for _, e := range registry {
		if len(want) > 0 && !want[e.name] {
			continue
		}
		out = append(out, e.gen(target)...)
	}
	return out
}

// dedupe removes variants with duplicate patterns while preserving order, and
// drops any empty patterns.
func dedupe(in []Variant) []Variant {
	seen := map[string]bool{}
	out := in[:0:0]
	for _, v := range in {
		if v.Pattern == "" || seen[v.Pattern] {
			continue
		}
		seen[v.Pattern] = true
		out = append(out, v)
	}
	return out
}

// SortByPatternLen returns a copy of variants sorted longest-pattern-first. This
// is handy for reporting, since longer patterns are higher-confidence matches.
func SortByPatternLen(in []Variant) []Variant {
	out := make([]Variant, len(in))
	copy(out, in)
	sort.SliceStable(out, func(i, j int) bool {
		return len(out[i].Pattern) > len(out[j].Pattern)
	})
	return out
}

// RegexpAlternation returns an extended-regular-expression alternation that
// matches any of the variant patterns, suitable for use with `ripgrep` or
// `grep -E`. Each pattern is regex-escaped (the base85 alphabets in particular
// are full of metacharacters) and de-duplicated. Patterns are ordered
// longest-first so more specific matches come earlier. Returns "" when there are
// no patterns.
func RegexpAlternation(variants []Variant) string {
	seen := map[string]bool{}
	var quoted []string
	for _, v := range SortByPatternLen(variants) {
		if v.Pattern == "" || seen[v.Pattern] {
			continue
		}
		seen[v.Pattern] = true
		quoted = append(quoted, regexp.QuoteMeta(v.Pattern))
	}
	if len(quoted) == 0 {
		return ""
	}
	return "(" + strings.Join(quoted, "|") + ")"
}

// ceilDiv returns ceil(a/b) for non-negative integers.
func ceilDiv(a, b int) int { return (a + b - 1) / b }
