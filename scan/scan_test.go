package scan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// countWord is a trivial per-file matcher used to exercise WalkFiles: it returns
// the byte offset of every occurrence of word in data.
func countWord(word string) func(string, []byte) ([]int, error) {
	return func(_ string, data []byte) ([]int, error) {
		var offs []int
		s := string(data)
		for i := 0; ; {
			j := strings.Index(s[i:], word)
			if j < 0 {
				break
			}
			offs = append(offs, i+j)
			i += j + 1
		}
		return offs, nil
	}
}

func TestWalkFilesSingleFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(f, []byte("xx hit yy hit"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, errs := WalkFiles(f, 0, false, countWord("hit"))
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	if len(res) != 2 {
		t.Fatalf("got %d results, want 2", len(res))
	}
}

func TestWalkFilesRecursiveVsFlat(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "nested")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "top.txt"), []byte("hit"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "deep.txt"), []byte("hit hit"), 0o644); err != nil {
		t.Fatal(err)
	}

	flat, errs := WalkFiles(dir, 0, false, countWord("hit"))
	if len(errs) != 0 {
		t.Fatalf("flat errs: %v", errs)
	}
	if len(flat) != 1 {
		t.Errorf("flat walk = %d hits, want 1 (top.txt only)", len(flat))
	}

	rec, errs := WalkFiles(dir, 0, true, countWord("hit"))
	if len(errs) != 0 {
		t.Fatalf("recursive errs: %v", errs)
	}
	if len(rec) != 3 {
		t.Errorf("recursive walk = %d hits, want 3", len(rec))
	}
}

func TestWalkFilesDeterministicOrder(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 40; i++ {
		name := filepath.Join(dir, "f"+string(rune('a'+i%26))+string(rune('0'+i/26))+".txt")
		if err := os.WriteFile(name, []byte("hit"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Return the path so we can assert ordering is by path regardless of jobs.
	byPath := func(p string, data []byte) ([]string, error) { return []string{p}, nil }

	var prev []string
	for _, jobs := range []int{1, 2, 8, 0} {
		got, errs := WalkFiles(dir, jobs, true, byPath)
		if len(errs) != 0 {
			t.Fatalf("jobs=%d errs: %v", jobs, errs)
		}
		if !sort_IsSorted(got) {
			t.Errorf("jobs=%d: results not sorted by path", jobs)
		}
		if prev != nil && strings.Join(prev, "\n") != strings.Join(got, "\n") {
			t.Errorf("jobs=%d produced a different order", jobs)
		}
		prev = got
	}
}

func sort_IsSorted(ss []string) bool {
	for i := 1; i < len(ss); i++ {
		if ss[i-1] > ss[i] {
			return false
		}
	}
	return true
}

func TestWalkFilesMissingRoot(t *testing.T) {
	_, errs := WalkFiles(filepath.Join(t.TempDir(), "nope"), 0, true, countWord("x"))
	if len(errs) == 0 {
		t.Error("expected an error for a missing root")
	}
}

func TestLineBounds(t *testing.T) {
	data := []byte("first\nsecond line\nthird")
	off := strings.Index(string(data), "line")
	s, e := LineBounds(data, off)
	if got := string(data[s:e]); got != "second line" {
		t.Errorf("LineBounds captured %q, want %q", got, "second line")
	}
}

func TestResolveColor(t *testing.T) {
	if got, _ := ResolveColor("always", nil); !got {
		t.Error("always should be true")
	}
	if got, _ := ResolveColor("never", nil); got {
		t.Error("never should be false")
	}
	if got, _ := ResolveColor("auto", nil); got {
		t.Error("auto with no terminal should be false")
	}
	if _, err := ResolveColor("bogus", nil); err == nil {
		t.Error("expected error for invalid mode")
	}
}
