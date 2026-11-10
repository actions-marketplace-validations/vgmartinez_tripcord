package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vgmartinez/tripcord/internal/exitcode"
	"github.com/vgmartinez/tripcord/internal/fpkey"
	"github.com/vgmartinez/tripcord/internal/txtar"
)

// TestDevcontainerCaseVariantEnvExit is the scan of the #125 fixtures.
// Case-variant environment keys are distinct, so a config that only has
// them exits 0. A fetch in one of those configs is still a finding and
// exits 1, not 3. An exact duplicate key stays incomplete and exits 3.
func TestDevcontainerCaseVariantEnvExit(t *testing.T) {
	t.Setenv(fpkey.EnvVar, noCINote)
	root := moduleRoot(t)

	code, stdout, stderr := scanTxtar(t, filepath.Join(root, "internal/adapters/devcontainer/testdata/issue125/case-variant.txtar"))
	if code != int(exitcode.OK) || stderr != "" || strings.Contains(stdout, "case-variant") || strings.Contains(stdout, "incomplete") {
		t.Fatalf("case variants: exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}

	code, stdout, stderr = scanTxtar(t, filepath.Join(root, "internal/adapters/devcontainer/testdata/issue125/case-variant-fetch.txtar"))
	if code != int(exitcode.Findings) || !strings.Contains(stdout, "TC-CFG-009") || strings.Contains(stdout, "case-variant") || strings.Contains(stderr, "incomplete") {
		t.Fatalf("fetch: exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}

	code, stdout, stderr = scanTxtar(t, filepath.Join(root, "internal/adapters/devcontainer/testdata/issue125/duplicate-key.txtar"))
	if code != int(exitcode.Config) || !strings.Contains(stderr, "incomplete") || !strings.Contains(stdout, "duplicate key") || strings.Contains(stdout, "case-variant") {
		t.Fatalf("duplicate: exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
}

func scanTxtar(t *testing.T, archive string) (int, string, string) {
	t.Helper()
	raw, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, file := range txtar.Parse(raw).Files {
		rel, err := txtar.Rel(file.Name)
		if err != nil {
			t.Fatal(err)
		}
		dest := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dest, file.Data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"scan", dir}, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}
