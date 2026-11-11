package githubaction_test

import (
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

type metadataInput struct {
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
	Default     string `yaml:"default"`
}

type metadataOutput struct {
	Description string `yaml:"description"`
	Value       string `yaml:"value"`
}

type actionMetadata struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Branding    struct {
		Icon  string `yaml:"icon"`
		Color string `yaml:"color"`
	} `yaml:"branding"`
	Inputs  map[string]metadataInput  `yaml:"inputs"`
	Outputs map[string]metadataOutput `yaml:"outputs"`
	Runs    struct {
		Using string       `yaml:"using"`
		Steps []actionStep `yaml:"steps"`
	} `yaml:"runs"`
}

func readActionMetadata(t *testing.T, path string) (string, actionMetadata) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m actionMetadata
	if err := yaml.Unmarshal(raw, &m); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return string(raw), m
}

// rootDelegate is the root action's one step: this repository's
// composite action at a full commit SHA, with a # main@ comment.
var rootDelegate = regexp.MustCompile(`(?m)^      uses: vgmartinez/tripcord/\.github/actions/tripcord@([0-9a-f]{40}) # main@([0-9a-f]{7,40}) # zizmor: ignore\[ref-version-mismatch\]$`)

var reusableActionPin = regexp.MustCompile(`uses: vgmartinez/tripcord/\.github/actions/tripcord@([0-9a-f]{40})`)

// TestRootActionIsTheOnlyRootActionFile keeps one Marketplace action:
// action.yml at the repository root and no action.yaml beside it.
func TestRootActionIsTheOnlyRootActionFile(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, e := range entries {
		switch strings.ToLower(e.Name()) {
		case "action.yml", "action.yaml":
			found = append(found, e.Name())
		}
	}
	if !slices.Equal(found, []string{"action.yml"}) {
		t.Fatalf("root action files = %v, want [action.yml]", found)
	}
}

// TestRootActionMetadata checks what the Marketplace lists: the name,
// a description of at most 125 characters, and branding.
func TestRootActionMetadata(t *testing.T) {
	_, root := readActionMetadata(t, filepath.Join("..", "..", "action.yml"))
	if root.Name != "Tripcord Security" {
		t.Errorf("name = %q, want Tripcord Security", root.Name)
	}
	if n := utf8.RuneCountInString(root.Description); n == 0 || n > 125 {
		t.Errorf("description is %d characters, want 1 to 125", n)
	}
	colors := []string{"white", "black", "yellow", "blue", "green", "orange", "red", "purple", "gray-dark"}
	if root.Branding.Icon == "" || !slices.Contains(colors, root.Branding.Color) {
		t.Errorf("branding = %+v, want an icon and one of %v", root.Branding, colors)
	}
}

// TestRootActionDelegates keeps the root action a thin wrapper: the same
// inputs and outputs as .github/actions/tripcord, one step that calls
// that action by repository and commit (a uses: ./ path would resolve in
// the caller's workspace), every input forwarded unchanged, and every
// output read from that step. The commit is the one both reusable
// workflows pin, so the repin chain moves all three together.
func TestRootActionDelegates(t *testing.T) {
	text, root := readActionMetadata(t, filepath.Join("..", "..", "action.yml"))
	_, nested := readActionMetadata(t, filepath.Join("..", "..", ".github", "actions", "tripcord", "action.yml"))
	if root.Runs.Using != "composite" {
		t.Fatalf("runs.using = %q, want composite", root.Runs.Using)
	}
	if got, want := slices.Sorted(maps.Keys(root.Inputs)), slices.Sorted(maps.Keys(nested.Inputs)); !slices.Equal(got, want) {
		t.Fatalf("inputs = %v, want %v", got, want)
	}
	for name, in := range nested.Inputs {
		if root.Inputs[name] != in {
			t.Errorf("input %s = %+v, want %+v", name, root.Inputs[name], in)
		}
	}
	if got, want := slices.Sorted(maps.Keys(root.Outputs)), slices.Sorted(maps.Keys(nested.Outputs)); !slices.Equal(got, want) {
		t.Fatalf("outputs = %v, want %v", got, want)
	}
	for name, out := range nested.Outputs {
		got := root.Outputs[name]
		if got.Description != out.Description {
			t.Errorf("output %s description = %q, want %q", name, got.Description, out.Description)
		}
		if want := "${{ steps.tripcord.outputs." + name + " }}"; got.Value != want {
			t.Errorf("output %s value = %q, want %q", name, got.Value, want)
		}
	}
	if len(root.Runs.Steps) != 1 {
		t.Fatalf("root action has %d steps, want 1", len(root.Runs.Steps))
	}
	step := root.Runs.Steps[0]
	if step.ID != "tripcord" || step.Run != "" || step.If != "" || step.ContinueOnError || len(step.Env) != 0 {
		t.Fatalf("root step = %+v, want only id tripcord, uses, and with", step)
	}
	m := rootDelegate.FindStringSubmatch(text)
	if m == nil || !strings.HasPrefix(m[1], m[2]) || step.Uses != "vgmartinez/tripcord/.github/actions/tripcord@"+m[1] {
		t.Fatalf("root step uses %q, want vgmartinez/tripcord/.github/actions/tripcord@<sha> # main@<prefix>", step.Uses)
	}
	wantWith := map[string]string{}
	for name := range nested.Inputs {
		wantWith[name] = "${{ inputs." + name + " }}"
	}
	if !maps.Equal(step.With, wantWith) {
		t.Errorf("root step with = %v, want %v", step.With, wantWith)
	}
	for _, name := range []string{"tripcord-pr.yml", "tripcord-pr-comment.yml"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", name))
		if err != nil {
			t.Fatal(err)
		}
		for _, pin := range reusableActionPin.FindAllStringSubmatch(string(raw), -1) {
			if pin[1] != m[1] {
				t.Errorf("%s pins the action at %s, the root action at %s; repin them together", name, pin[1], m[1])
			}
		}
	}
}
