// Command base-grep computes the base64/base32/base58/base85 alignment
// permutations of a target string and searches for them in files, directories
// or standard input.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/defektive/base-grep/internal/permute"
	"github.com/defektive/base-grep/internal/search"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin *os.File, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("base-grep", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		encCSV   = fs.String("encodings", "", "comma-separated encodings to use (default: all). Available: "+strings.Join(permute.Encodings(), ","))
		minLen   = fs.Int("min-len", 4, "ignore patterns shorter than this many characters")
		jsonOut  = fs.Bool("json", false, "emit matches as JSON")
		listOnly = fs.Bool("list", false, "print the generated patterns and exit (no search)")
		reOut    = fs.Bool("regexp", false, "print an ERE alternation (for ripgrep / grep -E) and exit (no search)")
		jobs     = fs.Int("jobs", 0, "number of files to search in parallel during a directory walk (0 = number of CPUs)")
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

	variants := permute.GenerateFor(target, encodings)
	searcher := search.New(variants, *minLen)
	searcher.Concurrency = *jobs

	if *listOnly {
		for _, v := range permute.SortByPatternLen(searcher.Variants()) {
			fmt.Fprintf(stdout, "%-10s off=%d  %s\n", v.Encoding, v.Offset, v.Pattern)
		}
		return 0
	}

	if *reOut {
		re := permute.RegexpAlternation(searcher.Variants())
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
			fmt.Fprintf(stdout, "%s:%d: [%s off=%d] %s\n", m.Source, m.Offset, m.Encoding, m.VarOffset, m.Pattern)
		}
	}
	return exit
}
