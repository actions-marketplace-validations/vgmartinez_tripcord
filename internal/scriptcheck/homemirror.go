package scriptcheck

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/vgmartinez/tripcord/internal/model"
)

// homeMirrorDir is the top-level directory a dotfiles repository mirrors
// the owner's home directory into: GNU Stow, rcm, and similar tools link
// home/.zshenv to ~/.zshenv and home/.claude/settings.json to
// ~/.claude/settings.json.
const homeMirrorDir = "home"

// HomeMirror reports whether relPath, a repository-relative slash path,
// is inside a harness config directory (one of configDirs) directly
// under a top-level home/ whose direct children are all dot-entries.
// resolvedRoot is the scanned repository with its own symlinks
// resolved. home/ must be a real directory, not a symlink. Any error
// reading it, an empty home/, or a single entry whose name does not start
// with "." means it is not a mirror. A home/ deeper in the tree
// (src/home/.claude) is not a mirror either.
//
// The shape is cheap to fake: anyone can commit home/.claude next to a
// home/.zshenv. That is why a match on such an item is lowered to low
// rather than dropped, so it stays in the report.
func HomeMirror(resolvedRoot, relPath string) bool {
	return inHomeConfigDir(relPath) && homeMirrorRoot(resolvedRoot)
}

// inHomeConfigDir reports whether relPath is home/<config dir>/<more>.
func inHomeConfigDir(relPath string) bool {
	parts := strings.SplitN(relPath, "/", 3)
	return len(parts) == 3 && parts[0] == homeMirrorDir && slices.Contains(configDirs, parts[1]) && parts[2] != ""
}

func homeMirrorRoot(resolvedRoot string) bool {
	dir := filepath.Join(resolvedRoot, homeMirrorDir)
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

// annotateHomeMirror sets HomeMirror on every repository item. home/ is
// read at most once.
func annotateHomeMirror(resolvedRoot string, items []model.ConfigItem) {
	known, mirror := false, false
	for i := range items {
		item := &items[i]
		item.HomeMirror = false
		if item.Scope != model.ScopeRepo {
			continue
		}
		if !inHomeConfigDir(item.Path) {
			continue
		}
		if !known {
			known, mirror = true, homeMirrorRoot(resolvedRoot)
		}
		item.HomeMirror = mirror
	}
}
