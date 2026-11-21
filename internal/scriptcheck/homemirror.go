package scriptcheck

import (
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/vgmartinez/tripcord/internal/model"
)

// homeMirrorDir is the directory a repository mirrors a home directory
// into. A dotfiles repository keeps it at the top level, where GNU Stow,
// rcm, and similar tools link home/.zshenv to ~/.zshenv and
// home/.claude/settings.json to ~/.claude/settings.json. A container
// image keeps one next to its Dockerfile, which copies it into the
// image user's home directory.
const homeMirrorDir = "home"

// imageFiles name a container image build file. A nested home/ is a
// mirror only when its parent directory holds one of them.
var imageFiles = []string{"Dockerfile", "Containerfile"}

// HomeMirror reports whether relPath, a repository-relative slash path,
// is inside a harness config directory (one of configDirs) directly
// under a home/ mirror. resolvedRoot is the scanned repository with its
// own symlinks resolved. A home/ is a mirror when it is a real
// directory, not a symlink, and every direct child is a dot-entry, and
// either:
//
//   - it is at the top level (home/.claude), as in a dotfiles
//     repository; or
//   - it is nested (images/agent/home/.codex) and its parent directory
//     holds a regular file named Dockerfile or Containerfile, the home
//     directory of a container image's user. No directory on the way
//     to it may be a symlink.
//
// Any error reading it, an empty home/, or a single entry whose name
// does not start with "." means it is not a mirror. A nested home/
// without an image build file beside it (src/home/.claude) is not a
// mirror either.
//
// The shape is cheap to fake: anyone can commit home/.claude next to a
// home/.zshenv, and a Dockerfile next to that. That is why a match on
// such an item is lowered to low rather than dropped, so it stays in
// the report.
func HomeMirror(resolvedRoot, relPath string) bool {
	home, ok := homeConfigDir(relPath)
	return ok && homeMirrorAt(resolvedRoot, home)
}

// homeConfigDir returns the home/ directory of relPath when relPath is
// <prefix>home/<config dir>/<more>, with <prefix> empty or a directory
// path. The first such home/ segment counts.
func homeConfigDir(relPath string) (string, bool) {
	parts := strings.Split(relPath, "/")
	for i := 0; i+2 < len(parts); i++ {
		if parts[i] == homeMirrorDir && slices.Contains(configDirs, parts[i+1]) && strings.Join(parts[i+2:], "/") != "" {
			return strings.Join(parts[:i+1], "/"), true
		}
	}
	return "", false
}

// homeMirrorAt reports whether home, a repository-relative slash path
// ending in home, is a mirror (see HomeMirror).
func homeMirrorAt(resolvedRoot, home string) bool {
	parent := path.Dir(home)
	if parent != "." {
		if !realDirs(resolvedRoot, parent) || !hasImageFile(filepath.Join(resolvedRoot, filepath.FromSlash(parent))) {
			return false
		}
	}
	dir := filepath.Join(resolvedRoot, filepath.FromSlash(home))
	st, err := os.Lstat(dir)
	if err != nil || !st.IsDir() {
		return false
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		return false
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			return false
		}
	}
	return true
}

// realDirs reports whether every component of rel, a slash path under
// resolvedRoot, is a directory and not a symlink.
func realDirs(resolvedRoot, rel string) bool {
	p := resolvedRoot
	for _, part := range strings.Split(rel, "/") {
		p = filepath.Join(p, part)
		st, err := os.Lstat(p)
		if err != nil || !st.IsDir() {
			return false
		}
	}
	return true
}

func hasImageFile(dir string) bool {
	for _, name := range imageFiles {
		if st, err := os.Lstat(filepath.Join(dir, name)); err == nil && st.Mode().IsRegular() {
			return true
		}
	}
	return false
}

// annotateHomeMirror sets HomeMirror on every repository item. Each
// home/ is read at most once.
func annotateHomeMirror(resolvedRoot string, items []model.ConfigItem) {
	known := map[string]bool{}
	for i := range items {
		item := &items[i]
		item.HomeMirror = false
		if item.Scope != model.ScopeRepo {
			continue
		}
		home, ok := homeConfigDir(item.Path)
		if !ok {
			continue
		}
		mirror, seen := known[home]
		if !seen {
			mirror = homeMirrorAt(resolvedRoot, home)
			known[home] = mirror
		}
		item.HomeMirror = mirror
	}
}
