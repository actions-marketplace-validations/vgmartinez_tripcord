package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestScanStagedDotIsRepositoryRoot checks that "." with --staged, before
// or after the flag, names the repository root and still scans every
// staged change, and that a path that escapes the repository, or "."
// from a subdirectory, fails closed.
func TestScanStagedDotIsRepositoryRoot(t *testing.T) {
	bin := buildStagedBinary(t)
	repo := stagedRepo(t)
	if code, out := stagedScan(t, bin, repo, "."); code != 0 {
		t.Fatalf("clean scan --staged . = %d, want 0\n%s", code, out)
	}
	stagedWrite(t, repo, ".claude/settings.json", stagedBad)
	stagedGit(t, repo, "add", ".claude/settings.json")
	for _, args := range [][]string{{"scan", ".", "--staged"}, {"scan", "--staged", "."}, {"scan", "--staged", "./"}} {
		code, out := runIn(t, bin, repo, args...)
		if code != 1 || !strings.Contains(out, "TC-CFG-001") {
			t.Fatalf("%q = %d, want 1 with TC-CFG-001\n%s", args, code, out)
		}
	}
	for _, path := range []string{"..", "../x", "a/../../x"} {
		code, out := stagedScan(t, bin, repo, path)
		if code != 3 || !strings.Contains(out, "escapes the repository") || !strings.Contains(out, "blocked the commit") {
			t.Fatalf("--staged %q = %d, want 3 escaping\n%s", path, code, out)
		}
	}
	sub := filepath.Join(repo, "sub")
	stagedWrite(t, repo, "sub/README", "x\n")
	if code, out := stagedScan(t, bin, sub, "."); code != 3 || !strings.Contains(out, "is not the repository root") {
		t.Fatalf("--staged . from a subdirectory = %d, want 3\n%s", code, out)
	}
}

func runIn(t *testing.T, bin, dir string, args ...string) (int, string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	if exit, ok := err.(*exec.ExitError); ok {
		return exit.ExitCode(), string(out)
	}
	t.Fatal(err)
	return -1, ""
}
