# base-grep

Find plaintext strings that have been **base64 / base32 / base58 / base85
encoded** inside files, directories, or input streams — even when the string is
not aligned to the encoder's block boundary.

## Why alignment matters

Block encodings pack a fixed number of input bytes into a fixed number of output
characters (base64: 3→4, base32: 5→8, base85: 4→5). When the bytes you are
hunting for begin part-way through one of those blocks, the encoded output
**shifts**, so a naïve `grep "$(echo -n secret | base64)"` misses it.

`base-grep` generates every alignment *permutation* of your target and keeps the
run of characters that is fully determined by your bytes (the "stable middle"),
then searches for all of them at once.

Permutations that produce the **same** bytes are collapsed, so each unique
pattern is scanned and reported only once. For example base64 and base64url are
identical whenever the pattern uses none of their two differing characters
(`+/` vs `-_`), so a hit is reported as one match carrying both encodings:

```
dump.bin:42: [base64,base64url off=0] c2VjcmV0
```

```
target:  "the password is hunter2"
base64:  dGhlIHBhc3N3b3JkIGlzIGh1bnRlcjI
                     ^^^^^^^^^^
"password" lives here at byte offset 4 (4 % 3 == 1), so it only appears via the
offset-1 base64 permutation: "Bhc3N3b3Jk"
```

| Encoding   | Block | Permutations | Notes                                   |
|------------|-------|--------------|-----------------------------------------|
| base64     | 3→4   | offsets 0–2  | standard + URL-safe alphabets           |
| base32     | 5→8   | offsets 0–4  | standard + extended-hex alphabets       |
| ascii85/z85| 4→5   | offsets 0–3  | whole-block alignment only              |
| base58     | n/a   | direct only  | positional; substrings don't align      |

base58 is a big-integer positional encoding — every output character depends on
the whole input — so only the direct encoding of the full string is meaningful.

## Build

```sh
go build -o base-grep .
```

## Usage

```sh
base-grep [flags] <target> [path ...]
```

With no path, `base-grep` reads from standard input.

```sh
# Search a file (and see which encoding/alignment matched)
base-grep password ./dump.bin

# Recurse a directory
base-grep "AKIA" ./logs/

# Pipe a stream in
curl -s https://example.com/ | base-grep secret

# Restrict to specific encodings
base-grep -encodings base64,base32 topsecret ./capture.pcap

# Just print the patterns it would search for
base-grep -list secret

# Emit a regexp and hand off to ripgrep / grep -E
rg "$(base-grep -regexp password)" ./logs/
base-grep -regexp password | xargs -0 ... # the regexp is a single line on stdout

# Machine-readable output
base-grep -json secret ./dump.bin
```

### Flags

| Flag          | Default | Description                                        |
|---------------|---------|----------------------------------------------------|
| `-encodings`  | all     | comma-separated list of encodings to use           |
| `-min-len`    | `4`     | ignore patterns shorter than N chars (cuts noise)  |
| `-json`       | false   | emit matches as JSON                                |
| `-list`       | false   | print generated patterns and exit (no search)      |
| `-regexp`     | false   | print an ERE alternation for ripgrep / `grep -E`    |
| `-jobs`       | `0`     | files searched in parallel on a dir walk (0 = #CPUs)|

## Performance

Recursive directory searches run files through a bounded pool of worker
goroutines (`-jobs`, defaulting to the CPU count). Because each file is read and
scanned independently, this overlaps disk I/O with CPU work and uses every core;
on a 2,000-file tree it is roughly an order of magnitude faster than a serial
walk. Results are gathered and sorted at the end, so output stays deterministic
regardless of `-jobs`.

Per file, each pattern is matched with Go's assembly-optimized `bytes.Index`.
With only a couple dozen short patterns this single-pass-per-pattern approach is
already fast; a single-pass multi-pattern automaton (Aho-Corasick) would mainly
help if you search for many targets at once. Note that higher `-jobs` raises peak
memory, since each worker reads a whole file into memory at a time.

Exit status follows `grep` convention: `0` = match found, `1` = no match,
`2` = usage/IO error.

## Project layout

```
main.go                       CLI: flag parsing, search orchestration, output
internal/permute/             permutation generation
  permute.go                  public API + registry
  bitlevel.go                 generic base64/base32 alignment logic
  base64.go base32.go         RFC 4648 wiring (std + alt alphabets)
  base85.go                   ascii85 + z85 (manual, fixed-width blocks)
  base58.go                   Bitcoin base58 (whole-value)
internal/search/              scanning files/dirs/streams for patterns
integration_test.go           builds the binary and exercises the real CLI
```

## Tests

```sh
go test ./...            # unit + integration
go test -race ./...      # race detector
go test -cover ./...     # coverage
```

The unit tests in `internal/permute` use a property-style check: for every
generated permutation they encode the target embedded at the corresponding byte
offset inside arbitrary surrounding data and assert the pattern really appears.
Integration tests compile the binary and drive it via files, stdin, JSON output,
and exit codes.
```
