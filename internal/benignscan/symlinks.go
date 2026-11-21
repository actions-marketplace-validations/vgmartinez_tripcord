package benignscan

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// Symbolic links in the fetched tree are resolved inside the tree, as
// paths, without writing a link to disk. A link that resolves to a
// regular file in the same commit is written as a copy of that file at
// the link's path; a link that resolves to a directory gets a copy of
// every regular file the directory reaches, under the link's path. So a
// config or hook script reached through a link is scanned at the path
// a harness would open, and the scan directory still holds only
// regular files and directories.
//
// A link is never followed out of the tree. A link is skipped when:
//   - its target is absolute (/x, or a backslash or NUL anywhere);
//   - resolving it, including every link it passes through, would climb
//     above the repository root (../ escapes);
//   - resolving it takes more than maxLinkHops links (a loop);
//   - it resolves to nothing in the tree, to a submodule, or through a
//     .git path component (dangling);
//   - it resolves to the repository root or to a directory that holds
//     the link itself (copying it would include the link's own copy).
//
// A directory link inside a linked directory is expanded too, up to
// maxLinkDirDepth levels, unless it reaches a directory that holds the
// outer link, or one already expanded for the same link or above it.
//
// Targets are read from the link blobs; a target over maxLinkTarget
// bytes is skipped as unsafe.
const (
	maxLinkHops   = 40
	maxLinkTarget = 4096
	maxLinkFiles  = 20000
	// maxLinkDirDepth bounds how many directory links inside a linked
	// directory are expanded below one another.
	maxLinkDirDepth = 8
)

// holds reports whether directory dir is the root, p itself, or a
// directory above p.
func holds(dir, p string) bool {
	return dir == "" || dir == p || strings.HasPrefix(p, dir+"/")
}

// treeIndex is the commit's tree as paths: regular files (path to
// blob id), links (path to target), and every directory that holds one
// of them.
type treeIndex struct {
	files map[string]string
	links map[string]string
	dirs  map[string]bool
}

func newTreeIndex(files []treeFile, links map[string]string, other []string) *treeIndex {
	t := &treeIndex{files: map[string]string{}, links: links, dirs: map[string]bool{"": true}}
	addDirs := func(p string) {
		for d := path.Dir(p); d != "." && !t.dirs[d]; d = path.Dir(d) {
			t.dirs[d] = true
		}
	}
	for _, f := range files {
		t.files[f.path] = f.oid
		addDirs(f.path)
	}
	for p := range links {
		addDirs(p)
	}
	for _, p := range other {
		addDirs(p)
	}
	return t
}

type linkKind int

const (
	linkFile linkKind = iota
	linkDir
)

// resolve follows p through the tree one component at a time, the way
// the kernel does, and returns the physical path it reaches. ".." pops
// the physical directory reached so far, so a link's ".." is relative
// to the directory that holds the link.
func (t *treeIndex) resolve(p string) (string, linkKind, error) {
	queue := strings.Split(p, "/")
	var stack []string
	hops := 0
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		switch {
		case c == "" || c == ".":
			continue
		case c == "..":
			if len(stack) == 0 {
				return "", 0, fmt.Errorf("escapes the repository")
			}
			stack = stack[:len(stack)-1]
			continue
		case strings.EqualFold(c, ".git"):
			return "", 0, fmt.Errorf("dangling (.git component)")
		}
		next := strings.Join(append(append([]string(nil), stack...), c), "/")
		if target, ok := t.links[next]; ok {
			hops++
			if hops > maxLinkHops {
				return "", 0, fmt.Errorf("too many links (loop)")
			}
			if target == "" || strings.HasPrefix(target, "/") || strings.ContainsAny(target, "\\\x00") || len(target) > maxLinkTarget {
				return "", 0, fmt.Errorf("absolute or unsafe target %q", target)
			}
			queue = append(strings.Split(target, "/"), queue...)
			continue
		}
		if _, ok := t.files[next]; ok {
			for _, rest := range queue {
				if rest != "" && rest != "." {
					return "", 0, fmt.Errorf("dangling (not a directory)")
				}
			}
			stack = append(stack, c)
			continue
		}
		if t.dirs[next] {
			stack = append(stack, c)
			continue
		}
		return "", 0, fmt.Errorf("dangling")
	}
	got := strings.Join(stack, "/")
	if _, ok := t.files[got]; ok {
		return got, linkFile, nil
	}
	return got, linkDir, nil
}

// linkSkip is one link that was not followed, and why.
type linkSkip struct {
	Path, Reason string
}

// linkedFiles returns the regular files to write for the tree's links:
// the blob id of the file a link reaches, at the link's path, or, for a
// directory link, at the link's path plus each file's path inside the
// directory. Paths already in the tree are never returned twice.
func (t *treeIndex) linkedFiles() ([]treeFile, []linkSkip) {
	var out []treeFile
	var skips []linkSkip
	seen := map[string]bool{}
	add := func(p, oid string) {
		if seen[p] || len(out) >= maxLinkFiles {
			return
		}
		if _, ok := t.files[p]; ok || !safeRel(p) {
			return
		}
		seen[p] = true
		out = append(out, treeFile{oid: oid, path: p})
	}
	names := make([]string, 0, len(t.links))
	for p := range t.links {
		names = append(names, p)
	}
	sort.Strings(names)
	// The tree's own entries, sorted, for directory expansion.
	entries := make([]string, 0, len(t.files)+len(t.links))
	for p := range t.files {
		entries = append(entries, p)
	}
	entries = append(entries, names...)
	sort.Strings(entries)
	for _, l := range names {
		got, kind, err := t.resolve(l)
		if err != nil {
			skips = append(skips, linkSkip{l, err.Error()})
			continue
		}
		if kind == linkFile {
			add(l, t.files[got])
			continue
		}
		if holds(got, l) {
			skips = append(skips, linkSkip{l, "directory holds the link (loop)"})
			continue
		}
		// Expand the directory, and each directory link inside it, at
		// the link's path. A directory that holds the link, or one
		// already expanded for this link or above it, is not expanded.
		type pending struct {
			virt, phys string
			depth      int
		}
		visited := map[string]bool{got: true}
		work := []pending{{l, got, 0}}
		for len(work) > 0 {
			w := work[0]
			work = work[1:]
			prefix := w.phys + "/"
			for i := sort.SearchStrings(entries, prefix); i < len(entries) && strings.HasPrefix(entries[i], prefix); i++ {
				v := w.virt + "/" + strings.TrimPrefix(entries[i], prefix)
				fp, k, err := t.resolve(v)
				if err != nil {
					continue
				}
				if k == linkFile {
					add(v, t.files[fp])
					continue
				}
				if _, isLink := t.links[entries[i]]; !isLink || w.depth >= maxLinkDirDepth || holds(fp, l) {
					continue
				}
				loop := false
				for d := range visited {
					if holds(fp, d) {
						loop = true
						break
					}
				}
				if !loop {
					visited[fp] = true
					work = append(work, pending{v, fp, w.depth + 1})
				}
			}
		}
	}
	return out, skips
}
