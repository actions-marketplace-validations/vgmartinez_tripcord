package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite testdata/tripcord.rb.golden")

const fixtureVersion = "1.2.3"

// TestFormulaGolden renders the formula for testdata/SHA256SUMS.
func TestFormulaGolden(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"formula", "-version", "v" + fixtureVersion, "-sums", filepath.Join("testdata", "SHA256SUMS")}, &out); err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join("testdata", "tripcord.rb.golden")
	if *update {
		if err := os.WriteFile(golden, out.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("formula differs from %s (go test ./scripts/release-packages -update):\n%s", golden, out.String())
	}
}

// TestFormulaSumsMatchSHA256SUMS checks every url/sha256 pair in the
// formula against the SHA256SUMS line for that archive, and that only
// the four macOS and Linux archives appear.
func TestFormulaSumsMatchSHA256SUMS(t *testing.T) {
	digests, err := readSums(filepath.Join("testdata", "SHA256SUMS"), fixtureVersion)
	if err != nil {
		t.Fatal(err)
	}
	text, err := formula(fixtureVersion, digests)
	if err != nil {
		t.Fatal(err)
	}
	pair := regexp.MustCompile(`(?m)^      url "https://github\.com/vgmartinez/tripcord/releases/download/v1\.2\.3/(\S+)"\n      sha256 "([0-9a-f]{64})"$`)
	var seen []string
	for _, m := range pair.FindAllStringSubmatch(text, -1) {
		if digests[m[1]] != m[2] {
			t.Errorf("%s: formula sha256 %s, SHA256SUMS %s", m[1], m[2], digests[m[1]])
		}
		seen = append(seen, m[1])
	}
	slices.Sort(seen)
	want := []string{
		"tripcord_1.2.3_darwin_amd64.tar.gz", "tripcord_1.2.3_darwin_arm64.tar.gz",
		"tripcord_1.2.3_linux_amd64.tar.gz", "tripcord_1.2.3_linux_arm64.tar.gz",
	}
	if !slices.Equal(seen, want) {
		t.Fatalf("formula archives = %v, want %v", seen, want)
	}
	for _, s := range []string{`test do`, `shell_output("#{bin}/tripcord --version")`, `license "Apache-2.0"`, `on_macos do`, `on_linux do`, `Hardware::CPU.arm?`, `Hardware::CPU.intel?`} {
		if !strings.Contains(text, s) {
			t.Errorf("formula is missing %s", s)
		}
	}
}

func fixtureSums(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestParseSumsFailsClosed(t *testing.T) {
	good := fixtureSums(t)
	lines := strings.SplitAfter(good, "\n")
	if _, err := parseSums([]byte(good), fixtureVersion); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	cases := map[string]string{
		"missing archive":   strings.Join(lines[1:], ""),
		"extra file":        good + strings.Repeat("0", 64) + "  tripcord_1.2.3_freebsd_amd64.tar.gz\n",
		"duplicate":         good + lines[0],
		"uppercase digest":  strings.ToUpper(lines[0][:64]) + lines[0][64:] + strings.Join(lines[1:], ""),
		"one space":         strings.Replace(good, "  ", " ", 1),
		"binary-mode star":  strings.Replace(good, "  ", " *", 1),
		"crlf":              strings.ReplaceAll(good, "\n", "\r\n"),
		"no final newline":  strings.TrimSuffix(good, "\n"),
		"other version":     strings.ReplaceAll(good, "1.2.3", "1.2.4"),
		"short digest":      lines[0][1:] + strings.Join(lines[1:], ""),
		"blank line":        good + "\n",
		"path in file name": strings.Replace(good, "  tripcord_", "  ./tripcord_", 1),
	}
	for name, text := range cases {
		if _, err := parseSums([]byte(text), fixtureVersion); err == nil {
			t.Errorf("%s: parseSums accepted it", name)
		}
	}
}

func TestVersionMustBePlainSemver(t *testing.T) {
	for _, bad := range []string{"", "0.1.0", "v0.1", "v0.1.0-rc.1", "v01.0.0", "v0.1.0+meta", "v0.1.0\n"} {
		if _, err := plainVersion(bad); err == nil {
			t.Errorf("plainVersion(%q) succeeded", bad)
		}
	}
	if v, err := plainVersion("v0.1.0"); err != nil || v != "0.1.0" {
		t.Fatalf("plainVersion(v0.1.0) = %q, %v", v, err)
	}
}

// fakeMachO is a minimal 64-bit Mach-O with one LC_BUILD_VERSION for
// macOS 13.0, enough for platformTags.
func fakeMachO(arm bool) []byte {
	var b bytes.Buffer
	le := binary.LittleEndian
	cpu := uint32(0x01000007) // x86_64
	if arm {
		cpu = 0x0100000c // arm64
	}
	for _, v := range []uint32{0xfeedfacf, cpu, 0, 2, 1, 24, 0, 0} {
		_ = binary.Write(&b, le, v)
	}
	for _, v := range []uint32{lcBuildVersion, 24, 1, 13 << 16, 14 << 16, 0} {
		_ = binary.Write(&b, le, v)
	}
	return b.Bytes()
}

// fakeBinary is what each fake archive holds as tripcord: a shell script
// on Linux, so the Python entry point can really exec it.
func fakeBinary(tg target) []byte {
	switch tg.GOOS {
	case "darwin":
		return fakeMachO(tg.GOARCH == "arm64")
	case "linux":
		return []byte("#!/bin/sh\necho \"v1.2.3 (commit fake) $*\"\n")
	}
	return []byte("MZ fake " + tg.GOARCH)
}

var fixtureTime = time.Date(2026, 11, 11, 7, 18, 0, 0, time.UTC)

func fakeArchive(t *testing.T, tg target, extra string) []byte {
	t.Helper()
	files := []struct {
		name string
		data []byte
		mode int64
	}{
		{tg.binary(), fakeBinary(tg), 0o755},
		{"LICENSE", []byte("Apache License 2.0\n"), 0o644},
		{"NOTICE", []byte("notice\n"), 0o644},
		{"THIRD_PARTY_NOTICES", []byte("third party\n"), 0o644},
	}
	if extra != "" {
		files = append(files, struct {
			name string
			data []byte
			mode int64
		}{extra, []byte("x"), 0o644})
	}
	var buf bytes.Buffer
	if tg.GOOS == "windows" {
		zw := zip.NewWriter(&buf)
		for _, f := range files {
			h := &zip.FileHeader{Name: f.name, Method: zip.Deflate, Modified: fixtureTime}
			h.SetMode(os.FileMode(f.mode))
			w, err := zw.CreateHeader(h)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = w.Write(f.data)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range files {
		if err := tw.WriteHeader(&tar.Header{Name: f.name, Mode: f.mode, Size: int64(len(f.data)), ModTime: fixtureTime, Typeflag: tar.TypeReg, Format: tar.FormatUSTAR}); err != nil {
			t.Fatal(err)
		}
		_, _ = tw.Write(f.data)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fakeDist writes six fake archives and their SHA256SUMS. extra adds an
// unexpected entry to the linux/amd64 archive.
func fakeDist(t *testing.T, extra string) string {
	t.Helper()
	dir := t.TempDir()
	var sums strings.Builder
	for _, tg := range targets {
		e := ""
		if tg == (target{"linux", "amd64"}) {
			e = extra
		}
		raw := fakeArchive(t, tg, e)
		name := tg.archive(fixtureVersion)
		if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(raw)
		fmt.Fprintf(&sums, "%s  %s\n", hex.EncodeToString(sum[:]), name)
	}
	if err := os.WriteFile(filepath.Join(dir, "SHA256SUMS"), []byte(sums.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

var pypiSrc = filepath.Join("..", "..", "packaging", "pypi")

func TestWheels(t *testing.T) {
	dist := fakeDist(t, "")
	out := t.TempDir()
	var stdout bytes.Buffer
	if err := run([]string{"wheels", "-version", "v" + fixtureVersion, "-dist", dist, "-src", pypiSrc, "-out", out}, &stdout); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"tripcord-1.2.3-py3-none-macosx_13_0_x86_64.whl",
		"tripcord-1.2.3-py3-none-macosx_13_0_arm64.whl",
		"tripcord-1.2.3-py3-none-manylinux_2_17_x86_64.manylinux2014_x86_64.musllinux_1_1_x86_64.whl",
		"tripcord-1.2.3-py3-none-manylinux_2_17_aarch64.manylinux2014_aarch64.musllinux_1_1_aarch64.whl",
		"tripcord-1.2.3-py3-none-win_amd64.whl",
		"tripcord-1.2.3-py3-none-win_arm64.whl",
	}
	if got := strings.Fields(stdout.String()); !slices.Equal(got, want) {
		t.Fatalf("wheels = %v, want %v", got, want)
	}
	for i, tg := range targets {
		checkWheel(t, filepath.Join(out, want[i]), tg)
	}
	// Same inputs, same bytes.
	again := t.TempDir()
	if err := run([]string{"wheels", "-version", "v" + fixtureVersion, "-dist", dist, "-src", pypiSrc, "-out", again}, io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, n := range want {
		a, _ := os.ReadFile(filepath.Join(out, n))
		b, _ := os.ReadFile(filepath.Join(again, n))
		if !bytes.Equal(a, b) {
			t.Errorf("%s is not reproducible", n)
		}
	}
	if runtime.GOOS == "linux" {
		runLinuxWheel(t, filepath.Join(out, want[2]))
	}
}

func checkWheel(t *testing.T, path string, tg target) {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = zr.Close() }()
	files := map[string][]byte{}
	var order []string
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(rc)
		_ = rc.Close()
		files[f.Name] = data
		order = append(order, f.Name)
		mode := f.Mode().Perm()
		if f.Name == "tripcord/bin/"+tg.binary() {
			if mode != 0o755 {
				t.Errorf("%s: binary mode %o", path, mode)
			}
		} else if mode != 0o644 {
			t.Errorf("%s: %s mode %o", path, f.Name, mode)
		}
	}
	di := "tripcord-1.2.3.dist-info/"
	if order[len(order)-1] != di+"RECORD" {
		t.Errorf("%s: RECORD is not last", path)
	}
	if !bytes.Equal(files["tripcord/bin/"+tg.binary()], fakeBinary(tg)) {
		t.Errorf("%s: the bundled binary is not the archive's binary", path)
	}
	for _, n := range noticeFiles {
		if files[di+"licenses/"+n] == nil {
			t.Errorf("%s: no licenses/%s", path, n)
		}
	}
	if !strings.Contains(string(files[di+"entry_points.txt"]), "tripcord = tripcord:main") {
		t.Errorf("%s: entry point missing", path)
	}
	meta := string(files[di+"METADATA"])
	for _, s := range []string{"Name: tripcord\n", "Version: 1.2.3\n", "License-Expression: Apache-2.0\n", "Requires-Python: >=3.8\n", "https://github.com/vgmartinez/tripcord"} {
		if !strings.Contains(meta, s) {
			t.Errorf("%s: METADATA missing %q", path, s)
		}
	}
	if strings.Contains(strings.ToLower(meta), "neuraltrust") {
		t.Errorf("%s: METADATA names a company", path)
	}
	if !strings.Contains(string(files[di+"WHEEL"]), "Root-Is-Purelib: false\n") {
		t.Errorf("%s: WHEEL is purelib", path)
	}
	// RECORD lists every other file with its urlsafe sha256 and size.
	sc := bufio.NewScanner(bytes.NewReader(files[di+"RECORD"]))
	listed := map[string]bool{}
	for sc.Scan() {
		parts := strings.Split(sc.Text(), ",")
		if len(parts) != 3 {
			t.Fatalf("%s: RECORD line %q", path, sc.Text())
		}
		listed[parts[0]] = true
		if parts[0] == di+"RECORD" {
			continue
		}
		sum := sha256.Sum256(files[parts[0]])
		if parts[1] != "sha256="+base64.RawURLEncoding.EncodeToString(sum[:]) || parts[2] != fmt.Sprint(len(files[parts[0]])) {
			t.Errorf("%s: RECORD entry for %s is wrong", path, parts[0])
		}
	}
	for n := range files {
		if !listed[n] {
			t.Errorf("%s: RECORD does not list %s", path, n)
		}
	}
}

// runLinuxWheel unpacks the linux/amd64 wheel and runs python3 -m
// tripcord with it on the path: the entry module must exec the bundled
// binary with the same arguments.
func runLinuxWheel(t *testing.T, whl string) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("no python3")
	}
	dir := t.TempDir()
	zr, err := zip.OpenReader(whl)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = zr.Close() }()
	for _, f := range zr.File {
		dst := filepath.Join(dir, filepath.FromSlash(f.Name))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		_ = rc.Close()
		if err := os.WriteFile(dst, data, f.Mode().Perm()); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(py, "-m", "tripcord", "--version", "a b")
	cmd.Env = append(os.Environ(), "PYTHONPATH="+dir, "PYTHONDONTWRITEBYTECODE=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python3 -m tripcord: %v\n%s", err, out)
	}
	if string(out) != "v1.2.3 (commit fake) --version a b\n" {
		t.Fatalf("python3 -m tripcord printed %q", out)
	}
}

func TestWheelsFailClosedOnChecksumMismatch(t *testing.T) {
	dist := fakeDist(t, "")
	victim := filepath.Join(dist, target{"windows", "arm64"}.archive(fixtureVersion))
	raw, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)/2] ^= 0xff
	if err := os.WriteFile(victim, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "wheels")
	err = run([]string{"wheels", "-version", "v" + fixtureVersion, "-dist", dist, "-src", pypiSrc, "-out", out}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "does not match SHA256SUMS") {
		t.Fatalf("err = %v, want a checksum mismatch", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("a mismatch still created %s", out)
	}
}

func TestWheelsFailOnMissingArchiveOrExtraEntry(t *testing.T) {
	dist := fakeDist(t, "")
	if err := os.Remove(filepath.Join(dist, target{"darwin", "amd64"}.archive(fixtureVersion))); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"wheels", "-version", "v" + fixtureVersion, "-dist", dist, "-src", pypiSrc, "-out", t.TempDir()}, io.Discard); err == nil {
		t.Error("a missing archive was accepted")
	}
	dist = fakeDist(t, "../escape")
	err := run([]string{"wheels", "-version", "v" + fixtureVersion, "-dist", dist, "-src", pypiSrc, "-out", t.TempDir()}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "unexpected archive entry") {
		t.Errorf("err = %v, want an unexpected entry", err)
	}
}

func TestStageNPM(t *testing.T) {
	out := filepath.Join(t.TempDir(), "npm")
	src := filepath.Join("..", "..", "packaging", "npm")
	lic := filepath.Join("..", "..", "LICENSE")
	sums := filepath.Join("testdata", "SHA256SUMS")
	if err := run([]string{"npm", "-version", "v" + fixtureVersion, "-sums", sums, "-src", src, "-license", lic, "-out", out}, io.Discard); err != nil {
		t.Fatal(err)
	}
	pkg, err := os.ReadFile(filepath.Join(out, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pkg), `"version": "1.2.3"`) || strings.Contains(string(pkg), "0.0.0-dev") {
		t.Fatalf("staged package.json version:\n%s", pkg)
	}
	var got []string
	_ = filepath.WalkDir(out, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(out, p)
			got = append(got, filepath.ToSlash(rel))
		}
		return err
	})
	slices.Sort(got)
	want := []string{"LICENSE", "README.md", "SHA256SUMS", "bin/tripcord.js", "lib/tripcord.js", "package.json"}
	if !slices.Equal(got, want) {
		t.Fatalf("staged files = %v, want %v", got, want)
	}
	staged, _ := os.ReadFile(filepath.Join(out, "SHA256SUMS"))
	if string(staged) != fixtureSums(t) {
		t.Error("staged SHA256SUMS differs from the release's")
	}
	if fi, err := os.Stat(filepath.Join(out, "bin", "tripcord.js")); err != nil || (runtime.GOOS != "windows" && fi.Mode().Perm() != 0o755) {
		t.Errorf("bin/tripcord.js mode: %v %v", fi, err)
	}
	// A second stage into the same directory is refused.
	if err := run([]string{"npm", "-version", "v" + fixtureVersion, "-sums", sums, "-src", src, "-license", lic, "-out", out}, io.Discard); err == nil {
		t.Error("staging into a non-empty directory succeeded")
	}
	// Bad sums are refused before anything is written.
	bad := filepath.Join(t.TempDir(), "SHA256SUMS")
	_ = os.WriteFile(bad, []byte(strings.SplitAfter(fixtureSums(t), "\n")[0]), 0o644)
	out2 := filepath.Join(t.TempDir(), "npm")
	if err := run([]string{"npm", "-version", "v" + fixtureVersion, "-sums", bad, "-src", src, "-license", lic, "-out", out2}, io.Discard); err == nil {
		t.Error("incomplete SHA256SUMS was staged")
	}
	if _, err := os.Stat(out2); !os.IsNotExist(err) {
		t.Error("bad sums still created the output directory")
	}
}

func TestPackageTextNamesOnlyTripcord(t *testing.T) {
	for _, p := range []string{
		filepath.Join("..", "..", "packaging", "npm", "README.md"),
		filepath.Join("..", "..", "packaging", "npm", "package.json"),
		filepath.Join("..", "..", "packaging", "pypi", "README.md"),
		filepath.Join("testdata", "tripcord.rb.golden"),
	} {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		low := strings.ToLower(string(raw))
		for _, bad := range []string{"neuraltrust", "telemetry\":", "postinstall"} {
			if strings.Contains(low, bad) {
				t.Errorf("%s mentions %q", p, bad)
			}
		}
		if !strings.Contains(string(raw), "Apache") {
			t.Errorf("%s does not name the license", p)
		}
	}
}
