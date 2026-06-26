// Package search scans files, directories and input streams for the encoded
// patterns produced by the permute package.
package search

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/defektive/base-grep/internal/permute"
	"github.com/defektive/base-grep/scan"
)

// Source records one encoding/alignment that produces a given pattern. Several
// encodings can yield the same bytes (e.g. base64 and base64url whenever the
// pattern uses none of the two alphabet-specific characters), so a single
// pattern may have several sources.
type Source struct {
	Encoding string
	Offset   int // alignment offset of the variant
}

// CompiledPattern is a unique literal pattern together with every encoding
// source that produces it. The searcher scans for each CompiledPattern exactly
// once, no matter how many encodings collapsed onto it.
type CompiledPattern struct {
	Pattern string
	Sources []Source
}

// Label renders the sources of a pattern, e.g. "base64,base64url off=1".
func (c CompiledPattern) Label() string { return sourcesLabel(c.Sources) }

// Match describes one occurrence of a pattern in a source.
type Match struct {
	Source  string // file path, or the label passed to SearchReader (e.g. "<stdin>")
	Offset  int    // byte offset within the source
	Pattern string // the literal text that matched
	// Sources are all the encoding/alignment combinations that produce Pattern.
	// A match against an ambiguous pattern carries every candidate encoding.
	Sources []Source
	// Line is the full line (newline-delimited) containing the match, with no
	// trailing newline. Col is the byte offset of the match within Line.
	Line string
	Col  int
}

// Encodings returns the distinct encoding names that could produce this match,
// in source order.
func (m Match) Encodings() []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range m.Sources {
		if !seen[s.Encoding] {
			seen[s.Encoding] = true
			out = append(out, s.Encoding)
		}
	}
	return out
}

// Label renders the match's sources, e.g. "base64,base64url off=1".
func (m Match) Label() string { return sourcesLabel(m.Sources) }

// sourcesLabel formats a set of sources. When every source shares one alignment
// offset (the usual case for collapsed patterns) it is shown once; otherwise the
// offset is shown per encoding.
func sourcesLabel(srcs []Source) string {
	if len(srcs) == 0 {
		return "?"
	}
	uniform := true
	for _, s := range srcs[1:] {
		if s.Offset != srcs[0].Offset {
			uniform = false
			break
		}
	}
	if uniform {
		seen := map[string]bool{}
		var encs []string
		for _, s := range srcs {
			if !seen[s.Encoding] {
				seen[s.Encoding] = true
				encs = append(encs, s.Encoding)
			}
		}
		return fmt.Sprintf("%s off=%d", strings.Join(encs, ","), srcs[0].Offset)
	}
	parts := make([]string, len(srcs))
	for i, s := range srcs {
		parts[i] = fmt.Sprintf("%s:%d", s.Encoding, s.Offset)
	}
	return strings.Join(parts, ",")
}

// Searcher holds a compiled set of unique patterns and search options.
type Searcher struct {
	patterns []CompiledPattern

	// Concurrency is the number of files searched in parallel during a recursive
	// directory walk. A value <= 0 means runtime.NumCPU().
	Concurrency int
}

// New builds a Searcher from variants, discarding any whose pattern is shorter
// than minLen characters, then collapsing variants that share an identical
// pattern into a single CompiledPattern. This means the data is scanned only
// once per distinct pattern and each occurrence is reported once, carrying all
// the encodings that could have produced it. Short patterns produce large
// numbers of false positives, so a sensible floor (e.g. 4) is recommended; a
// minLen <= 0 keeps every pattern.
func New(variants []permute.Variant, minLen int) *Searcher {
	idx := map[string]int{}
	var pats []CompiledPattern
	for _, v := range variants {
		if v.Pattern == "" || (minLen > 0 && len(v.Pattern) < minLen) {
			continue
		}
		src := Source{Encoding: v.Encoding, Offset: v.Offset}
		if i, ok := idx[v.Pattern]; ok {
			pats[i].Sources = append(pats[i].Sources, src)
			continue
		}
		idx[v.Pattern] = len(pats)
		pats = append(pats, CompiledPattern{Pattern: v.Pattern, Sources: []Source{src}})
	}
	return &Searcher{patterns: pats}
}

// Patterns returns the unique patterns the searcher will scan for, each with the
// encodings that collapsed onto it.
func (s *Searcher) Patterns() []CompiledPattern { return s.patterns }

// SearchBytes returns every (overlapping) occurrence of any unique pattern in
// data, sorted by offset then pattern.
func (s *Searcher) SearchBytes(source string, data []byte) []Match {
	var matches []Match
	for _, cp := range s.patterns {
		pat := []byte(cp.Pattern)
		from := 0
		for {
			i := bytes.Index(data[from:], pat)
			if i < 0 {
				break
			}
			abs := from + i
			ls, le := scan.LineBounds(data, abs)
			matches = append(matches, Match{
				Source:  source,
				Offset:  abs,
				Pattern: cp.Pattern,
				Sources: cp.Sources,
				Line:    string(data[ls:le]),
				Col:     abs - ls,
			})
			from = abs + 1 // allow overlapping matches
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Offset != matches[j].Offset {
			return matches[i].Offset < matches[j].Offset
		}
		return matches[i].Pattern < matches[j].Pattern
	})
	return matches
}

// SearchReader reads r fully and searches its contents, labelling matches with
// source.
func (s *Searcher) SearchReader(source string, r io.Reader) ([]Match, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", source, err)
	}
	return s.SearchBytes(source, data), nil
}

// SearchFile searches a single file.
func (s *Searcher) SearchFile(path string) ([]Match, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return s.SearchBytes(path, data), nil
}

// SearchPath searches a file, or every regular file under a directory
// (recursively), using the shared scan.WalkFiles worker pool. Unreadable files
// are reported via the errors slice rather than aborting the whole walk. Results
// are sorted for deterministic output regardless of worker scheduling.
func (s *Searcher) SearchPath(path string) (matches []Match, errs []error) {
	matches, errs = scan.WalkFiles(path, s.Concurrency, true, func(p string, data []byte) ([]Match, error) {
		return s.SearchBytes(p, data), nil
	})
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Source != matches[j].Source {
			return matches[i].Source < matches[j].Source
		}
		if matches[i].Offset != matches[j].Offset {
			return matches[i].Offset < matches[j].Offset
		}
		return matches[i].Pattern < matches[j].Pattern
	})
	return matches, errs
}
