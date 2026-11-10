package githubaction_test

import (
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type releaseWorkflow struct {
	On struct {
		Push struct {
			Tags []string `yaml:"tags"`
		} `yaml:"push"`
		PullRequest struct {
			Paths []string `yaml:"paths"`
		} `yaml:"pull_request"`
	} `yaml:"on"`
	Permissions map[string]string `yaml:"permissions"`
	Jobs        map[string]struct {
		If          string            `yaml:"if"`
		Needs       any               `yaml:"needs"`
		Permissions map[string]string `yaml:"permissions"`
		Strategy    struct {
			Matrix struct {
				GOOS   []string `yaml:"goos"`
				GOARCH []string `yaml:"goarch"`
			} `yaml:"matrix"`
		} `yaml:"strategy"`
		Steps []actionStep `yaml:"steps"`
	} `yaml:"jobs"`
}

var pinnedUses = regexp.MustCompile(`(?m)^\s*(?:-\s+)?uses:\s+(\S+)@([0-9a-f]{40}) # v[0-9]+\.[0-9]+\.[0-9]+$`)

func readReleaseWorkflow(t *testing.T) (string, releaseWorkflow) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var wf releaseWorkflow
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("parse release.yml: %v", err)
	}
	return string(raw), wf
}

// TestReleaseWorkflowContract keeps release.yml a draft-only, least
// privilege release: a tag attests and drafts, a pull request only
// builds and uploads run artifacts.
func TestReleaseWorkflowContract(t *testing.T) {
	text, wf := readReleaseWorkflow(t)
	if !slices.Equal(wf.On.Push.Tags, []string{"v*"}) {
		t.Errorf("push tags = %v, want [v*]", wf.On.Push.Tags)
	}
	if want := []string{".github/workflows/release.yml", "go.mod", "go.sum", "cmd/**"}; !slices.Equal(wf.On.PullRequest.Paths, want) {
		t.Errorf("pull_request paths = %v, want %v", wf.On.PullRequest.Paths, want)
	}
	if wf.Permissions == nil || len(wf.Permissions) != 0 {
		t.Errorf("top-level permissions = %v, want {}", wf.Permissions)
	}
	wantPerms := map[string]map[string]string{
		"build":     {"contents": "read"},
		"checksums": {},
		"attest":    {"id-token": "write", "attestations": "write"},
		"release":   {"contents": "write"},
	}
	if got := slices.Sorted(maps.Keys(wf.Jobs)); !slices.Equal(got, slices.Sorted(maps.Keys(wantPerms))) {
		t.Fatalf("jobs = %v, want %v", got, slices.Sorted(maps.Keys(wantPerms)))
	}
	for name, want := range wantPerms {
		job := wf.Jobs[name]
		if job.Permissions == nil || !maps.Equal(job.Permissions, want) {
			t.Errorf("job %s permissions = %v, want %v", name, job.Permissions, want)
		}
	}
	for _, name := range []string{"attest", "release"} {
		if got := wf.Jobs[name].If; got != "github.event_name == 'push' && github.ref_type == 'tag'" {
			t.Errorf("job %s runs on %q, want a tag push only", name, got)
		}
	}
	for _, name := range []string{"build", "checksums"} {
		if wf.Jobs[name].If != "" {
			t.Errorf("job %s has if %q; the dry run must run the same build", name, wf.Jobs[name].If)
		}
	}
	if m := wf.Jobs["build"].Strategy.Matrix; !slices.Equal(m.GOOS, []string{"linux", "darwin", "windows"}) || !slices.Equal(m.GOARCH, []string{"amd64", "arm64"}) {
		t.Errorf("build matrix = %v x %v, want linux, darwin, windows x amd64, arm64", m.GOOS, m.GOARCH)
	}

	usesLines := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "uses:") {
			usesLines++
		}
	}
	pinned := pinnedUses.FindAllStringSubmatch(text, -1)
	if len(pinned) != usesLines || usesLines == 0 {
		t.Errorf("%d uses lines, %d pinned to a full sha with a # vX.Y.Z comment", usesLines, len(pinned))
	}
	attests := 0
	for _, name := range slices.Sorted(maps.Keys(wf.Jobs)) {
		for _, step := range wf.Jobs[name].Steps {
			if strings.Contains(step.Run, "${{") {
				t.Errorf("job %s step %q interpolates an expression into run", name, step.Name)
			}
			if strings.HasPrefix(step.Uses, "actions/checkout@") && step.With["persist-credentials"] != "false" {
				t.Errorf("job %s checkout does not set persist-credentials: false", name)
			}
			if strings.HasPrefix(step.Uses, "actions/attest-build-provenance@") {
				attests++
				if name != "attest" {
					t.Errorf("job %s attests; only the tag-only attest job may", name)
				}
			}
			if strings.HasPrefix(step.Uses, "actions/setup-go@") && step.With["cache"] != "false" {
				t.Errorf("job %s setup-go restores a cache", name)
			}
			if strings.Contains(step.Run, "gh release") && name != "release" {
				t.Errorf("job %s runs gh release", name)
			}
		}
	}
	if attests != 1 {
		t.Errorf("%d attest steps, want 1", attests)
	}
	var create string
	for _, step := range wf.Jobs["release"].Steps {
		if strings.Contains(step.Run, "gh release create") {
			create = step.Run
		}
	}
	if !strings.Contains(create, "--draft \\\n") {
		t.Error("gh release create does not pass --draft")
	}
	for _, bad := range []string{"--draft=false", "gh release edit", "draft: false", "--latest", "make_latest"} {
		if strings.Contains(text, bad) {
			t.Errorf("release.yml contains %q; a release is only ever a draft", bad)
		}
	}
	if !strings.Contains(create, "scripts/release-build.sh\" notes \"$TAG\"") {
		t.Error("the release body is not the tag's release notes section")
	}
	build := ""
	for _, step := range wf.Jobs["build"].Steps {
		build += step.Run
	}
	if strings.Count(build, "scripts/release-build.sh build") != 2 || !strings.Contains(build, "cmp ") {
		t.Error("the build job does not build with scripts/release-build.sh and compare a clean rebuild")
	}
}

// TestReleaseBuildFlags keeps the script's build reproducible: no cgo,
// -trimpath, stripped with an empty build ID, the tag and commit linked
// in, and archives with a fixed mtime, owner, and order.
func TestReleaseBuildFlags(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "scripts", "release-build.sh"))
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw)
	for _, want := range []string{
		"CGO_ENABLED=0",
		"go build -trimpath",
		`-ldflags "-s -w -buildid= -X main.version=${version} -X main.commit=${commit}"`,
		"export SOURCE_DATE_EPOCH=$epoch",
		"epoch=$(git log -1 --format=%ct HEAD)",
		`--owner=0 --group=0 --numeric-owner`,
		`--mtime="@${epoch}"`,
		"gzip -n",
		"zip -q -X -D",
		`"$bin" LICENSE NOTICE THIRD_PARTY_NOTICES`,
		"go version -m",
		"git status --porcelain",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("scripts/release-build.sh is missing %q", want)
		}
	}
}
