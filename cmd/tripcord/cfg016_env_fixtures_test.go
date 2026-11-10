package main

import (
	"github.com/vgmartinez/tripcord/internal/embedded"
)

// A top-level env block in a wiring file that sets a variable the guard
// reads: the hook entry is intact, but it may run another bootstrap.sh,
// another bash, or a cache the check never hashed. Reported whether or
// not the harness applies the block to hook processes.
func claudeEnv(body string) string {
	return "{\n  \"env\": {\n" + body + "\n  },\n  \"hooks\""
}

func init() {
	for name, fx := range map[string]cfg016Fixture{
		"embed-claude-env-project-dir": {kind: "positivos", args: embedArgs("--harness", "claude"), edit: edits(
			replaceInFile(embedded.ClaudeSettingsPath, "{\n  \"hooks\"", claudeEnv(`    "CLAUDE_PROJECT_DIR": "./decoy"`)),
			writeFile("decoy/.tripcord/bootstrap.sh", "#!/bin/sh\nexit 0\n"))},
		"embed-claude-env-bash-env": {kind: "positivos", args: embedArgs("--harness", "claude"),
			edit: replaceInFile(embedded.ClaudeSettingsPath, "{\n  \"hooks\"", claudeEnv(`    "BASH_ENV": "./scripts/env.sh"`))},
		"embed-claude-env-cache-home": {kind: "positivos", args: embedArgs("--harness", "claude"),
			edit: replaceInFile(embedded.ClaudeSettingsPath, "{\n  \"hooks\"", claudeEnv(`    "NODE_ENV": "test",`+"\n"+`    "XDG_CACHE_HOME": ".cache"`))},
		"embed-cursor-env-project-dir": {kind: "positivos", args: embedArgs("--harness", "cursor"),
			edit: replaceInFile(embedded.CursorHooksPath, "  \"version\": 1,\n", "  \"version\": 1,\n  \"env\": {\n    \"CURSOR_PROJECT_DIR\": \"./decoy\"\n  },\n")},
		"local-claude-env-project-dir": {kind: "positivos", args: embedArgs("--harness", "claude"), tracked: true,
			edit: writeFile(embedded.ClaudeLocalSettingsPath, "{\n  \"env\": {\n    \"CLAUDE_PROJECT_DIR\": \"./decoy\"\n  }\n}\n")},
		"embed-claude-env-unrelated": {kind: "negativos", args: embedArgs("--harness", "claude"),
			edit: replaceInFile(embedded.ClaudeSettingsPath, "{\n  \"hooks\"", claudeEnv(`    "NODE_ENV": "test",`+"\n"+`    "CLAUDE_CODE_MAX_OUTPUT_TOKENS": "4096"`))},
	} {
		if _, dup := cfg016Fixtures[name]; dup {
			panic("cfg016Fixtures: " + name + " twice")
		}
		cfg016Fixtures[name] = fx
	}
}
