package main

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type gateActionMetadata struct {
	Inputs map[string]struct {
		Required bool   `yaml:"required"`
		Default  string `yaml:"default"`
	} `yaml:"inputs"`
	Outputs map[string]any `yaml:"outputs"`
}

// TestReleaseGateRootActionMatchesPin keeps a release from shipping a
// Marketplace action whose delegate does not match it: the composite
// action at the commit action.yml pins must have the same inputs, with
// the same defaults, and the same outputs as action.yml, and must have
// the fail-on none report-only mode action.yml documents. The pin moves
// in B of the repin chain. It runs only with TRIPCORD_RELEASE_GATE=1.
func TestReleaseGateRootActionMatchesPin(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", rootActionPath))
	if err != nil {
		t.Fatal(err)
	}
	pins := actionPins(t, rootActionPath, string(raw))
	if len(pins) != 1 {
		t.Fatalf("%s pins the composite action %d times, want once", rootActionPath, len(pins))
	}
	var root gateActionMetadata
	if err := yaml.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("TRIPCORD_RELEASE_GATE") != "1" {
		t.Log("release gate not requested; set TRIPCORD_RELEASE_GATE=1 to run it")
		return
	}
	pin := pins[0]
	text, err := gitShowAt(t, pin, ".github/actions/tripcord/action.yml")
	if err != nil {
		t.Fatalf("release gate: the action at %s, pinned by %s, cannot be read in this clone: %v", pin[:7], rootActionPath, err)
	}
	var pinned gateActionMetadata
	if err := yaml.Unmarshal([]byte(text), &pinned); err != nil {
		t.Fatalf("release gate: action.yml@%s: %v", pin[:7], err)
	}
	if got, want := slices.Sorted(maps.Keys(root.Inputs)), slices.Sorted(maps.Keys(pinned.Inputs)); !slices.Equal(got, want) {
		t.Errorf("release gate: %s inputs %v, the action at %s has %v", rootActionPath, got, pin[:7], want)
	}
	for name, in := range pinned.Inputs {
		if got, ok := root.Inputs[name]; ok && got != in {
			t.Errorf("release gate: %s input %s is %+v, the action at %s has %+v", rootActionPath, name, got, pin[:7], in)
		}
	}
	if got, want := slices.Sorted(maps.Keys(root.Outputs)), slices.Sorted(maps.Keys(pinned.Outputs)); !slices.Equal(got, want) {
		t.Errorf("release gate: %s outputs %v, the action at %s has %v", rootActionPath, got, pin[:7], want)
	}
	if !strings.Contains(text, `[ "$TRIPCORD_FAIL_ON" != "none" ]`) || !strings.Contains(text, `[ "$TRIPCORD_FAIL_ON" = "none" ]`) {
		t.Errorf("release gate: the action at %s, pinned by %s, has no fail-on none report-only mode; repin the composite action before tagging", pin[:7], rootActionPath)
	}
}
