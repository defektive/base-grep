package main

import (
	"bytes"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// binPath is the compiled base-grep binary, built once in TestMain.
var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "base-grep-it")
	if err != nil {
		panic(err)
	}
	binPath = filepath.Join(dir, "base-grep")
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		os.RemoveAll(dir)
		panic("build failed: " + err.Error())
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// runBin executes the binary with args and optional stdin, returning stdout,
// stderr and the process exit code.
func runBin(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errBuf
	err := cmd.Run()
	code := 0
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run failed: %v (stderr: %s)", err, errBuf.String())
		}
		code = ee.ExitCode()
	}
	return out.String(), errBuf.String(), code
}

func TestIntegrationFileSearch(t *testing.T) {
	dir := t.TempDir()
	secret := "the password is hunter2"
	encoded := base64.StdEncoding.EncodeToString([]byte(secret))
	file := filepath.Join(dir, "dump.bin")
	if err := os.WriteFile(file, []byte("garbage "+encoded+" trailing"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, stderr, code := runBin(t, "", "password", file)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(out, "base64") {
		t.Errorf("expected a base64 hit, got:\n%s", out)
	}
	if !strings.Contains(out, file) {
		t.Errorf("output missing source path:\n%s", out)
	}
}

func TestIntegrationStdinBase32(t *testing.T) {
	stdin := "header " + base32.StdEncoding.EncodeToString([]byte("topsecret")) + " footer"
	out, stderr, code := runBin(t, stdin, "-encodings", "base32", "topsecret")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(out, "base32") {
		t.Errorf("expected base32 hit, got:\n%s", out)
	}
}

func TestIntegrationJSONOutput(t *testing.T) {
	stdin := "x c2VjcmV0 y" // base64("secret")
	out, stderr, code := runBin(t, stdin, "-json", "secret")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	var matches []struct {
		Source   string `json:"Source"`
		Offset   int    `json:"Offset"`
		Encoding string `json:"Encoding"`
		Pattern  string `json:"Pattern"`
	}
	if err := json.Unmarshal([]byte(out), &matches); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(matches) == 0 {
		t.Fatal("expected at least one JSON match")
	}
}

func TestIntegrationNoMatchExitCode(t *testing.T) {
	out, _, code := runBin(t, "absolutely nothing encoded here", "needle")
	if code != 1 {
		t.Errorf("no-match exit code = %d, want 1", code)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("expected empty output on no match, got %q", out)
	}
}

func TestIntegrationListMode(t *testing.T) {
	out, _, code := runBin(t, "", "-list", "secret")
	if code != 0 {
		t.Fatalf("list exit=%d", code)
	}
	for _, enc := range []string{"base64", "base32", "base58", "ascii85", "z85"} {
		if !strings.Contains(out, enc) {
			t.Errorf("list output missing %s:\n%s", enc, out)
		}
	}
}

func TestIntegrationRegexpMode(t *testing.T) {
	out, _, code := runBin(t, "", "-regexp", "secret")
	if code != 0 {
		t.Fatalf("regexp exit=%d", code)
	}
	re := strings.TrimSpace(out)
	if re == "" {
		t.Fatal("expected a regexp on stdout")
	}
	// The emitted regexp must compile and match real base64 of the target.
	rx, err := regexp.Compile(re)
	if err != nil {
		t.Fatalf("emitted regexp does not compile: %v\n%s", err, re)
	}
	encoded := base64.StdEncoding.EncodeToString([]byte("xx secret yy"))
	if !rx.MatchString(encoded) {
		t.Errorf("emitted regexp did not match %q", encoded)
	}
}

func TestIntegrationUsageError(t *testing.T) {
	_, stderr, code := runBin(t, "")
	if code != 2 {
		t.Errorf("missing-target exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "Usage") {
		t.Errorf("expected usage text, got: %s", stderr)
	}
}
