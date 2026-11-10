package main

import (
	"github.com/vgmartinez/tripcord/internal/embedded"
)

// A Cursor hook file whose "version" is not the 1 init writes. init
// refuses to merge into such a file, so the guard reports it too. A
// Copilot version other than 1, and a Cursor file with no version, are
// already adapter diagnostics: the scan is incomplete. The fixture is
// check items, so it needs no allowlist.tsv line for bootstrap.sh.
func init() {
	for name, fx := range map[string]cfg016Fixture{
		"embed-cursor-hooks-version-2-items": {kind: "positivos", args: embedArgs("--harness", "cursor"), tracked: true,
			edit: replaceInFile(embedded.CursorHooksPath, "  \"version\": 1,\n", "  \"version\": 2,\n")},
	} {
		if _, dup := cfg016Fixtures[name]; dup {
			panic("cfg016Fixtures: " + name + " twice")
		}
		cfg016Fixtures[name] = fx
	}
}
