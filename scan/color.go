package scan

import (
	"fmt"
	"os"
)

// ANSI escapes for a highlighted match (bold red, matching grep's default).
const (
	HiOn  = "\x1b[1;31m"
	HiOff = "\x1b[0m"
)

// ResolveColor turns a -color flag value ("always", "never", "auto", or "")
// into a boolean, consulting the terminal for "auto"/"".
func ResolveColor(when string, out *os.File) (bool, error) {
	switch when {
	case "always":
		return true, nil
	case "never":
		return false, nil
	case "auto", "":
		return IsTerminal(out), nil
	default:
		return false, fmt.Errorf("invalid -color %q (want always, never, or auto)", when)
	}
}

// IsTerminal reports whether f refers to a character device (a terminal),
// returning false for a nil file, a pipe, or a regular file.
func IsTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
