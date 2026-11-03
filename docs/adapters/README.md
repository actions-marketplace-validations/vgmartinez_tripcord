# Harness adapters

An adapter knows where one coding agent (a harness) keeps its
configuration, reads those files without running anything, and
normalizes hooks, MCP servers, skills, permissions, tasks, and setup
commands into the items rules match.

`tripcord scan` autodetects these adapters. `--harness` takes a
comma-separated subset of the same ids. A product that also loads files
another adapter owns pulls that adapter in; an informational note on
stderr says so. For example, `--harness cursor` also runs `claude` and
`generic`.

| Id | Adapter | Page |
| --- | --- | --- |
| `claude` | Claude Code | [Claude Code](claude.md) |
| `codex` | Codex | [Codex](codex.md) |
| `cursor` | Cursor | [Cursor](cursor.md) |
| `copilot` | GitHub Copilot | [GitHub Copilot](copilot.md) |
| `gemini` | Gemini CLI | [Gemini CLI](gemini.md) |
| `opencode` | OpenCode | [OpenCode](opencode.md) |
| `windsurf` | Windsurf | [Windsurf](windsurf.md) |
| `devin` | Devin blueprint | [Devin](devin.md) |
| `devcontainer` | Dev Container configuration | [Dev Container](devcontainer.md) |
| `generic` | Shared skills, `AGENTS.md`, and `.vscode/` | [Shared files](generic.md) |

`.vscode/tasks.json` findings use harness `vscode`. That id is not a
`--harness` value (`--harness vscode` exits 3). A harness-limited scan
still reads `.vscode/tasks.json`, because the editor runs its
folder-open tasks whichever agent is in use.

Each adapter page covers its locations per scope (`--scope repo`, the
default, then `user` and `managed`), the normalized payloads rules see,
parsing limits, and what is not scanned in v0. The
[shared-files adapter](generic.md#ownership) explains which adapter owns
a file several harnesses read.

`init --embed` wires a subset of these harnesses (`claude`, `cursor`,
`copilot`, `devin`, `codex`); see [init](../init.md#with---embed).
