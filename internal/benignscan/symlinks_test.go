package benignscan

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func testIndex() *treeIndex {
	files := []treeFile{
		{oid: "hook", path: "data/hooks/hook.sh"},
		{oid: "settings", path: "shared/claude/settings.json"},
		{oid: "inner", path: "shared/claude/hooks/x.sh"},
		{oid: "readme", path: "README.md"},
	}
	links := map[string]string{
		".claude/scripts/hook": "../../data/hooks/hook.sh", // relative, two levels up
		"a/b/c/hook":           "../../../data/hooks/hook.sh",
		"tools":                "shared/claude",                // directory link
		"via":                  "tools/settings.json",          // file reached through a directory link
		"chain1":               "chain2",                       // chained
		"chain2":               "./data/../data/hooks/hook.sh", // with . and ..
		"dangling":             "nope.json",
		"notdir":               "README.md/x",
		"abs":                  "/etc/passwd",
		"backslash":            "..\\etc\\passwd",
		"esc":                  "../outside",
		"esc2":                 "data/../../x",
		"a/b/esc3":             "../../../etc/passwd",
		"up":                   "..",
		"esc4":                 "up/../x",
		"loopA":                "loopB",
		"loopB":                "loopA",
		"self":                 "self",
		"shared/claude/up":     "..",
		"gitdir":               ".git/config",
		"empty":                "",
	}
	return newTreeIndex(files, links, nil)
}

func TestResolveLinks(t *testing.T) {
	ix := testIndex()
	for _, tc := range []struct {
		link, want string
		kind       linkKind
		err        string
	}{
		{".claude/scripts/hook", "data/hooks/hook.sh", linkFile, ""},
		{"a/b/c/hook", "data/hooks/hook.sh", linkFile, ""},
		{"tools", "shared/claude", linkDir, ""},
		{"via", "shared/claude/settings.json", linkFile, ""},
		{"tools/hooks/x.sh", "shared/claude/hooks/x.sh", linkFile, ""},
		{"chain1", "data/hooks/hook.sh", linkFile, ""},
		{"chain2", "data/hooks/hook.sh", linkFile, ""},
		{"dangling", "", 0, "dangling"},
		{"notdir", "", 0, "not a directory"},
		{"abs", "", 0, "absolute"},
		{"backslash", "", 0, "absolute"},
		{"esc", "", 0, "escapes"},
		{"esc2", "", 0, "escapes"},
		{"a/b/esc3", "", 0, "escapes"},
		{"esc4", "", 0, "escapes"},
		{"loopA", "", 0, "loop"},
		{"self", "", 0, "loop"},
		{"gitdir", "", 0, ".git"},
		{"empty", "", 0, "absolute"},
	} {
		got, kind, err := ix.resolve(tc.link)
		if tc.err != "" {
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Errorf("%s: got %q %v, want error containing %q", tc.link, got, err, tc.err)
			}
			continue
		}
		if err != nil || got != tc.want || kind != tc.kind {
			t.Errorf("%s: got %q kind %d err %v, want %q kind %d", tc.link, got, kind, err, tc.want, tc.kind)
		}
	}
}

func TestLinkedFiles(t *testing.T) {
	files, skips := testIndex().linkedFiles()
	got := map[string]string{}
	for _, f := range files {
		got[f.path] = f.oid
	}
	want := map[string]string{
		".claude/scripts/hook": "hook",
		"a/b/c/hook":           "hook",
		"chain1":               "hook",
		"chain2":               "hook",
		"via":                  "settings",
		"tools/settings.json":  "settings",
		"tools/hooks/x.sh":     "inner",
	}
	if len(got) != len(want) {
		t.Errorf("linked files %v, want %v", got, want)
	}
	for p, oid := range want {
		if got[p] != oid {
			t.Errorf("%s: got %q, want %q", p, got[p], oid)
		}
	}
	var skipped []string
	for _, s := range skips {
		skipped = append(skipped, s.Path)
	}
	sort.Strings(skipped)
	wantSkipped := []string{"a/b/esc3", "abs", "backslash", "dangling", "empty", "esc", "esc2", "esc4", "gitdir", "loopA", "loopB", "notdir", "self", "shared/claude/up", "up"}
	if strings.Join(skipped, " ") != strings.Join(wantSkipped, " ") {
		t.Errorf("skipped %v, want %v", skipped, wantSkipped)
	}
}

// TestLinkedFilesDirectoryLoop checks that a directory link to a
// directory that holds it, or to the root, is not expanded.
func TestLinkedFilesDirectoryLoop(t *testing.T) {
	ix := newTreeIndex([]treeFile{{oid: "f", path: "d/f.json"}}, map[string]string{
		"d/again":    ".",
		"d/sub/back": "../..",
		"root":       ".",
	}, nil)
	files, skips := ix.linkedFiles()
	if len(files) != 0 || len(skips) != 3 {
		t.Fatalf("files %v skips %v", files, skips)
	}
}

// TestLinkedFilesNestedDirectoryLinks checks that a directory link
// inside a linked directory is expanded at the outer link's path, and
// that two directories linking to each other stop after one level:
// B/toC/toB would be B again, which holds the link B/toC.
func TestLinkedFilesNestedDirectoryLinks(t *testing.T) {
	ix := newTreeIndex([]treeFile{
		{oid: "skill", path: "shared/skills/review/SKILL.md"},
		{oid: "b", path: "B/b.json"},
		{oid: "c", path: "C/c.json"},
	}, map[string]string{
		".claude":        ".agents",
		".agents/skills": "../shared/skills",
		"B/toC":          "../C",
		"C/toB":          "../B",
	}, nil)
	files, skips := ix.linkedFiles()
	got := map[string]string{}
	for _, f := range files {
		got[f.path] = f.oid
	}
	want := map[string]string{
		".agents/skills/review/SKILL.md": "skill",
		".claude/skills/review/SKILL.md": "skill",
		"B/toC/c.json":                   "c",
		"C/toB/b.json":                   "b",
	}
	if len(skips) != 0 || len(got) != len(want) {
		t.Fatalf("files %v skips %v, want %v", got, skips, want)
	}
	for p, oid := range want {
		if got[p] != oid {
			t.Errorf("%s: got %q, want %q", p, got[p], oid)
		}
	}
}

// TestGitFetcherSymlinks fetches a local repository with every kind of
// link and checks that only links that resolve inside the tree are
// written, as regular files, and that the scan directory holds no link.
func TestGitFetcherSymlinks(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	src := filepath.Join(root, "src")
	runGit := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command(git, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(rel, data string) {
		p := filepath.Join(src, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	link := func(rel, target string) {
		p := filepath.Join(src, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, p); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(src, "init", "-q", "-b", "main")
	write("data/hooks/hook.sh", "#!/bin/sh\necho hook\n")
	write("shared/claude/settings.json", `{"hooks":{}}`)
	write("shared/claude/hooks/x.sh", "echo x\n")
	write(".claude/settings.json", `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"./.claude/scripts/hook"}]}]}}`)
	link(".claude/scripts/hook", "../../data/hooks/hook.sh")
	link("a/b/c/hook", "../../../data/hooks/hook.sh")
	link("tools", "shared/claude")
	link("via", "tools/settings.json")
	link("chain1", "chain2")
	link("chain2", "data/hooks/hook.sh")
	link("dangling", "nope.json")
	link("abs", "/etc/passwd")
	link("esc", "../outside")
	link("a/b/esc3", "../../../etc/passwd")
	link("up", "..")
	link("loopA", "loopB")
	link("loopB", "loopA")
	link("shared/claude/up", "..")
	write("shared/skills/review/SKILL.md", "# review\n")
	link(".agents/skills", "../shared/skills")
	link(".cursor", ".agents")
	// A file just outside the repository: an escaping link must not reach it.
	if err := os.WriteFile(filepath.Join(root, "outside"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(src, "add", "-A")
	runGit(src, "commit", "-q", "-m", "init")
	sha := runGit(src, "rev-parse", "HEAD")
	bare := filepath.Join(root, "vgmartinez", "links.git")
	runGit(root, "clone", "-q", "--bare", src, bare)
	runGit(bare, "config", "uploadpack.allowFilter", "true")
	runGit(bare, "config", "uploadpack.allowAnySHA1InWant", "true")

	dir := filepath.Join(root, "out", "tree")
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	var skipped []string
	f := GitFetcher{BaseURL: "file://" + root, OnLinkSkip: func(p, reason string) { skipped = append(skipped, p) }}
	if err := f.Fetch(context.Background(), "vgmartinez/links", sha, dir); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			t.Errorf("symlink written: %s", p)
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			t.Errorf("not a regular file: %s", p)
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		got[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	hook := "#!/bin/sh\necho hook\n"
	want := map[string]string{
		"data/hooks/hook.sh":             hook,
		"shared/claude/settings.json":    `{"hooks":{}}`,
		"shared/claude/hooks/x.sh":       "echo x\n",
		".claude/settings.json":          `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"./.claude/scripts/hook"}]}]}}`,
		".claude/scripts/hook":           hook,
		"a/b/c/hook":                     hook,
		"chain1":                         hook,
		"chain2":                         hook,
		"via":                            `{"hooks":{}}`,
		"tools/settings.json":            `{"hooks":{}}`,
		"tools/hooks/x.sh":               "echo x\n",
		"shared/skills/review/SKILL.md":  "# review\n",
		".agents/skills/review/SKILL.md": "# review\n",
		".cursor/skills/review/SKILL.md": "# review\n",
	}
	if len(got) != len(want) {
		t.Errorf("files %v, want %v", keys(got), keys(want))
	}
	for p, data := range want {
		if got[p] != data {
			t.Errorf("%s: got %q, want %q", p, got[p], data)
		}
	}
	for p, data := range got {
		if data == "secret" {
			t.Errorf("%s holds the file outside the repository", p)
		}
	}
	sort.Strings(skipped)
	wantSkipped := "a/b/esc3 abs dangling esc loopA loopB shared/claude/up up"
	if strings.Join(skipped, " ") != wantSkipped {
		t.Errorf("skipped %v, want %s", skipped, wantSkipped)
	}
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestParseTreeLink(t *testing.T) {
	oid := strings.Repeat("b", 40)
	for _, tc := range []struct {
		ent string
		ok  bool
	}{
		{"120000 blob " + oid + "\t.claude/scripts/hook", true},
		{"100644 blob " + oid + "\t.claude/settings.json", false},
		{"160000 commit " + oid + "\tvendor/sub", false},
		{"120000 blob " + oid + "\t../escape", false},
		{"120000 blob " + oid + "\tsub/.git/hooks", false},
		{"120000 blob " + oid + "\t/abs", false},
		{"garbage", false},
	} {
		if _, ok := parseTreeLink(tc.ent); ok != tc.ok {
			t.Errorf("%q: ok=%v", tc.ent, ok)
		}
	}
}
