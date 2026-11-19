# TC-CFG-010: Repository ships an agent plugin, which runs arbitrary code

| | |
| --- | --- |
| Severity | medium; low under a fixture directory, and for a Claude Code plugin manifest or marketplace also under a test directory |
| Harnesses | OpenCode, Claude Code |
| Files | `opencode.json`, `opencode.jsonc`, `.opencode/plugin/*`, `.opencode/plugins/*`, `.opencode/tool/*`, `.opencode/tools/*`, `.claude-plugin/plugin.json`, `.claude-plugin/marketplace.json`, `.claude/settings.json` (any depth, repository scope) |
| Rule | [`rules/TC-CFG-010.yaml`](../../rules/TC-CFG-010.yaml) |

## What it detects

A plugin is code the agent loads and runs with the user's privileges.
An OpenCode plugin is a JavaScript or TypeScript module imported when
OpenCode starts. A Claude Code plugin can bring hooks, MCP servers,
agents, and commands. A repository that ships or enables one asks every
contributor's agent to run it.

Reported, once per plugin entry:

- OpenCode: each entry of `plugin` in `opencode.json` or
  `opencode.jsonc` (an npm package OpenCode installs and loads), and
  each JS/TS file directly inside `.opencode/plugin/`,
  `.opencode/plugins/`, `.opencode/tool/`, or `.opencode/tools/`.
- Claude Code: a `.claude-plugin/plugin.json` manifest, each plugin
  listed in a `.claude-plugin/marketplace.json`, and in the shared
  `.claude/settings.json` an `enabledPlugins` object with at least one
  plugin set to `true`, or a non-empty `extraKnownMarketplaces`.

Not reported:

- `.claude/settings.local.json`, which is personal.
- `enabledPlugins` entries that are all `false`, and an empty
  `extraKnownMarketplaces`.
- OpenCode files nested below a plugin or tool directory, which
  OpenCode does not load.
- User-scope plugins.

## Why medium

Scan mode reads the repository as it is. It sees that a plugin is
there, not whether a change just added it, and not what the plugin's
code does. Shipping a reviewed plugin with a project is a legitimate
use, so a plugin's presence alone does not fail a scan at the default
fail-on (high); it is listed for review. Raising an addition in a pull
request is the job of a pull-request context, which is not implemented
yet.

## Low: test and fixture directories

Two cases are test input that no harness loads from the checkout. They
are still reported, at low, so a plugin placed there stays visible; they
are not dropped.

- A Claude Code `.claude-plugin/plugin.json` or
  `.claude-plugin/marketplace.json` under a directory named `fixtures`,
  `__fixtures__`, `testdata`, `test-fixtures`, `test_fixtures`, `tests`,
  or `test`, at any depth. A test suite reads it or installs it into a
  temporary home. For example
  `test/e2e/fixtures/market/.claude-plugin/marketplace.json` and
  `tests/modules/sample-plugin/.claude-plugin/plugin.json` are low.
- Any other match, for every harness the rule reads, under a
  fixture-type directory only: `fixtures`, `testdata`, or
  `__fixtures__`, at any depth (`tests/fixtures/` is covered through
  `fixtures/`). That covers an OpenCode `opencode.json(c)` plugin entry,
  a module under `.opencode/plugin(s)/` or `.opencode/tool(s)/`, and a
  Claude Code `.claude/settings.json` entry. For example
  `tests/fixtures/plugin-case/opencode.json` and its
  `.opencode/plugins/local.ts` are low.

A config under plain `tests/` or `test/` that is not a Claude Code
plugin manifest or marketplace stays medium: it can be one the agent
really loads when started in that directory, such as a test
environment's `tests/dev-env/opencode.json`. A directory whose name only
contains one of these words (`contests/`, `latest/`) stays medium, and so
does the repository-root config.

## Examples

Reported:

```json
{"plugin": ["opencode-acme-notify@1.2.0"]}
```

```json
{"enabledPlugins": {"formatter@acme-tools": true}}
```

Not reported:

```json
{"enabledPlugins": {"formatter@acme-tools": false}}
```

## How to fix

Review the plugin's code and what it registers (hooks, MCP servers,
commands) before you keep it. Prefer a plugin pinned to a reviewed
version or commit, and remove plugins the project does not need. To
accept a reviewed plugin, add a policy exception for this rule and
file.

## References

- [OpenCode plugins](https://opencode.ai/docs/plugins/)
- [OpenCode config](https://opencode.ai/docs/config/)
- [Claude Code plugins](https://code.claude.com/docs/en/plugins)
- [Claude Code plugin marketplaces](https://code.claude.com/docs/en/plugin-marketplaces)
- [Claude Code settings](https://code.claude.com/docs/en/settings)
