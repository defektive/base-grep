// Package search scans files, directories and input streams for the encoded
// patterns produced by the permute package.
package search

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/defektive/base-grep/internal/permute"
)

// Match describes one occurrence of a pattern in a source.
type Match struct {
	Source    string // file path, or the label passed to SearchReader (e.g. "<stdin>")
	Offset    int    // byte offset within the source
	Encoding  string
	VarOffset int    // alignment offset of the matched variant
	Pattern   string // the literal text that matched
}

// Searcher holds a compiled set of patterns and search options.
type Searcher struct {
	variants []permute.Variant
	minLen   int
}

// New builds a Searcher from variants, discarding any whose pattern is shorter
// than minLen characters. Short patterns produce large numbers of false
// positives, so a sensible floor (e.g. 4) is recommended. A minLen <= 0 keeps
// every pattern.
func New(variants []permute.Variant, minLen int) *Searcher {
	kept := make([]permute.Variant, 0, len(variants))
	for _, v := range variants {
		if v.Pattern == "" || (minLen > 0 && len(v.Pattern) < minLen) {
			continue
		}
		kept = append(kept, v)
	}
	return &Searcher{variants: kept, minLen: minLen}
}

// Variants returns the patterns the searcher will look for (after minLen
// filtering).
func (s *Searcher) Variants() []permute.Variant { return s.variants }

// SearchBytes returns every (overlapping) occurrence of any pattern in data,
// sorted by offset then encoding.
func (s *Searcher) SearchBytes(source string, data []byte) []Match {
	var matches []Match
	for _, v := range s.variants {
		pat := []byte(v.Pattern)
		from := 0
		for {
			i := bytes.Index(data[from:], pat)
			if i < 0 {
				break
			}
			abs := from + i
			matches = append(matches, Match{
				Source:    source,
				Offset:    abs,
				Encoding:  v.Encoding,
				VarOffset: v.Offset,
				Pattern:   v.Pattern,
			})
			from = abs + 1 // allow overlapping matches
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Offset != matches[j].Offset {
			return matches[i].Offset < matches[j].Offset
		}
		return matches[i].Encoding < matches[j].Encoding
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
// (recursively). Unreadable files are reported via the errors slice rather than
// aborting the whole walk.
func (s *Searcher) SearchPath(path string) (matches []Match, errs []error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, []error{err}
	}
	if !info.IsDir() {
		m, err := s.SearchFile(path)
		if err != nil {
			return nil, []error{err}
		}
		return m, nil
	}

	walkErr := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		m, err := s.SearchFile(p)
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		matches = append(matches, m...)
		return nil
	})
	if walkErr != nil {
		errs = append(errs, walkErr)
	}
	return matches, errs
}
