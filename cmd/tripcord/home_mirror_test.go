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
