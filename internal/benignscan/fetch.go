package benignscan

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/vgmartinez/tripcord/internal/adapters"
)

// Fetcher writes the tree of repo at sha into dir.
type Fetcher interface {
	Fetch(ctx context.Context, repo, sha, dir string) error
}

// GitFetcher shallow-fetches one commit from GitHub into a bare
// repository and writes its regular files into the scan directory. No
// work tree is checked out, so no hook, filter, submodule, or LFS
// smudge runs, and submodules are skipped. A symlink is resolved inside
// the commit's tree and written as a regular copy of what it reaches;
// one that is absolute, dangling, part of a loop, or resolves outside
// the repository is skipped (see symlinks.go). A blob the
// adapters would refuse (over adapters.MaxConfigFileBytes) is not
// downloaded; it is written as a sparse file one byte over that cap,
// so the scan still sees an oversize file at that path.
type GitFetcher struct {
	Git     string
	BaseURL string
	// OnLinkSkip, when set, is called for each symlink that is not
	// followed, with the reason.
	OnLinkSkip func(path, reason string)
}

const oversize = adapters.MaxConfigFileBytes + 1

func (g GitFetcher) Fetch(ctx context.Context, repo, sha, dir string) error {
	git := g.Git
	if git == "" {
		git = "git"
	}
	base := g.BaseURL
	if base == "" {
		base = "https://github.com"
	}
	gitDir := dir + ".git"
	defer func() { _ = os.RemoveAll(gitDir) }()
	run := func(stdin io.Reader, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, git, append([]string{"--git-dir", gitDir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
			"GIT_TERMINAL_PROMPT=0", "GIT_LFS_SKIP_SMUDGE=1", "GIT_NO_LAZY_FETCH=1")
		cmd.Stdin = stdin
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
		}
		return out, nil
	}
	if _, err := run(nil, "init", "-q", "--bare", "--template="); err != nil {
		return err
	}
	url := strings.TrimRight(base, "/") + "/" + repo + ".git"
	scheme := "https"
	if strings.HasPrefix(base, "file://") {
		scheme = "file"
	}
	if _, err := run(nil, "-c", "protocol.allow=never", "-c", "protocol."+scheme+".allow=always",
		"-c", "fetch.recurseSubmodules=false", "-c", "core.hooksPath="+os.DevNull,
		"fetch", "-q", "--depth=1", "--no-tags", "--no-write-fetch-head",
		"--filter=blob:limit="+strconv.FormatInt(oversize, 10), url, sha+":refs/scan"); err != nil {
		return err
	}
	// Without a promisor remote, a missing blob stays missing instead of
	// being fetched lazily on read. The section is absent when the server
	// ignored the filter.
	_, _ = run(nil, "config", "--remove-section", "remote."+url)
	if out, err := run(nil, "config", "--get-regexp", `\.promisor$`); err == nil {
		return fmt.Errorf("promisor remote left after fetch: %s", strings.TrimSpace(string(out)))
	}
	list, err := run(nil, "ls-tree", "-r", "-z", "--full-tree", "refs/scan")
	if err != nil {
		return err
	}
	var files, links []treeFile
	for _, ent := range bytes.Split(list, []byte{0}) {
		if len(ent) == 0 {
			continue
		}
		f, ok, err := parseTreeEntry(string(ent))
		if err != nil {
			return err
		}
		if ok {
			files = append(files, f)
		} else if l, isLink := parseTreeLink(string(ent)); isLink {
			links = append(links, l)
		}
	}
	if len(links) > 0 {
		targets, err := readLinkTargets(ctx, git, gitDir, links)
		if err != nil {
			return err
		}
		linked, skips := newTreeIndex(files, targets, nil).linkedFiles()
		files = append(files, linked...)
		if g.OnLinkSkip != nil {
			for _, s := range skips {
				g.OnLinkSkip(s.Path, s.Reason)
			}
		}
	}
	if len(files) == 0 {
		return os.MkdirAll(dir, 0o755)
	}
	var ids bytes.Buffer
	for _, f := range files {
		ids.WriteString(f.oid + "\n")
	}
	return writeBlobs(ctx, git, gitDir, dir, files, &ids)
}

type treeFile struct {
	oid  string
	path string
}

// parseTreeEntry reads one "mode type oid\tpath" line. ok is false for
// a symlink, a submodule, or anything else that is not a regular blob;
// symlinks are read separately by parseTreeLink.
func parseTreeEntry(ent string) (treeFile, bool, error) {
	meta, p, found := strings.Cut(ent, "\t")
	fields := strings.Fields(meta)
	if !found || len(fields) != 3 {
		return treeFile{}, false, fmt.Errorf("ls-tree: bad entry %q", ent)
	}
	if fields[1] != "blob" || (fields[0] != "100644" && fields[0] != "100755") {
		return treeFile{}, false, nil
	}
	if !safeRel(p) {
		return treeFile{}, false, fmt.Errorf("ls-tree: unsafe path %q", p)
	}
	return treeFile{oid: fields[2], path: p}, true, nil
}

// parseTreeLink reads one ls-tree entry and reports whether it is a
// symlink (mode 120000) at a safe path.
func parseTreeLink(ent string) (treeFile, bool) {
	meta, p, found := strings.Cut(ent, "\t")
	fields := strings.Fields(meta)
	if !found || len(fields) != 3 || fields[0] != "120000" || fields[1] != "blob" || !safeRel(p) {
		return treeFile{}, false
	}
	return treeFile{oid: fields[2], path: p}, true
}

// readLinkTargets reads each symlink blob, which holds the link target.
// A target that is missing or over maxLinkTarget bytes is recorded as
// empty, so the link is skipped as unsafe.
func readLinkTargets(ctx context.Context, git, gitDir string, links []treeFile) (map[string]string, error) {
	var ids bytes.Buffer
	for _, l := range links {
		ids.WriteString(l.oid + "\n")
	}
	cmd := exec.CommandContext(ctx, git, "--git-dir", gitDir, "cat-file", "--batch")
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_NO_LAZY_FETCH=1")
	cmd.Stdin = &ids
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git cat-file: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	r := bufio.NewReader(bytes.NewReader(out))
	targets := make(map[string]string, len(links))
	for _, l := range links {
		head, err := r.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("cat-file: %w", err)
		}
		fields := strings.Fields(head)
		if len(fields) == 2 && fields[1] == "missing" {
			targets[l.path] = ""
			continue
		}
		if len(fields) != 3 || fields[0] != l.oid || fields[1] != "blob" {
			return nil, fmt.Errorf("cat-file: unexpected header %q", strings.TrimSpace(head))
		}
		n, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("cat-file: bad size in %q", strings.TrimSpace(head))
		}
		buf := make([]byte, n)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, fmt.Errorf("cat-file: %w", err)
		}
		if b, err := r.ReadByte(); err != nil || b != '\n' {
			return nil, errors.New("cat-file: missing record terminator")
		}
		if n > maxLinkTarget {
			targets[l.path] = ""
		} else {
			targets[l.path] = string(buf)
		}
	}
	return targets, nil
}

func safeRel(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.ContainsAny(p, "\\\x00") || path.Clean(p) != p {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." || part == "." || strings.EqualFold(part, ".git") {
			return false
		}
	}
	return true
}

func writeBlobs(ctx context.Context, git, gitDir, dir string, files []treeFile, ids io.Reader) error {
	cmd := exec.CommandContext(ctx, git, "--git-dir", gitDir, "cat-file", "--batch")
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_NO_LAZY_FETCH=1")
	cmd.Stdin = ids
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	werr := readBatch(bufio.NewReader(out), dir, files)
	if werr != nil {
		_ = cmd.Process.Kill()
	}
	if err := cmd.Wait(); err != nil && werr == nil {
		werr = fmt.Errorf("git cat-file: %w", err)
	}
	return werr
}

func readBatch(r *bufio.Reader, dir string, files []treeFile) error {
	for _, f := range files {
		head, err := r.ReadString('\n')
		if err != nil {
			return fmt.Errorf("cat-file: %w", err)
		}
		fields := strings.Fields(head)
		dst := filepath.Join(dir, filepath.FromSlash(f.path))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if len(fields) == 2 && fields[1] == "missing" {
			if err := writeSparse(dst, oversize); err != nil {
				return err
			}
			continue
		}
		if len(fields) != 3 || fields[0] != f.oid || fields[1] != "blob" {
			return fmt.Errorf("cat-file: unexpected header %q", strings.TrimSpace(head))
		}
		n, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || n < 0 {
			return fmt.Errorf("cat-file: bad size in %q", strings.TrimSpace(head))
		}
		if err := writeFile(dst, io.LimitReader(r, n), n); err != nil {
			return err
		}
		if b, err := r.ReadByte(); err != nil || b != '\n' {
			return errors.New("cat-file: missing record terminator")
		}
	}
	return nil
}

func writeFile(dst string, r io.Reader, n int64) error {
	fh, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.CopyN(fh, r, n); err != nil {
		_ = fh.Close()
		return err
	}
	return fh.Close()
}

func writeSparse(dst string, size int64) error {
	fh, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if err := fh.Truncate(size); err != nil {
		_ = fh.Close()
		return err
	}
	return fh.Close()
}
