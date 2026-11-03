# Roadmap

Nothing here is released. There is no tagged release yet; `main` is the
pre-release v0 tree described in [Release notes](release-notes.md).

## In this tree

`scan`, `init` (including `--embed`), `preopen`, `pr-context`, the
[GitHub Action](github-action.md) and its reusable workflows, and the
[pre-commit hook](pre-commit.md).

## Coming in v0

- Hook mode: `tripcord hook`, which the harness hooks `init --embed`
  writes call before each tool use. Until it lands `init --embed` is not
  releasable ([init](init.md#release-gate)), and exit code 2 (hook-mode
  block) is defined but unused.
- The `ask` decision mapped to each harness's confirmation prompt in hook
  mode ([Development](development.md#decisions)).

## Later

- Secrets in transcripts. `--secrets` and `--all` (v1) already parse and
  refuse to scan.
- Live watch.
- Local forensics and blast radius.
- Import from cloud agents.
- Monorepo scoping
  ([#59](https://github.com/vgmartinez/tripcord/issues/59)).
- TC-CFG-013 through zizmor's `agentic-actions` audit, once it is
  released ([#41](https://github.com/vgmartinez/tripcord/issues/41)).
