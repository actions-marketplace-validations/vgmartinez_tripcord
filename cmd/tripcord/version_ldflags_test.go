package main

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestVersionLinkedTagAndCommit builds tripcord the way release.yml
// does and checks that version and --version print the tag and the
// commit, and that a build without -X flags still prints dev.
func TestVersionLinkedTagAndCommit(t *testing.T) {
	const (
		tag = "v0.1.0"
		sha = "0123456789abcdef0123456789abcdef01234567"
	)
	cases := []struct {
		name    string
		ldflags string
		want    string
	}{
		{"release", "-s -w -buildid= -X main.version=" + tag + " -X main.commit=" + sha, tag + " (commit " + sha + ")\n"},
		{"plain", "", "dev\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bin := filepath.Join(t.TempDir(), "tripcord")
			build := exec.Command("go", "build", "-trimpath", "-ldflags", c.ldflags, "-o", bin, ".")
			build.Env = append(build.Environ(), "CGO_ENABLED=0")
			if out, err := build.CombinedOutput(); err != nil {
				t.Fatalf("go build: %v\n%s", err, out)
			}
			for _, arg := range []string{"version", "--version"} {
				out, err := exec.Command(bin, arg).Output()
				if err != nil {
					t.Fatalf("tripcord %s: %v", arg, err)
				}
				if string(out) != c.want {
					t.Errorf("tripcord %s = %q, want %q", arg, out, c.want)
				}
			}
		})
	}
}
