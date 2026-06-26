package main

import (
	"strings"
	"testing"

	"github.com/defektive/base-grep/internal/search"
)

func TestRenderLineHighlight(t *testing.T) {
	m := search.Match{Pattern: "c2VjcmV0", Line: "alpha c2VjcmV0 omega", Col: 6}

	plain := renderLine(m, false, 0)
	if plain != "alpha c2VjcmV0 omega" {
		t.Errorf("plain render = %q", plain)
	}
	if strings.Contains(plain, "\x1b[") {
		t.Error("plain render should contain no ANSI escapes")
	}

	colored := renderLine(m, true, 0)
	if colored != "alpha "+hiOn+"c2VjcmV0"+hiOff+" omega" {
		t.Errorf("colored render = %q", colored)
	}
}

func TestRenderLineMaxColumns(t *testing.T) {
	long := strings.Repeat("A", 100) + "c2VjcmV0" + strings.Repeat("B", 100)
	m := search.Match{Pattern: "c2VjcmV0", Line: long, Col: 100}

	out := renderLine(m, false, 20)
	if !strings.Contains(out, "c2VjcmV0") {
		t.Errorf("truncated output dropped the match: %q", out)
	}
	if !strings.HasPrefix(out, "…") || !strings.HasSuffix(out, "…") {
		t.Errorf("expected ellipses on both sides: %q", out)
	}
	if len([]rune(out)) > 30 { // ~maxCols + match + ellipses, generously bounded
		t.Errorf("truncation too loose: %d runes", len([]rune(out)))
	}
}

func TestResolveColor(t *testing.T) {
	cases := map[string]bool{"always": true, "never": false}
	for when, want := range cases {
		got, err := resolveColor(when, nil)
		if err != nil {
			t.Fatalf("%s: %v", when, err)
		}
		if got != want {
			t.Errorf("resolveColor(%q) = %v, want %v", when, got, want)
		}
	}
	// auto with a nil (non-terminal) writer must be false.
	if got, _ := resolveColor("auto", nil); got {
		t.Error("auto with no terminal should disable color")
	}
	if _, err := resolveColor("bogus", nil); err == nil {
		t.Error("expected error for invalid color mode")
	}
}
