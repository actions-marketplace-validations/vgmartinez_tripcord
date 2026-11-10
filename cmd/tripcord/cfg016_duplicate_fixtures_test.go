package main

import (
	"github.com/vgmartinez/tripcord/internal/embedded"
)

// A second copy of the guard entry init writes, byte for byte, under the
// same event. Exactly one registered entry is allowed: the harness runs
// every copy. Pre-tool copies go before the guard, session-start copies
// after it.
const (
	dupClaudePreTool = `      {
        "matcher": "*",
        "hooks": [
          {
            "type": "command",
            "command": "bash \"${CLAUDE_PROJECT_DIR:-.}/.tripcord/bootstrap.sh\" hook --harness claude --event pre-tool --on-failure block",
            "onFailure": "block"
          }
        ]
      },
`
	dupClaudeSessionStart = `,
      {
        "matcher": "startup|resume",
        "hooks": [
          {
            "type": "command",
            "command": "bash \"${CLAUDE_PROJECT_DIR:-.}/.tripcord/bootstrap.sh\" session-start --harness claude"
          }
        ]
      }`
	dupCursorPreTool = `      {
        "command": "bash \"${CURSOR_PROJECT_DIR}/.tripcord/bootstrap.sh\" hook --harness cursor --event pre-tool --on-failure block",
        "failClosed": true
      },
`
	dupCursorSessionStart = `,
      {
        "command": "bash \"${CURSOR_PROJECT_DIR}/.tripcord/bootstrap.sh\" session-start --harness cursor"
      }`
	dupCopilotPreTool = `      {
        "type": "command",
        "bash": "bash .tripcord/bootstrap.sh hook --harness copilot --event pre-tool --on-failure block",
        "cwd": ".",
        "timeoutSec": 60
      },
`
	dupCopilotSessionStart = `,
      {
        "type": "command",
        "bash": "bash .tripcord/bootstrap.sh session-start --harness copilot",
        "cwd": ".",
        "timeoutSec": 300
      }`
)

// The duplicate-entry positives, tracked JSON like the extra-entry ones.
func init() {
	for name, fx := range map[string]cfg016Fixture{
		"embed-claude-duplicate-pre-tool-entry": {kind: "positivos", tracked: true, args: embedArgs("--harness", "claude"),
			edit: replaceInFile(embedded.ClaudeSettingsPath, "    \"PreToolUse\": [\n", "    \"PreToolUse\": [\n"+dupClaudePreTool)},
		"embed-claude-duplicate-session-start-entry": {kind: "positivos", tracked: true, args: embedArgs("--harness", "claude"),
			edit: replaceInFile(embedded.ClaudeSettingsPath, "--harness claude\"\n          }\n        ]\n      }", "--harness claude\"\n          }\n        ]\n      }"+dupClaudeSessionStart)},
		"embed-cursor-duplicate-pre-tool-entry": {kind: "positivos", tracked: true, args: embedArgs("--harness", "cursor"),
			edit: replaceInFile(embedded.CursorHooksPath, "    \"preToolUse\": [\n", "    \"preToolUse\": [\n"+dupCursorPreTool)},
		"embed-cursor-duplicate-session-start-entry": {kind: "positivos", tracked: true, args: embedArgs("--harness", "cursor"),
			edit: replaceInFile(embedded.CursorHooksPath, "session-start --harness cursor\"\n      }", "session-start --harness cursor\"\n      }"+dupCursorSessionStart)},
		"embed-copilot-duplicate-pre-tool-entry": {kind: "positivos", tracked: true, args: embedArgs("--harness", "copilot"),
			edit: replaceInFile(embedded.CopilotHooksPath, "    \"preToolUse\": [\n", "    \"preToolUse\": [\n"+dupCopilotPreTool)},
		"embed-copilot-duplicate-session-start-entry": {kind: "positivos", tracked: true, args: embedArgs("--harness", "copilot"),
			edit: replaceInFile(embedded.CopilotHooksPath, "\"timeoutSec\": 300\n      }", "\"timeoutSec\": 300\n      }"+dupCopilotSessionStart)},
	} {
		if _, dup := cfg016Fixtures[name]; dup {
			panic("cfg016Fixtures: " + name + " twice")
		}
		cfg016Fixtures[name] = fx
	}
}
