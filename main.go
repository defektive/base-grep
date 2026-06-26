// Command base-grep computes the base64/base32/base58/base85 alignment
// permutations of a target string and searches for them in files, directories
// or standard input.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/defektive/base-grep/internal/permute"
	"github.com/defektive/base-grep/internal/search"
)

// patternVariants flattens compiled patterns to variants so the permute
// regexp builder (which keys on the pattern text) can consume them.
func patternVariants(pats []search.CompiledPattern) []permute.Variant {
	out := make([]permute.Variant, len(pats))
	for i, p := range pats {
		out[i] = permute.Variant{Pattern: p.Pattern}
	}
	return out
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin *os.File, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("base-grep", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		encCSV    = fs.String("encodings", "", "comma-separated encodings to use (default: all). Available: "+strings.Join(permute.Encodings(), ","))
		minLen    = fs.Int("min-len", 4, "ignore patterns shorter than this many characters")
		jsonOut   = fs.Bool("json", false, "emit matches as JSON")
		listOnly  = fs.Bool("list", false, "print the generated patterns and exit (no search)")
		reOut     = fs.Bool("regexp", false, "print an ERE alternation (for ripgrep / grep -E) and exit (no search)")
		jobs      = fs.Int("jobs", 0, "number of files to search in parallel during a directory walk (0 = number of CPUs)")
		colorWhen = fs.String("color", "auto", "highlight matches in the printed line: always, never, or auto (color only on a terminal)")
		maxCols   = fs.Int("max-columns", 0, "truncate printed lines to this many columns around the match (0 = whole line)")
	)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: base-grep [flags] <target> [path ...]\n\n")
		fmt.Fprintf(stderr, "Searches for encoded permutations of <target>. With no path, reads stdin.\n\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fs.Usage()
		return 2
	}

	target := []byte(fs.Arg(0))
	paths := fs.Args()[1:]

	var encodings []string
	if *encCSV != "" {
		encodings = strings.Split(*encCSV, ",")
	}

	useColor, err := resolveColor(*colorWhen, stdout)
	if err != nil {
		fmt.Fprintln(stderr, "base-grep:", err)
		return 2
	}

	variants := permute.GenerateFor(target, encodings)
	searcher := search.New(variants, *minLen)
	searcher.Concurrency = *jobs

	if *listOnly {
		pats := searcher.Patterns()
		sort.SliceStable(pats, func(i, j int) bool {
			return len(pats[i].Pattern) > len(pats[j].Pattern)
		})
		for _, p := range pats {
			fmt.Fprintf(stdout, "%-22s %s\n", p.Label(), p.Pattern)
		}
		return 0
	}

	if *reOut {
		re := permute.RegexpAlternation(patternVariants(searcher.Patterns()))
		if re == "" {
			fmt.Fprintln(stderr, "base-grep: no patterns to build a regexp from (try a lower -min-len)")
			return 1
		}
		fmt.Fprintln(stdout, re)
		return 0
	}

	var matches []search.Match
	exit := 1 // grep convention: 1 means "no matches"

	if len(paths) == 0 {
		m, err := searcher.SearchReader("<stdin>", stdin)
		if err != nil {
			fmt.Fprintln(stderr, "base-grep:", err)
			return 2
		}
		matches = append(matches, m...)
	}
	for _, p := range paths {
		m, errs := searcher.SearchPath(p)
		for _, e := range errs {
			fmt.Fprintln(stderr, "base-grep:", e)
		}
		matches = append(matches, m...)
	}

	if len(matches) > 0 {
		exit = 0
	}

	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(matches); err != nil {
			fmt.Fprintln(stderr, "base-grep:", err)
			return 2
		}
	} else {
		for _, m := range matches {
			fmt.Fprintf(stdout, "%s:%d: [%s] %s\n", m.Source, m.Offset, m.Label(), renderLine(m, useColor, *maxCols))
		}
	}
	return exit
}

// ANSI escapes for the highlighted match (bold red, matching grep's default).
const (
	hiOn  = "\x1b[1;31m"
	hiOff = "\x1b[0m"
)

// resolveColor turns the -color flag value into a boolean, consulting the
// terminal for "auto".
func resolveColor(when string, out *os.File) (bool, error) {
	switch when {
	case "always":
		return true, nil
	case "never":
		return false, nil
	case "auto", "":
		return isTerminal(out), nil
	default:
		return false, fmt.Errorf("invalid -color %q (want always, never, or auto)", when)
	}
}

func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// renderLine returns the match's line with the matched span highlighted (when
// color is enabled). When maxCols > 0 and the line is longer, it is truncated to
// a window of about maxCols columns centred on the match, with an ellipsis on
// each trimmed side.
func renderLine(m search.Match, color bool, maxCols int) string {
	line := m.Line
	start, end := m.Col, m.Col+len(m.Pattern)
	if start < 0 || end > len(line) { // safety; should not happen
		return line
	}
	prefix, matched, suffix := line[:start], line[start:end], line[end:]

	if maxCols > 0 && len(line) > maxCols {
		budget := maxCols - len(matched)
		if budget < 0 {
			budget = 0
		}
		left := budget / 2
		right := budget - left
		if len(prefix) > left {
			prefix = "…" + prefix[len(prefix)-left:]
		}
		if len(suffix) > right {
			suffix = suffix[:right] + "…"
		}
	}

	if color {
		matched = hiOn + matched + hiOff
	}
	return prefix + matched + suffix
}
