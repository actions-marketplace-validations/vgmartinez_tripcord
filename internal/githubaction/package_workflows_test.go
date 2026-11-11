package githubaction_test

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type packageWorkflow struct {
	On struct {
		Release struct {
			Types []string `yaml:"types"`
		} `yaml:"release"`
		WorkflowDispatch struct {
			Inputs map[string]struct {
				Required bool   `yaml:"required"`
				Type     string `yaml:"type"`
			} `yaml:"inputs"`
		} `yaml:"workflow_dispatch"`
	} `yaml:"on"`
	Permissions map[string]string `yaml:"permissions"`
	Jobs        map[string]struct {
		If          string            `yaml:"if"`
		Environment string            `yaml:"environment"`
		Permissions map[string]string `yaml:"permissions"`
		Steps       []actionStep      `yaml:"steps"`
	} `yaml:"jobs"`
}

// TestPackageWorkflowsContract keeps the homebrew, npm, and pypi
// workflows thin and least privilege: they run for a published release
// or a dispatched tag, check the release with scripts/release-fetch.sh,
// never build tripcord, hold id-token: write only in the job that
// publishes, and publish only from a protected environment with no
// stored registry token.
func TestPackageWorkflowsContract(t *testing.T) {
	type jobWant struct {
		env   string
		perms map[string]string
	}
	read := map[string]string{"contents": "read", "attestations": "read"}
	want := map[string]map[string]jobWant{
		"homebrew.yml": {"formula": {"homebrew", read}},
		"npm.yml":      {"stage": {"", read}, "publish": {"npm", map[string]string{"id-token": "write"}}},
		"pypi.yml":     {"build": {"", read}, "publish": {"pypi", map[string]string{"id-token": "write"}}},
	}
	const guard = "github.repository == 'vgmartinez/tripcord' && (github.event_name == 'workflow_dispatch' || !github.event.release.prerelease)"
	for file, jobs := range want {
		raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", file))
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
		var wf packageWorkflow
		if err := yaml.Unmarshal(raw, &wf); err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		if !slices.Equal(wf.On.Release.Types, []string{"published"}) {
			t.Errorf("%s: release types = %v, want [published]", file, wf.On.Release.Types)
		}
		if in, ok := wf.On.WorkflowDispatch.Inputs["tag"]; !ok || !in.Required || in.Type != "string" || len(wf.On.WorkflowDispatch.Inputs) != 1 {
			t.Errorf("%s: workflow_dispatch needs exactly one required string input, tag", file)
		}
		if wf.Permissions == nil || len(wf.Permissions) != 0 {
			t.Errorf("%s: top-level permissions = %v, want {}", file, wf.Permissions)
		}
		if got := slices.Sorted(maps.Keys(wf.Jobs)); !slices.Equal(got, slices.Sorted(maps.Keys(jobs))) {
			t.Fatalf("%s: jobs = %v", file, got)
		}
		fetches := 0
		for name, w := range jobs {
			job := wf.Jobs[name]
			if job.Environment != w.env {
				t.Errorf("%s job %s environment = %q, want %q", file, name, job.Environment, w.env)
			}
			if job.Permissions == nil || !maps.Equal(job.Permissions, w.perms) {
				t.Errorf("%s job %s permissions = %v, want %v", file, name, job.Permissions, w.perms)
			}
			if w.perms["id-token"] == "" && job.If != guard {
				t.Errorf("%s job %s if = %q, want the repository and prerelease guard", file, name, job.If)
			}
			for _, step := range job.Steps {
				if strings.Contains(step.Run, "${{") {
					t.Errorf("%s job %s step %q interpolates an expression into run", file, name, step.Name)
				}
				if strings.Contains(step.Run, "scripts/release-fetch.sh") {
					fetches++
				}
				for _, bad := range []string{"go build", "release-build.sh", "go install"} {
					if strings.Contains(step.Run, bad) {
						t.Errorf("%s job %s builds tripcord (%s)", file, name, bad)
					}
				}
				switch {
				case strings.HasPrefix(step.Uses, "actions/checkout@") && step.With["persist-credentials"] != "false":
					t.Errorf("%s job %s checkout keeps credentials", file, name)
				case strings.HasPrefix(step.Uses, "actions/setup-go@") && step.With["cache"] != "false":
					t.Errorf("%s job %s setup-go restores a cache", file, name)
				case strings.HasPrefix(step.Uses, "actions/setup-node@") && step.With["package-manager-cache"] != "false":
					t.Errorf("%s job %s setup-node restores a cache", file, name)
				}
			}
		}
		if fetches != 1 {
			t.Errorf("%s runs scripts/release-fetch.sh %d times, want once", file, fetches)
		}
		usesLines := strings.Count(text, "uses:")
		if pinned := pinnedUses.FindAllString(text, -1); len(pinned) != usesLines || usesLines == 0 {
			t.Errorf("%s: %d uses lines, %d pinned to a full sha with a # vX.Y.Z comment", file, usesLines, len(pinned))
		}
		for _, bad := range []string{"NPM_TOKEN", "NODE_AUTH_TOKEN", "PYPI_API_TOKEN", "password:", "--force", "registry-url", "python -m build", "pull_request_target"} {
			if strings.Contains(text, bad) {
				t.Errorf("%s contains %q", file, bad)
			}
		}
	}

	brew, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "homebrew.yml"))
	if err != nil {
		t.Fatal(err)
	}
	b := string(brew)
	for _, s := range []string{
		"secrets.HOMEBREW_TAP_TOKEN",
		`if [ -z "$TAP_TOKEN" ]; then`,
		`push --quiet origin "HEAD:refs/heads/${branch}"`,
		`gh pr create --repo "$tap" --base "$base" --head "$branch"`,
		`[ "$base" = "$branch" ]`,
	} {
		if !strings.Contains(b, s) {
			t.Errorf("homebrew.yml is missing %s", s)
		}
	}
	if strings.Count(b, " push ") != 1 {
		t.Error("homebrew.yml pushes more than the one pull request branch")
	}
	npm, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "npm.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(npm), `npm publish "${RUNNER_TEMP}/tarball/tripcord-${version}.tgz" --provenance --access public`) {
		t.Error("npm.yml does not publish the staged tarball with --provenance --access public")
	}
}
