package main

import (
	"bytes"
	"errors"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

type linkedModule struct {
	path, version, dir string
}

// linkedModules is every module other than this one that ./cmd/tripcord
// links for goos, as go version -m lists it for that binary.
func linkedModules(t *testing.T, root, goos string) []linkedModule {
	t.Helper()
	cmd := exec.Command("go", "list", "-deps", "-f", "{{with .Module}}{{if not .Main}}{{.Path}}\t{{.Version}}\t{{.Dir}}{{end}}{{end}}", "./cmd/tripcord")
	cmd.Dir = root
	cmd.Env = append(cmd.Environ(), "GOOS="+goos, "GOARCH=amd64", "CGO_ENABLED=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list for %s: %v\n%s", goos, err, stderr.String())
	}
	seen := map[string]linkedModule{}
	for line := range strings.Lines(string(out)) {
		line = strings.TrimRight(line, "\n")
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 3 || f[0] == "" || f[1] == "" || f[2] == "" {
			t.Fatalf("go list for %s: unexpected line %q", goos, line)
		}
		seen[f[0]] = linkedModule{path: f[0], version: f[1], dir: f[2]}
	}
	var mods []linkedModule
	for _, p := range slices.Sorted(maps.Keys(seen)) {
		mods = append(mods, seen[p])
	}
	return mods
}

var noticesModuleEntry = regexp.MustCompile(`(?m)^Module: (\S+)\nVersion: (\S+)\n`)

// TestNoticesCoverLinkedModules fails when a module linked into the
// tripcord binary on linux, darwin, or windows is missing from NOTICE or
// THIRD_PARTY_NOTICES, when either names a module the binary does not
// link, or when a module's LICENSE, NOTICE, or PATENTS file is not
// reproduced verbatim.
func TestNoticesCoverLinkedModules(t *testing.T) {
	root := filepath.Join("..", "..")
	read := func(name string) string {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	notice, thirdParty, license := read("NOTICE"), read("THIRD_PARTY_NOTICES"), read("LICENSE")

	linked := map[string]linkedModule{}
	for _, goos := range []string{"linux", "darwin", "windows"} {
		for _, m := range linkedModules(t, root, goos) {
			linked[m.path] = m
		}
	}
	for _, want := range []string{
		"github.com/bmatcuk/doublestar/v4", "github.com/pelletier/go-toml/v2", "go.yaml.in/yaml/v3",
		"github.com/antlr4-go/antlr/v4", "google.golang.org/protobuf", "golang.org/x/exp",
		"golang.org/x/text", "cel.dev/cel-go", "google.golang.org/genproto/googleapis/api",
		"google.golang.org/genproto/googleapis/rpc",
	} {
		if _, ok := linked[want]; !ok {
			t.Errorf("go list does not report %s as linked; the module query is broken", want)
		}
	}

	listed := map[string]string{}
	for _, m := range noticesModuleEntry.FindAllStringSubmatch(thirdParty, -1) {
		if _, dup := listed[m[1]]; dup {
			t.Errorf("THIRD_PARTY_NOTICES lists %s twice", m[1])
		}
		listed[m[1]] = m[2]
	}
	for _, p := range slices.Sorted(maps.Keys(listed)) {
		if _, ok := linked[p]; !ok {
			t.Errorf("THIRD_PARTY_NOTICES lists %s, which the binary does not link", p)
		}
	}
	noticeLine := regexp.MustCompile(`(?m)^  (\S+) (\S+) \(`)
	inNotice := map[string]string{}
	for _, m := range noticeLine.FindAllStringSubmatch(notice, -1) {
		inNotice[m[1]] = m[2]
	}
	for _, p := range slices.Sorted(maps.Keys(inNotice)) {
		if _, ok := linked[p]; !ok {
			t.Errorf("NOTICE lists %s, which the binary does not link", p)
		}
	}

	for _, p := range slices.Sorted(maps.Keys(linked)) {
		m := linked[p]
		if v, ok := listed[p]; !ok {
			t.Errorf("THIRD_PARTY_NOTICES has no Module: %s entry", p)
		} else if v != m.version {
			t.Errorf("THIRD_PARTY_NOTICES lists %s %s, the binary links %s", p, v, m.version)
		}
		if v, ok := inNotice[p]; !ok {
			t.Errorf("NOTICE does not list %s", p)
		} else if v != m.version {
			t.Errorf("NOTICE lists %s %s, the binary links %s", p, v, m.version)
		}
		files := map[string]string{}
		for _, name := range []string{"LICENSE", "NOTICE", "PATENTS"} {
			raw, err := os.ReadFile(filepath.Join(m.dir, name))
			switch {
			case errors.Is(err, fs.ErrNotExist):
				continue
			case err != nil:
				t.Fatal(err)
			}
			files[name] = string(raw)
		}
		text, ok := files["LICENSE"]
		switch {
		case !ok:
			t.Errorf("%s has no LICENSE file to reproduce", p)
		case text == license:
			if !strings.Contains(thirdParty, "Module: "+p+"\nVersion: "+m.version+"\n") ||
				!strings.Contains(thirdParty, "Module: "+p+"\nVersion: "+m.version+"\nLicense: Apache-2.0\nLicense file: LICENSE (identical to this repository's LICENSE)\n") {
				t.Errorf("THIRD_PARTY_NOTICES does not record that %s's LICENSE is this repository's LICENSE", p)
			}
		case !strings.Contains(thirdParty, text):
			t.Errorf("THIRD_PARTY_NOTICES does not reproduce %s's LICENSE verbatim", p)
		}
		if text, ok := files["NOTICE"]; ok {
			if !strings.Contains(thirdParty, text) {
				t.Errorf("THIRD_PARTY_NOTICES does not reproduce %s's NOTICE verbatim", p)
			}
			if !strings.Contains(notice, text) {
				t.Errorf("NOTICE does not reproduce %s's NOTICE verbatim", p)
			}
		}
		if text, ok := files["PATENTS"]; ok && !strings.Contains(thirdParty, text) {
			t.Errorf("THIRD_PARTY_NOTICES does not reproduce %s's PATENTS verbatim", p)
		}
	}
}
