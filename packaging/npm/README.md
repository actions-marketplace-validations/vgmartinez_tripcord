# tripcord

tripcord catches malicious coding-agent configuration before it runs:
hooks, MCP servers, skills, and setup commands in Claude Code, Cursor,
GitHub Copilot, Codex, Gemini CLI, and more.

```sh
npx tripcord scan .
```

This package has no dependencies and no install script. On first run,
`tripcord` downloads the release archive for your platform from
`https://github.com/vgmartinez/tripcord/releases/download/v<version>/`
(GitHub redirects the download to `release-assets.githubusercontent.com`),
checks its sha256 against the release's `SHA256SUMS`, which ships inside
this package, and refuses to run anything on a mismatch. The verified
binary is cached and reused; later runs make no network request. There
is no telemetry.

Supported: Linux, macOS, and Windows on x64 and arm64. Any other
platform gets an error naming it.

The cache is `$TRIPCORD_CACHE_DIR` if set, otherwise
`$XDG_CACHE_HOME/tripcord` or `~/.cache/tripcord` on Linux,
`~/Library/Caches/tripcord` on macOS, and `%LOCALAPPDATA%\tripcord\cache`
on Windows.

Source, documentation, and issues:
<https://github.com/vgmartinez/tripcord>.

Licensed under the Apache License 2.0.
