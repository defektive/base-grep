package search

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/defektive/base-grep/internal/permute"
)

func newSearcher(t *testing.T, target string, minLen int) *Searcher {
	t.Helper()
	return New(permute.Generate([]byte(target)), minLen)
}

func TestSearchBytesFindsAlignedBase64(t *testing.T) {
	// "the password is hunter2" base64 contains the offset-1 encoding of
	// "password" because "password" starts at byte offset 4 (4 % 3 == 1).
	blob := "junk dGhlIHBhc3N3b3JkIGlzIGh1bnRlcjI junk"
	s := newSearcher(t, "password", 4)
	matches := s.SearchBytes("blob", []byte(blob))
	if len(matches) == 0 {
		t.Fatal("expected at least one match")
	}
	var foundB64 bool
	for _, m := range matches {
		if m.Encoding == "base64" {
			foundB64 = true
			if !strings.Contains(blob, m.Pattern) {
				t.Errorf("reported pattern %q not in blob", m.Pattern)
			}
			if got := blob[m.Offset : m.Offset+len(m.Pattern)]; got != m.Pattern {
				t.Errorf("offset %d points at %q, not %q", m.Offset, got, m.Pattern)
			}
		}
	}
	if !foundB64 {
		t.Error("expected a base64 match")
	}
}

func TestMinLenFiltersShortPatterns(t *testing.T) {
	all := New(permute.Generate([]byte("ab")), 0)
	filtered := New(permute.Generate([]byte("ab")), 6)
	if len(filtered.Variants()) >= len(all.Variants()) {
		t.Errorf("min-len did not filter: all=%d filtered=%d",
			len(all.Variants()), len(filtered.Variants()))
	}
	for _, v := range filtered.Variants() {
		if len(v.Pattern) < 6 {
			t.Errorf("pattern %q shorter than min-len", v.Pattern)
		}
	}
}

func TestSearchBytesOverlapping(t *testing.T) {
	// A pattern that overlaps itself should be reported at each start position.
	s := New([]permute.Variant{{Encoding: "test", Pattern: "aa"}}, 0)
	matches := s.SearchBytes("x", []byte("aaaa"))
	if len(matches) != 3 {
		t.Fatalf("overlapping matches = %d, want 3", len(matches))
	}
	for i, m := range matches {
		if m.Offset != i {
			t.Errorf("match %d offset = %d, want %d", i, m.Offset, i)
		}
	}
}

func TestSearchReader(t *testing.T) {
	s := newSearcher(t, "secret", 4)
	r := strings.NewReader("prefix c2VjcmV0 suffix") // base64("secret") == "c2VjcmV0"
	matches, err := s.SearchReader("<stream>", r)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatal("expected matches from reader")
	}
	if matches[0].Source != "<stream>" {
		t.Errorf("source = %q, want <stream>", matches[0].Source)
	}
}

func TestSearchPathFileAndDir(t *testing.T) {
	dir := t.TempDir()
	hit := filepath.Join(dir, "hit.txt")
	miss := filepath.Join(dir, "miss.txt")
	sub := filepath.Join(dir, "nested")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	nestedHit := filepath.Join(sub, "deep.txt")

	if err := os.WriteFile(hit, []byte("xx c2VjcmV0 yy"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(miss, []byte("nothing to see here"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nestedHit, []byte("c2VjcmV0"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := newSearcher(t, "secret", 4)

	// Single file.
	fileMatches, errs := s.SearchPath(hit)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(fileMatches) == 0 {
		t.Error("expected match in single file")
	}

	// Recursive directory.
	dirMatches, errs := s.SearchPath(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	sources := map[string]bool{}
	for _, m := range dirMatches {
		sources[m.Source] = true
	}
	if !sources[hit] || !sources[nestedHit] {
		t.Errorf("recursive walk missed files; sources=%v", sources)
	}
	if sources[miss] {
		t.Error("matched a file that should have no hits")
	}
}

func TestSearchPathParallelDeterministic(t *testing.T) {
	dir := t.TempDir()
	// Many files across nested dirs so the worker pool is actually exercised.
	for i := 0; i < 50; i++ {
		sub := filepath.Join(dir, "d", "e", "f")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		name := filepath.Join(sub, "f"+string(rune('a'+i%26))+string(rune('0'+i/26))+".txt")
		if err := os.WriteFile(name, []byte("pad c2VjcmV0 pad"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := newSearcher(t, "secret", 4)

	// Run several times with different worker counts; results must be identical
	// and stably ordered.
	var prev []Match
	for _, jobs := range []int{1, 2, 8, 0} {
		s.Concurrency = jobs
		got, errs := s.SearchPath(dir)
		if len(errs) != 0 {
			t.Fatalf("jobs=%d: errors %v", jobs, errs)
		}
		if len(got) == 0 {
			t.Fatalf("jobs=%d: no matches", jobs)
		}
		if prev != nil && !sameMatches(prev, got) {
			t.Fatalf("jobs=%d produced different/ordered results than the previous run", jobs)
		}
		prev = got
	}
}

func sameMatches(a, b []Match) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSearchPathMissing(t *testing.T) {
	s := newSearcher(t, "secret", 4)
	_, errs := s.SearchPath(filepath.Join(t.TempDir(), "does-not-exist"))
	if len(errs) == 0 {
		t.Error("expected an error for missing path")
	}
}
