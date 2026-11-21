package scriptcheck

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/vgmartinez/tripcord/internal/model"
)

func writeTree(t *testing.T, root string, files ...string) {
	t.Helper()
	for _, name := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestHomeMirror(t *testing.T) {
	const settings = "home/.claude/settings.json"
	for _, tc := range []struct {
		name  string
		files []string
		path  string
		want  bool
	}{
		{"dotfiles only", []string{settings, "home/.zshenv", "home/.config/git/config"}, settings, true},
		{"config dir only", []string{settings}, settings, true},
		{"cursor", []string{"home/.cursor/hooks.json", "home/.zshrc"}, "home/.cursor/hooks.json", true},
		{"non-dot entry", []string{settings, "home/.zshenv", "home/index.ts"}, settings, false},
		{"non-dot dir", []string{settings, "home/components/x.ts"}, settings, false},
		{"not top level", []string{"src/home/.claude/settings.json", "src/home/.zshenv"}, "src/home/.claude/settings.json", false},
		{"not a config dir", []string{"home/.pi/settings.json", "home/.zshenv"}, "home/.pi/settings.json", false},
		{"file directly in home", []string{"home/.mcp.json"}, "home/.mcp.json", false},
		{"root config", []string{".claude/settings.json", "home/.zshenv"}, ".claude/settings.json", false},
		{"Home capitalized", []string{"Home/.claude/settings.json"}, "Home/.claude/settings.json", false},
		{"image home", []string{"images/agent/Dockerfile", "images/agent/home/.codex/config.toml", "images/agent/home/.bashrc"}, "images/agent/home/.codex/config.toml", true},
		{"image home Containerfile", []string{"agent/Containerfile", "agent/home/.claude/settings.json"}, "agent/home/.claude/settings.json", true},
		{"image home deep", []string{"a/b/c/Dockerfile", "a/b/c/home/.codex/hooks.json", "a/b/c/home/.profile"}, "a/b/c/home/.codex/hooks.json", true},
		{"image home non-dot entry", []string{"images/agent/Dockerfile", "images/agent/home/.codex/config.toml", "images/agent/home/setup.sh"}, "images/agent/home/.codex/config.toml", false},
		{"nested without image file", []string{"images/agent/build.sh", "images/agent/home/.codex/config.toml"}, "images/agent/home/.codex/config.toml", false},
		{"image file in grandparent", []string{"images/Dockerfile", "images/agent/home/.codex/config.toml"}, "images/agent/home/.codex/config.toml", false},
		{"image file in home", []string{"images/agent/home/Dockerfile", "images/agent/home/.codex/config.toml"}, "images/agent/home/.codex/config.toml", false},
		{"dockerfile lowercase", []string{"images/agent/dockerfile", "images/agent/home/.codex/config.toml"}, "images/agent/home/.codex/config.toml", false},
		{"Dockerfile variant name", []string{"images/agent/Dockerfile.dev", "images/agent/home/.codex/config.toml"}, "images/agent/home/.codex/config.toml", false},
		{"root config beside image home", []string{".claude/settings.json", "images/agent/Dockerfile", "images/agent/home/.codex/config.toml"}, ".claude/settings.json", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeTree(t, root, tc.files...)
			if got := HomeMirror(root, tc.path); got != tc.want {
				t.Fatalf("HomeMirror(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

// A home/ that is itself a symlink is not a mirror: what it holds is
// not in the scanned tree.
func TestHomeMirrorSymlinkedHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	root := t.TempDir()
	writeTree(t, root, "dots/.claude/settings.json", "dots/.zshenv")
	if err := os.Symlink("dots", filepath.Join(root, "home")); err != nil {
		t.Fatal(err)
	}
	if HomeMirror(root, "home/.claude/settings.json") {
		t.Fatal("a symlinked home/ is a mirror")
	}
}

// A Dockerfile that is a directory is not an image build file.
func TestHomeMirrorDockerfileDir(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, "images/agent/Dockerfile/x", "images/agent/home/.codex/config.toml")
	if HomeMirror(root, "images/agent/home/.codex/config.toml") {
		t.Fatal("a Dockerfile directory makes a nested home/ a mirror")
	}
}

// A nested home/ reached through a symlinked directory is not a
// mirror, nor is one whose image build file is a symlink.
func TestHomeMirrorNestedSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	root := t.TempDir()
	writeTree(t, root, "real/agent/Dockerfile", "real/agent/home/.codex/config.toml")
	if err := os.Symlink("real", filepath.Join(root, "images")); err != nil {
		t.Fatal(err)
	}
	if HomeMirror(root, "images/agent/home/.codex/config.toml") {
		t.Fatal("a nested home/ under a symlinked directory is a mirror")
	}
	if !HomeMirror(root, "real/agent/home/.codex/config.toml") {
		t.Fatal("the real nested home/ is not a mirror")
	}
	root = t.TempDir()
	writeTree(t, root, "Dockerfile.real", "agent/home/.codex/config.toml")
	if err := os.Symlink(filepath.Join("..", "Dockerfile.real"), filepath.Join(root, "agent", "Dockerfile")); err != nil {
		t.Fatal(err)
	}
	if HomeMirror(root, "agent/home/.codex/config.toml") {
		t.Fatal("a symlinked Dockerfile makes a nested home/ a mirror")
	}
}

func TestAnnotateHomeMirror(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, "home/.claude/settings.json", "home/.zshenv", ".claude/settings.json",
		"images/agent/Dockerfile", "images/agent/home/.codex/config.toml",
		"src/home/.claude/settings.json", "src/home/.zshenv")
	items := []model.ConfigItem{
		{Harness: model.HarnessClaude, Scope: model.ScopeRepo, Path: "home/.claude/settings.json", Kind: model.ConfigKindHook},
		{Harness: model.HarnessClaude, Scope: model.ScopeRepo, Path: ".claude/settings.json", Kind: model.ConfigKindHook},
		{Harness: model.HarnessClaude, Scope: model.ScopeUser, Path: "home/.claude/settings.json", Kind: model.ConfigKindHook, HomeMirror: true},
		{Harness: model.HarnessCodex, Scope: model.ScopeRepo, Path: "images/agent/home/.codex/config.toml", Kind: model.ConfigKindHook},
		{Harness: model.HarnessClaude, Scope: model.ScopeRepo, Path: "src/home/.claude/settings.json", Kind: model.ConfigKindHook},
	}
	if err := Annotate(root, items); err != nil {
		t.Fatal(err)
	}
	for i, want := range []bool{true, false, false, true, false} {
		if items[i].HomeMirror != want {
			t.Errorf("item %d (%s %s): HomeMirror = %v, want %v", i, items[i].Scope, items[i].Path, items[i].HomeMirror, want)
		}
	}
}
