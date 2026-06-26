// Package scan provides the reusable, content-agnostic machinery for grep-like
// tools: a bounded-parallel file/directory walker, line-bounds capture, and
// terminal/color helpers. It was lifted out of base-grep's internal packages so
// sibling tools (e.g. jottlr, the JWT grep) can share the same fast directory
// walk and consistent output behaviour.
//
// The walker is generic over the per-file result type, so each tool plugs in
// its own matcher (literal-pattern search, JWT extraction, ...) while reusing
// the concurrency, error collection and deterministic ordering.
package scan

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
)

// fileResult pairs a path with whatever the per-file function produced, so the
// walker can re-order results deterministically after a nondeterministic
// parallel walk.
type fileResult[T any] struct {
	path    string
	results []T
}

// WalkFiles applies fn to the contents of root, returning the flattened results
// and any per-file errors. If root is a regular file, fn is applied to it
// directly. If root is a directory, every regular file is processed by a bounded
// pool of jobs worker goroutines (jobs <= 0 means runtime.NumCPU()); when
// recursive is false only the directory's immediate children are read, otherwise
// the whole tree is walked.
//
// File reads/searches are independent, so running them in parallel overlaps disk
// I/O with CPU work. Results are gathered and returned in a deterministic order
// — sorted by path, preserving the within-file order fn returned — regardless of
// how the workers were scheduled. An unreadable file is reported via errs rather
// than aborting the whole walk.
func WalkFiles[T any](root string, jobs int, recursive bool, fn func(path string, data []byte) ([]T, error)) (results []T, errs []error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, []error{err}
	}
	if !info.IsDir() {
		data, err := os.ReadFile(root)
		if err != nil {
			return nil, []error{err}
		}
		res, err := fn(root, data)
		if err != nil {
			return nil, []error{err}
		}
		return res, nil
	}

	paths, walkErrs := collectFiles(root, recursive)
	errs = append(errs, walkErrs...)

	workers := jobs
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if workers > len(paths) && len(paths) > 0 {
		workers = len(paths)
	}

	var mu sync.Mutex // guards collected and errs
	var collected []fileResult[T]
	addErr := func(e error) {
		mu.Lock()
		errs = append(errs, e)
		mu.Unlock()
	}

	in := make(chan string, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range in {
				data, err := os.ReadFile(p)
				if err != nil {
					addErr(err)
					continue
				}
				res, err := fn(p, data)
				if err != nil {
					addErr(err)
					continue
				}
				if len(res) > 0 {
					mu.Lock()
					collected = append(collected, fileResult[T]{path: p, results: res})
					mu.Unlock()
				}
			}
		}()
	}
	for _, p := range paths {
		in <- p
	}
	close(in)
	wg.Wait()

	sort.SliceStable(collected, func(i, j int) bool {
		return collected[i].path < collected[j].path
	})
	for _, fr := range collected {
		results = append(results, fr.results...)
	}
	return results, errs
}

// collectFiles lists the regular files under root. When recursive is false only
// root's immediate entries are considered; otherwise the whole tree is walked.
// Per-entry errors are collected rather than aborting.
func collectFiles(root string, recursive bool) (paths []string, errs []error) {
	if !recursive {
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, []error{err}
		}
		for _, e := range entries {
			if e.Type().IsRegular() {
				paths = append(paths, filepath.Join(root, e.Name()))
			}
		}
		return paths, nil
	}
	walkErr := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		paths = append(paths, p)
		return nil
	})
	if walkErr != nil {
		errs = append(errs, walkErr)
	}
	return paths, errs
}

// LineBounds returns the [start, end) byte range of the line containing off:
// from just after the previous newline to just before the next one. Callers use
// it to capture the whole line around a match for grep-like output. A match that
// contains no newline lies entirely within this range.
func LineBounds(data []byte, off int) (start, end int) {
	start = bytes.LastIndexByte(data[:off], '\n') + 1 // 0 when there is no prior newline
	if n := bytes.IndexByte(data[off:], '\n'); n >= 0 {
		end = off + n
	} else {
		end = len(data)
	}
	return start, end
}
