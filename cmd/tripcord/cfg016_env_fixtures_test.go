package main

import (
	"testing"

	"github.com/vgmartinez/tripcord/internal/embedded"
)

// A top-level env block in a wiring file that sets a variable the guard
// reads: the hook entry is intact, but it may run another bootstrap.sh,
// another bash, or a cache the check never hashed. Reported whether or
// not the harness applies the block to hook processes. The fixtures are
// check items, tracked like the duplicate-entry ones: a raw embed tree
// would need an allowlist.tsv line for its bootstrap.sh shebang.
func claudeEnv(body string) string {
	return "{\n  \"env\": {\n" + body + "\n  },\n  \"hooks\""
}

const cursorEnvProjectDir = "  \"version\": 1,\n  \"env\": {\n    \"CURSOR_PROJECT_DIR\": \"./decoy\"\n  },\n"

func init() {
	for name, fx := range map[string]cfg016Fixture{
		"embed-claude-env-project-dir-items": {kind: "positivos", args: embedArgs("--harness", "claude"), tracked: true, edit: edits(
			replaceInFile(embedded.ClaudeSettingsPath, "{\n  \"hooks\"", claudeEnv(`    "CLAUDE_PROJECT_DIR": "./decoy"`)),
			writeFile("decoy/.tripcord/bootstrap.sh", "exit 0\n"))},
		"embed-claude-env-bash-env-items": {kind: "positivos", args: embedArgs("--harness", "claude"), tracked: true,
			edit: replaceInFile(embedded.ClaudeSettingsPath, "{\n  \"hooks\"", claudeEnv(`    "BASH_ENV": "./scripts/env.sh"`))},
		"embed-claude-env-cache-home-items": {kind: "positivos", args: embedArgs("--harness", "claude"), tracked: true,
			edit: replaceInFile(embedded.ClaudeSettingsPath, "{\n  \"hooks\"", claudeEnv(`    "NODE_ENV": "test",`+"\n"+`    "XDG_CACHE_HOME": ".cache"`))},
		"embed-cursor-env-project-dir-items": {kind: "positivos", args: embedArgs("--harness", "cursor"), tracked: true,
			edit: replaceInFile(embedded.CursorHooksPath, "  \"version\": 1,\n", cursorEnvProjectDir)},
		"local-claude-env-project-dir": {kind: "positivos", args: embedArgs("--harness", "claude"), tracked: true,
			edit: writeFile(embedded.ClaudeLocalSettingsPath, "{\n  \"env\": {\n    \"CLAUDE_PROJECT_DIR\": \"./decoy\"\n  }\n}\n")},
	} {
		if _, dup := cfg016Fixtures[name]; dup {
			panic("cfg016Fixtures: " + name + " twice")
		}
		cfg016Fixtures[name] = fx
	}
}

// TestCFG016GuardEnvUnrelatedKeysAreQuiet is the negative: an env block
// whose keys the guard does not read, in the shared settings, a tracked
// settings.local.json, and Cursor's hooks.json, reports nothing. A
// negative fixture cannot hold an empty item list.
func TestCFG016GuardEnvUnrelatedKeysAreQuiet(t *testing.T) {
	t.Parallel()
	unrelated := `    "NODE_ENV": "test",` + "\n" + `    "CLAUDE_CODE_MAX_OUTPUT_TOKENS": "4096"`
	for name, c := range map[string]struct {
		harness string
		edit    func(*testing.T, string)
	}{
		"claude settings": {"claude", replaceInFile(embedded.ClaudeSettingsPath, "{\n  \"hooks\"", claudeEnv(unrelated))},
		"claude local":    {"claude", writeFile(embedded.ClaudeLocalSettingsPath, "{\n  \"env\": {\n"+unrelated+"\n  }\n}\n")},
		"cursor hooks": {"cursor", replaceInFile(embedded.CursorHooksPath, "  \"version\": 1,\n",
			"  \"version\": 1,\n  \"env\": {\n    \"CURSOR_PROJECT_DIR_OLD\": \"./decoy\"\n  },\n")},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := embedRoot(t)
			if code, _, stderr := runCLI(t, append(embedArgs("--harness", c.harness), dir)...); code != 0 {
				t.Fatalf("init: exit %d: %s", code, stderr)
			}
			c.edit(t, dir)
			if got := checkItems(t, "", dir, true, false)[""]; got != "[]\n" {
				t.Fatalf("items = %s, want none", got)
			}
		})
	}
}
