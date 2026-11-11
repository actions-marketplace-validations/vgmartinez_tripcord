// Command release-packages turns one published tripcord release into
// the Homebrew formula, the npm package, and the PyPI wheels. It never
// builds tripcord: every package wraps the release archives that
// scripts/release-build.sh made, and every command fails unless those
// archives match the release's SHA256SUMS.
//
//	go run ./scripts/release-packages formula -version vX.Y.Z -sums DIST/SHA256SUMS [-out FILE]
//	go run ./scripts/release-packages wheels -version vX.Y.Z -dist DIST -src packaging/pypi -out DIR
//	go run ./scripts/release-packages npm -version vX.Y.Z -sums DIST/SHA256SUMS -src packaging/npm -license LICENSE -out DIR
//
// formula prints Formula/tripcord.rb for the vgmartinez/homebrew-tap
// repository: one url and sha256 per macOS and Linux CPU, copied from
// SHA256SUMS.
//
// wheels checks all six archives in DIST against DIST/SHA256SUMS, then
// writes one py3-none-<platform> wheel per archive. Each wheel holds the
// release binary unchanged, LICENSE, NOTICE, THIRD_PARTY_NOTICES, and the
// small Python package in -src that execs the binary.
//
// npm stages the npm package in DIR: the shim in -src with its version
// set from the tag, the repository LICENSE, and the release's
// SHA256SUMS, which the shim checks its one download against.
//
// Only vX.Y.Z tags are packaged; a prerelease tag is an error.
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"debug/macho"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"text/template"
	"time"
)

const (
	repo     = "vgmartinez/tripcord"
	repoURL  = "https://github.com/" + repo
	summary  = "Catch malicious coding-agent config before it runs"
	devNPMV  = `"version": "0.0.0-dev"`
	maxEntry = 256 << 20
)

var versionRE = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// target is one release platform.
type target struct {
	GOOS, GOARCH string
}

// targets is the release matrix, in SHA256SUMS order.
var targets = []target{
	{"darwin", "amd64"},
	{"darwin", "arm64"},
	{"linux", "amd64"},
	{"linux", "arm64"},
	{"windows", "amd64"},
	{"windows", "arm64"},
}

func (t target) archive(ver string) string {
	ext := ".tar.gz"
	if t.GOOS == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("tripcord_%s_%s_%s%s", ver, t.GOOS, t.GOARCH, ext)
}

func (t target) binary() string {
	if t.GOOS == "windows" {
		return "tripcord.exe"
	}
	return "tripcord"
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "release-packages:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: release-packages formula|wheels|npm [flags]")
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	version := fs.String("version", "", "release tag, vX.Y.Z")
	sums := fs.String("sums", "", "the release's SHA256SUMS")
	dist := fs.String("dist", "", "directory with the six release archives and SHA256SUMS")
	src := fs.String("src", "", "package source directory")
	license := fs.String("license", "", "the repository LICENSE")
	out := fs.String("out", "", "output file or directory")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments %q", fs.Args())
	}
	ver, err := plainVersion(*version)
	if err != nil {
		return err
	}
	switch args[0] {
	case "formula":
		if *sums == "" {
			return errors.New("formula needs -sums")
		}
		digests, err := readSums(*sums, ver)
		if err != nil {
			return err
		}
		text, err := formula(ver, digests)
		if err != nil {
			return err
		}
		if *out == "" {
			_, err = io.WriteString(stdout, text)
			return err
		}
		return os.WriteFile(*out, []byte(text), 0o644)
	case "wheels":
		if *dist == "" || *src == "" || *out == "" {
			return errors.New("wheels needs -dist, -src, and -out")
		}
		names, err := buildWheels(ver, *dist, *src, *out)
		if err != nil {
			return err
		}
		for _, n := range names {
			if _, err := fmt.Fprintln(stdout, n); err != nil {
				return err
			}
		}
		return nil
	case "npm":
		if *sums == "" || *src == "" || *license == "" || *out == "" {
			return errors.New("npm needs -sums, -src, -license, and -out")
		}
		return stageNPM(ver, *sums, *src, *license, *out)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

// plainVersion returns X.Y.Z for the tag vX.Y.Z.
func plainVersion(tag string) (string, error) {
	if !versionRE.MatchString(tag) {
		return "", fmt.Errorf("version %q is not vX.Y.Z; prerelease tags are not packaged", tag)
	}
	return strings.TrimPrefix(tag, "v"), nil
}

// readSums parses SHA256SUMS. It fails unless the file lists exactly the
// six release archives for ver, once each, as sha256sum writes them.
func readSums(file, ver string) (map[string]string, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return parseSums(raw, ver)
}

var sumLine = regexp.MustCompile(`^([0-9a-f]{64})  (\S+)$`)

func parseSums(raw []byte, ver string) (map[string]string, error) {
	want := map[string]bool{}
	for _, t := range targets {
		want[t.archive(ver)] = true
	}
	text := string(raw)
	if !strings.HasSuffix(text, "\n") {
		return nil, errors.New("SHA256SUMS does not end in a newline")
	}
	got := map[string]string{}
	for i, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		m := sumLine.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("SHA256SUMS line %d is not \"<sha256>  <file>\"", i+1)
		}
		if !want[m[2]] {
			return nil, fmt.Errorf("SHA256SUMS lists %q, which is not a %s release archive", m[2], ver)
		}
		if _, dup := got[m[2]]; dup {
			return nil, fmt.Errorf("SHA256SUMS lists %q twice", m[2])
		}
		got[m[2]] = m[1]
	}
	for name := range want {
		if _, ok := got[name]; !ok {
			return nil, fmt.Errorf("SHA256SUMS has no line for %s", name)
		}
	}
	return got, nil
}

// verifyFile fails unless file's sha256 is want.
func verifyFile(file, want string) ([]byte, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(raw)
	if got := hex.EncodeToString(sum[:]); got != want {
		return nil, fmt.Errorf("%s: sha256 %s does not match SHA256SUMS %s", filepath.Base(file), got, want)
	}
	return raw, nil
}

var formulaTmpl = template.Must(template.New("formula").Parse(`# Generated by scripts/release-packages in {{.Repo}} from the
# SHA256SUMS of {{.RepoURL}}/releases/tag/v{{.Version}}.
# Do not edit by hand: the homebrew workflow regenerates it.
class Tripcord < Formula
  desc "{{.Summary}}"
  homepage "{{.RepoURL}}"
  version "{{.Version}}"
  license "Apache-2.0"

  on_macos do
    if Hardware::CPU.arm?
      url "{{.URL "darwin" "arm64"}}"
      sha256 "{{.Sum "darwin" "arm64"}}"
    end
    if Hardware::CPU.intel?
      url "{{.URL "darwin" "amd64"}}"
      sha256 "{{.Sum "darwin" "amd64"}}"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "{{.URL "linux" "arm64"}}"
      sha256 "{{.Sum "linux" "arm64"}}"
    end
    if Hardware::CPU.intel?
      url "{{.URL "linux" "amd64"}}"
      sha256 "{{.Sum "linux" "amd64"}}"
    end
  end

  def install
    bin.install "tripcord"
    prefix.install "THIRD_PARTY_NOTICES"
  end

  test do
    assert_match "v#{version} (commit ", shell_output("#{bin}/tripcord --version")
  end
end
`))

type formulaData struct {
	Repo, RepoURL, Summary, Version string
	digests                         map[string]string
}

func (d formulaData) name(goos, goarch string) string {
	return target{goos, goarch}.archive(d.Version)
}

func (d formulaData) URL(goos, goarch string) string {
	return fmt.Sprintf("%s/releases/download/v%s/%s", d.RepoURL, d.Version, d.name(goos, goarch))
}

func (d formulaData) Sum(goos, goarch string) (string, error) {
	s, ok := d.digests[d.name(goos, goarch)]
	if !ok {
		return "", fmt.Errorf("no digest for %s", d.name(goos, goarch))
	}
	return s, nil
}

func formula(ver string, digests map[string]string) (string, error) {
	var b strings.Builder
	err := formulaTmpl.Execute(&b, formulaData{Repo: repo, RepoURL: repoURL, Summary: summary, Version: ver, digests: digests})
	return b.String(), err
}

// payload is what one release archive holds.
type payload struct {
	binary   []byte
	notices  map[string][]byte
	modified time.Time
}

var noticeFiles = []string{"LICENSE", "NOTICE", "THIRD_PARTY_NOTICES"}

// readArchive returns the binary and notices in one release archive. It
// fails on any other entry, a duplicate, or a missing file.
func readArchive(t target, raw []byte) (payload, error) {
	files := map[string][]byte{}
	var modified time.Time
	add := func(name string, mod time.Time, r io.Reader) error {
		want := name == t.binary() || slices.Contains(noticeFiles, name)
		if !want {
			return fmt.Errorf("unexpected archive entry %q", name)
		}
		if _, dup := files[name]; dup {
			return fmt.Errorf("archive entry %q appears twice", name)
		}
		data, err := io.ReadAll(io.LimitReader(r, maxEntry+1))
		if err != nil {
			return err
		}
		if len(data) > maxEntry {
			return fmt.Errorf("archive entry %q is too large", name)
		}
		files[name] = data
		if name == t.binary() {
			modified = mod
		}
		return nil
	}
	if t.GOOS == "windows" {
		zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			return payload{}, err
		}
		for _, f := range zr.File {
			if !f.Mode().IsRegular() {
				return payload{}, fmt.Errorf("archive entry %q is not a regular file", f.Name)
			}
			rc, err := f.Open()
			if err != nil {
				return payload{}, err
			}
			err = add(f.Name, f.Modified, rc)
			_ = rc.Close()
			if err != nil {
				return payload{}, err
			}
		}
	} else {
		gz, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return payload{}, err
		}
		tr := tar.NewReader(gz)
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return payload{}, err
			}
			if h.Typeflag != tar.TypeReg {
				return payload{}, fmt.Errorf("archive entry %q is not a regular file", h.Name)
			}
			if err := add(h.Name, h.ModTime, tr); err != nil {
				return payload{}, err
			}
		}
	}
	p := payload{binary: files[t.binary()], notices: map[string][]byte{}, modified: modified}
	if p.binary == nil {
		return payload{}, fmt.Errorf("archive has no %s", t.binary())
	}
	for _, n := range noticeFiles {
		if files[n] == nil {
			return payload{}, fmt.Errorf("archive has no %s", n)
		}
		p.notices[n] = files[n]
	}
	return p, nil
}

// platformTags are the wheel platform tags for one target. Go builds
// with CGO_ENABLED=0 are static, so the Linux wheels claim the oldest
// glibc manylinux tag pip accepts for both CPUs (2014 / 2_17) and
// musllinux_1_1. The macOS tag is the binary's own minimum OS version
// from its LC_BUILD_VERSION, so pip never installs it on a macOS the Go
// runtime refuses.
func platformTags(t target, bin []byte) ([]string, error) {
	switch t {
	case target{"linux", "amd64"}:
		return []string{"manylinux_2_17_x86_64", "manylinux2014_x86_64", "musllinux_1_1_x86_64"}, nil
	case target{"linux", "arm64"}:
		return []string{"manylinux_2_17_aarch64", "manylinux2014_aarch64", "musllinux_1_1_aarch64"}, nil
	case target{"windows", "amd64"}:
		return []string{"win_amd64"}, nil
	case target{"windows", "arm64"}:
		return []string{"win_arm64"}, nil
	case target{"darwin", "amd64"}, target{"darwin", "arm64"}:
		major, minor, err := machoMinOS(bin)
		if err != nil {
			return nil, err
		}
		arch := "x86_64"
		if t.GOARCH == "arm64" {
			arch = "arm64"
		}
		if major >= 11 {
			minor = 0
		}
		return []string{fmt.Sprintf("macosx_%d_%d_%s", major, minor, arch)}, nil
	}
	return nil, fmt.Errorf("no wheel platform for %s/%s", t.GOOS, t.GOARCH)
}

const lcBuildVersion = 0x32

func machoMinOS(bin []byte) (int, int, error) {
	f, err := macho.NewFile(bytes.NewReader(bin))
	if err != nil {
		return 0, 0, fmt.Errorf("read Mach-O: %w", err)
	}
	defer func() { _ = f.Close() }()
	for _, l := range f.Loads {
		raw := l.Raw()
		if len(raw) >= 16 && f.ByteOrder.Uint32(raw) == lcBuildVersion {
			v := f.ByteOrder.Uint32(raw[12:])
			return int(v >> 16), int((v >> 8) & 0xff), nil
		}
	}
	return 0, 0, errors.New("the Mach-O binary has no LC_BUILD_VERSION")
}

// wheelFile is one file in a wheel, in write order.
type wheelFile struct {
	name string
	data []byte
	mode os.FileMode
}

func buildWheels(ver, dist, src, out string) ([]string, error) {
	digests, err := readSums(filepath.Join(dist, "SHA256SUMS"), ver)
	if err != nil {
		return nil, err
	}
	readme, err := os.ReadFile(filepath.Join(src, "README.md"))
	if err != nil {
		return nil, err
	}
	var pyFiles []wheelFile
	for _, n := range []string{"__init__.py", "__main__.py"} {
		data, err := os.ReadFile(filepath.Join(src, "tripcord", n))
		if err != nil {
			return nil, err
		}
		pyFiles = append(pyFiles, wheelFile{name: "tripcord/" + n, data: data, mode: 0o644})
	}
	// Check every archive before writing any wheel.
	payloads := map[target]payload{}
	for _, t := range targets {
		raw, err := verifyFile(filepath.Join(dist, t.archive(ver)), digests[t.archive(ver)])
		if err != nil {
			return nil, err
		}
		p, err := readArchive(t, raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", t.archive(ver), err)
		}
		payloads[t] = p
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return nil, err
	}
	var names []string
	for _, t := range targets {
		p := payloads[t]
		tags, err := platformTags(t, p.binary)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", t.archive(ver), err)
		}
		name, data, err := wheel(ver, t, tags, p, pyFiles, readme)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(out, name), data, 0o644); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, nil
}

func metadata(ver string, readme []byte) []byte {
	var b strings.Builder
	b.WriteString("Metadata-Version: 2.4\n")
	b.WriteString("Name: tripcord\n")
	fmt.Fprintf(&b, "Version: %s\n", ver)
	fmt.Fprintf(&b, "Summary: %s\n", summary)
	b.WriteString("License-Expression: Apache-2.0\n")
	for _, n := range noticeFiles {
		fmt.Fprintf(&b, "License-File: %s\n", n)
	}
	b.WriteString("Requires-Python: >=3.8\n")
	fmt.Fprintf(&b, "Project-URL: Homepage, %s\n", repoURL)
	fmt.Fprintf(&b, "Project-URL: Source, %s\n", repoURL)
	fmt.Fprintf(&b, "Project-URL: Changelog, %s/releases/tag/v%s\n", repoURL, ver)
	b.WriteString("Description-Content-Type: text/markdown\n\n")
	b.Write(readme)
	return []byte(b.String())
}

func wheel(ver string, t target, tags []string, p payload, pyFiles []wheelFile, readme []byte) (string, []byte, error) {
	distInfo := "tripcord-" + ver + ".dist-info"
	var wheelMeta strings.Builder
	wheelMeta.WriteString("Wheel-Version: 1.0\nGenerator: tripcord release-packages\nRoot-Is-Purelib: false\n")
	for _, tag := range tags {
		fmt.Fprintf(&wheelMeta, "Tag: py3-none-%s\n", tag)
	}
	files := append([]wheelFile{}, pyFiles...)
	files = append(files, wheelFile{name: "tripcord/bin/" + t.binary(), data: p.binary, mode: 0o755})
	for _, n := range noticeFiles {
		files = append(files, wheelFile{name: distInfo + "/licenses/" + n, data: p.notices[n], mode: 0o644})
	}
	files = append(files,
		wheelFile{name: distInfo + "/METADATA", data: metadata(ver, readme), mode: 0o644},
		wheelFile{name: distInfo + "/WHEEL", data: []byte(wheelMeta.String()), mode: 0o644},
		wheelFile{name: distInfo + "/entry_points.txt", data: []byte("[console_scripts]\ntripcord = tripcord:main\n"), mode: 0o644},
	)
	var record strings.Builder
	for _, f := range files {
		sum := sha256.Sum256(f.data)
		fmt.Fprintf(&record, "%s,sha256=%s,%d\n", f.name, base64.RawURLEncoding.EncodeToString(sum[:]), len(f.data))
	}
	fmt.Fprintf(&record, "%s/RECORD,,\n", distInfo)
	files = append(files, wheelFile{name: distInfo + "/RECORD", data: []byte(record.String()), mode: 0o644})

	modified := p.modified.UTC()
	if modified.Year() < 1980 {
		modified = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		h := &zip.FileHeader{Name: f.name, Method: zip.Deflate, Modified: modified}
		h.SetMode(f.mode)
		w, err := zw.CreateHeader(h)
		if err != nil {
			return "", nil, err
		}
		if _, err := w.Write(f.data); err != nil {
			return "", nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return "", nil, err
	}
	name := fmt.Sprintf("tripcord-%s-py3-none-%s.whl", ver, strings.Join(tags, "."))
	return name, buf.Bytes(), nil
}

// npmFiles are the files of packaging/npm that ship; tests stay behind.
var npmFiles = []string{"package.json", "README.md", "bin/tripcord.js", "lib/tripcord.js"}

func stageNPM(ver, sums, src, license, out string) error {
	rawSums, err := os.ReadFile(sums)
	if err != nil {
		return err
	}
	if _, err := parseSums(rawSums, ver); err != nil {
		return err
	}
	lic, err := os.ReadFile(license)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	if entries, err := os.ReadDir(out); err != nil {
		return err
	} else if len(entries) != 0 {
		return fmt.Errorf("%s is not empty", out)
	}
	for _, rel := range npmFiles {
		data, err := os.ReadFile(filepath.Join(src, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if rel == "package.json" {
			if n := bytes.Count(data, []byte(devNPMV)); n != 1 {
				return fmt.Errorf("package.json has %d %s lines, want 1", n, devNPMV)
			}
			data = bytes.Replace(data, []byte(devNPMV), []byte(`"version": "`+ver+`"`), 1)
		}
		if rel == "bin/tripcord.js" {
			mode = 0o755
		}
		dst := filepath.Join(out, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, mode); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(out, "LICENSE"), lic, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "SHA256SUMS"), rawSums, 0o644)
}
