# tripcord

tripcord catches malicious coding-agent configuration before it runs:
hooks, MCP servers, skills, and setup commands in Claude Code, Cursor,
GitHub Copilot, Codex, Gemini CLI, and more.

```sh
uvx tripcord scan .
# or
pip install tripcord
tripcord scan .
```

Each wheel bundles the unchanged `tripcord` binary from the matching
[GitHub release](https://github.com/vgmartinez/tripcord/releases)
archive, checked against that release's `SHA256SUMS` when the wheel was
built. The `tripcord` command runs that binary. There is one wheel for
each of Linux (glibc and musl), macOS, and Windows on x86-64 and arm64;
other platforms have no wheel, and pip reports that no matching
distribution was found.

Source, documentation, and issues:
<https://github.com/vgmartinez/tripcord>.

Licensed under the Apache License 2.0.
