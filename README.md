# hitch

Install an MCP server into coding agents on your machine with one command, while handling the
credential carefully.

> **Status: pre-release.** v0.0.1 is the first installable release path. It supports `curl | sh`
> and `go install`; Homebrew and npm publishing are wired but waiting on publisher tokens.

## What v0.0.1 Does

Every MCP client stores server config in a different file, under a different key, with a different
schema for the same server. `hitch` detects the harnesses you actually have, **asks which ones you
want this server in**, and writes the correct config for each.

- **Remote HTTP servers.**
- **Credentials done properly** — never echoed, never printed, written atomically at `0600`, and a
  config that won't parse is reported rather than clobbered.
- **Interactive and non-interactive installs** — choose detected harnesses, pass `--yes`, or target
  explicit clients with `--client`.

Important: v0.0.1 can install a credential into selected config files, but it does not have
`hitch uninstall` or `hitch scan` yet. Keep track of what you install until those commands land.

## Supported harnesses

Writable in v0.0.1: Claude Code · Cursor · Windsurf · Zed · VS Code · Gemini CLI · opencode

Codex is detected, but hitch cannot configure Codex automatically yet. When Codex is detected or
selected explicitly, hitch prints manual `[mcp_servers.<name>]` instructions using
`bearer_token_env_var` and an `export HITCH_TOKEN_<NAME>=...` command.

Claude Desktop and JetBrains are recognized but deliberately not written to. See the spec for why.

## Install hitch

Install from the GitHub Release:

```sh
curl -fsSL https://raw.githubusercontent.com/artisan-build/hitch/main/install.sh | sh
```

Install from source with Go:

```sh
go install github.com/artisan-build/hitch@v0.0.1
```

Coming soon:

- Homebrew: the GoReleaser cask is configured for `artisan-build/homebrew-tap`, but publishing is
  skipped until `HOMEBREW_TAP_GITHUB_TOKEN` is configured.
- npm: the `hitch-mcp` shim package is configured, but publishing is skipped until `NPM_TOKEN` is
  configured.

## Remote MCP Install

```sh
hitch install https://example.com/mcp --token-stdin -c claude-code
```

You may pass the token as the optional second positional argument, but that can be visible to `ps`
and saved in shell history. Prefer `--token-stdin` or `--token-env` for credentials.

## Build From Source

```sh
go build ./...
```

v0.0.1 exposes `hitch version`, `hitch list`, and remote HTTP `hitch install` for the seven
JSON-backed clients above. Scan, uninstall, stdio servers, and Codex automatic setup are planned in
later PRs.

## License

MIT
