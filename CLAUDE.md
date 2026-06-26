# CLAUDE.md

Guidance for working in this repo. Read this first.

## What this is

`base-grep` is a Go CLI that finds plaintext strings which have been **base64 /
base32 / base58 / base85 encoded** inside files, directories, or input streams —
even when the string is not aligned to the encoder's block boundary. Think of it
as a CTF / defensive-security tool for spotting encoded secrets in data.

Module path: `github.com/defektive/base-grep`. Go 1.26. **Standard library only —
no third-party deps.** Keep it that way unless there's a strong reason.

## The core idea (don't lose this)

Block encodings pack a fixed number of input bytes into a fixed number of output
chars (base64 3→4, base32 5→8, base85 4→5). When the target bytes start
part-way through a block, the encoded output **shifts**. So to find a plaintext
substring inside encoded data you must search every byte **alignment** and keep
only the run of characters fully determined by the target (the "stable middle").

- **base64 / base32** are pure bit-repacking → bit-level stable-substring math
  (`internal/permute/bitlevel.go`): drop leading/trailing chars that straddle
  prefix/suffix bits. `startChar = ceil(prefixBits/bitsPerChar)`,
  `endChar = floor((prefixBits+L*8)/bitsPerChar)`.
- **base85** (ascii85, z85) is positional within a 4-byte block (every char
  depends on all 4 bytes) → whole-block alignment only (`base85.go`). Manual
  encoder, no `z` abbreviation, so blocks are always 5 chars.
- **base58** is big-integer positional over the whole input → no substring
  alignment exists; only the direct full-string encoding is produced.

## Layout

```
main.go                  CLI: flags, orchestration, output rendering, color
internal/permute/        permutation generation (one file per scheme)
  permute.go             public API (Variant, Generate/GenerateFor), RegexpAlternation
  bitlevel.go            generic base64/base32 alignment logic
  base64.go base32.go    RFC 4648 wiring (std + alt alphabets)
  base85.go              ascii85 + z85 (manual fixed-width blocks)
  base58.go              Bitcoin base58 (whole-value, exported EncodeBase58)
internal/search/         scanning files/dirs/streams; line capture
integration_test.go      builds the real binary and drives it
render_test.go           unit tests for CLI render/color helpers
```

## Behavior decisions already made (don't regress these)

- **Unique-pattern collapsing**: variants that produce identical bytes (commonly
  base64 == base64url when the pattern has no `+/`-`-_` chars) are merged in
  `search.New` into one `CompiledPattern` with multiple `Source`s. Data is
  scanned once per distinct pattern; a match carries all candidate encodings and
  prints as `[base64,base64url off=1]`.
- **Parallel directory walk**: `searchDir` uses a bounded worker pool
  (`-jobs`, default `runtime.NumCPU()`). Results are gathered under a mutex and
  **sorted at the end** so output is deterministic regardless of scheduling.
  ~9× faster than serial on a 2k-file tree.
- **grep-like output**: prints the whole line with the match highlighted
  (`Match.Line`/`Col`, rendered in `main.go`). `-color auto|always|never`
  (auto = terminal only). `-max-columns N` truncates long lines to a window
  centered on the match with `…`.
- **`-min-len` default 4**: short patterns cause heavy false positives.
- **Exit codes** follow grep: 0 = match, 1 = no match, 2 = usage/IO error.

## Conventions

- Run `gofmt -w .` before finishing; keep `go vet ./...` clean.
- Tests must pass under `go test -race ./...`.
- The permute unit tests are **property-style**: for every generated variant
  they encode the target embedded at the matching offset inside arbitrary data
  and assert the pattern really appears. Preserve this when adding encodings.
- Keep `Match`/JSON output changes deliberate — they are a public-ish contract.

## Commands

```sh
go build -o base-grep .
go test ./...            # unit + integration
go test -race ./...      # must stay clean
go run . -list secret    # show generated patterns
go run . -regexp secret  # emit an ERE alternation for ripgrep / grep -E
```

## Likely next steps (discussed, not yet done)

- Aho-Corasick single-pass matcher (only worth it for many simultaneous targets;
  Go's `bytes.Index` is already fast for ~dozens of short patterns).
- Binary-file handling (grep-style "Binary file matches" / sanitize control
  bytes in printed context). Currently surrounding context is printed raw.
- Optional `-max-filesize` skip guard (parallel workers each hold one file in
  memory, so peak memory scales with `-jobs`).
