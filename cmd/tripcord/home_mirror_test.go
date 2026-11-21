package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vgmartinez/tripcord/internal/fpkey"
)

// homeMirrorSettings is a user-level Claude settings file of the kind a
// dotfiles repository keeps under home/: a broad allow rule (TC-CFG-004),
// the bypass-warning skip (TC-CFG-005), a hook into the owner's home
// directory (TC-CFG-006), and an enabled plugin (TC-CFG-010).
const homeMirrorSettings = `{
  "permissions": {"allow": ["Bash"]},
  "skipDangerousModePermissionPrompt": true,
  "enabledPlugins": {"formatter@example-tools": true},
  "hooks": {
    "Stop": [
      {"matcher": "*", "hooks": [{"type": "command", "command": "/Users/dev/.local/share/notify/venv/bin/python /Users/dev/.local/share/notify/hook.py"}]}
    ]
  }
}
`

type homeMirrorHit struct {
	Rule, Severity, Fingerprint string
}

func scanHomeMirror(t *testing.T, extra map[string]string) []homeMirrorHit {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"home/.claude/settings.json": homeMirrorSettings,
		"home/.zshenv":               "export EDITOR=vim\n",
	}
	for name, body := range extra {
		files[name] = body
	}
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var stdout, stderr bytes.Buffer
	run([]string{"scan", "--format", "json", root}, &stdout, &stderr)
	var hits []homeMirrorHit
	dec := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	for dec.More() {
		var doc struct {
			Schema, Rule, Severity, Fingerprint string
			Location                            struct{ Path string }
		}
		if err := dec.Decode(&doc); err != nil {
			t.Fatalf("findings: %v\n%s", err, stdout.String())
		}
		if doc.Schema != "tripcord.finding/v1" || doc.Location.Path != "home/.claude/settings.json" {
			continue
		}
		hits = append(hits, homeMirrorHit{doc.Rule, doc.Severity, doc.Fingerprint})
	}
	slices.SortFunc(hits, func(a, b homeMirrorHit) int { return strings.Compare(a.Fingerprint, b.Fingerprint) })
	return hits
}

// TestScanHomeMirrorLowersEveryRule pins the dotfiles home/ mirror: every
// finding on home/.claude/settings.json, whatever the rule, is low when
// home/ holds only dot-entries, and keeps its own severity once home/
// holds anything else. The findings and their fingerprints are the same
// either way.
func TestScanHomeMirrorLowersEveryRule(t *testing.T) {
	t.Setenv(fpkey.EnvVar, noCINote)
	mirror := scanHomeMirror(t, nil)
	plain := scanHomeMirror(t, map[string]string{"home/index.ts": "export const page = 1;\n"})
	rules := map[string]bool{}
	for _, h := range mirror {
		rules[h.Rule] = true
		if h.Severity != "low" {
			t.Errorf("mirror: %s severity = %s, want low", h.Rule, h.Severity)
		}
	}
	for _, id := range []string{"TC-CFG-004", "TC-CFG-005", "TC-CFG-006", "TC-CFG-010"} {
		if !rules[id] {
			t.Errorf("mirror: no %s finding; got %v", id, mirror)
		}
	}
	if len(plain) != len(mirror) {
		t.Fatalf("plain has %d findings, mirror %d:\n%v\n%v", len(plain), len(mirror), plain, mirror)
	}
	raised := false
	for i := range plain {
		if plain[i].Rule != mirror[i].Rule || plain[i].Fingerprint != mirror[i].Fingerprint {
			t.Errorf("finding %d: plain %v, mirror %v: rule and fingerprint must not change", i, plain[i], mirror[i])
		}
		if plain[i].Severity != "low" {
			raised = true
		}
		if plain[i].Rule == "TC-CFG-006" && plain[i].Severity != "high" {
			t.Errorf("plain: TC-CFG-006 severity = %s, want high", plain[i].Severity)
		}
	}
	if !raised {
		t.Errorf("plain: every finding is low; home/ with a non-dot entry must not be a mirror: %v", plain)
	}
}

// imageHomeConfig is the user-level Codex config a container image keeps
// in the home/ next to its Dockerfile: approval_policy "never"
// (TC-CFG-005) and a hook into the image user's home directory
// (TC-CFG-006).
const imageHomeConfig = `approval_policy = "never"

[features]
hooks = true

[[hooks.Stop]]
matcher = ""

[[hooks.Stop.hooks]]
type = "command"
command = "sh /home/agent/.codex/notify.sh"
`

func scanImageHome(t *testing.T, extra map[string]string) []homeMirrorHit {
	t.Helper()
	const cfg = "images/agent/home/.codex/config.toml"
	root := t.TempDir()
	files := map[string]string{
		"images/agent/Dockerfile":            "FROM node:22-slim\nCOPY home/ /home/agent/\n",
		"images/agent/home/.bashrc":          "export EDITOR=vim\n",
		cfg:                                  imageHomeConfig,
		"images/agent/home/.codex/notify.sh": "exit 0\n",
	}
	for name, body := range extra {
		files[name] = body
	}
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var stdout, stderr bytes.Buffer
	run([]string{"scan", "--format", "json", root}, &stdout, &stderr)
	var hits []homeMirrorHit
	dec := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	for dec.More() {
		var doc struct {
			Schema, Rule, Severity, Fingerprint string
			Location                            struct{ Path string }
		}
		if err := dec.Decode(&doc); err != nil {
			t.Fatalf("findings: %v\n%s", err, stdout.String())
		}
		if doc.Schema != "tripcord.finding/v1" || doc.Location.Path != cfg {
			continue
		}
		hits = append(hits, homeMirrorHit{doc.Rule, doc.Severity, doc.Fingerprint})
	}
	slices.SortFunc(hits, func(a, b homeMirrorHit) int { return strings.Compare(a.Fingerprint, b.Fingerprint) })
	return hits
}

// TestScanImageHomeMirrorLowersEveryRule pins the nested home/ of a
// container image: with a Dockerfile beside it and only dot-entries in
// it, TC-CFG-005 and TC-CFG-006 report the Codex config at low; with a
// non-dot entry in home/ they are high again. Rule and fingerprint do
// not change.
func TestScanImageHomeMirrorLowersEveryRule(t *testing.T) {
	t.Setenv(fpkey.EnvVar, noCINote)
	mirror := scanImageHome(t, nil)
	plain := scanImageHome(t, map[string]string{"images/agent/home/setup.sh": "echo setup\n"})
	rules := map[string]bool{}
	for _, h := range mirror {
		rules[h.Rule] = true
		if h.Severity != "low" {
			t.Errorf("mirror: %s severity = %s, want low", h.Rule, h.Severity)
		}
	}
	for _, id := range []string{"TC-CFG-005", "TC-CFG-006"} {
		if !rules[id] {
			t.Errorf("mirror: no %s finding; got %v", id, mirror)
		}
	}
	if len(plain) != len(mirror) {
		t.Fatalf("plain has %d findings, mirror %d:\n%v\n%v", len(plain), len(mirror), plain, mirror)
	}
	for i := range plain {
		if plain[i].Rule != mirror[i].Rule || plain[i].Fingerprint != mirror[i].Fingerprint {
			t.Errorf("finding %d: plain %v, mirror %v: rule and fingerprint must not change", i, plain[i], mirror[i])
		}
		if plain[i].Severity != "high" {
			t.Errorf("plain: %s severity = %s, want high", plain[i].Rule, plain[i].Severity)
		}
	}
}
