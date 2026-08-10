# hitch

Install an MCP server into coding agents on your machine with one command, while handling the
credential carefully.

> **Status: pre-release.** The build is in progress; see [`docs/hitch-build.md`](docs/hitch-build.md)
> for the spec. Install one-liners land in PR6.

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

## Remote Install

```sh
hitch install https://example.com/mcp --token-stdin -c claude-code
```

You may pass the token as the optional second positional argument, but that can be visible to `ps`
and saved in shell history. Prefer `--token-stdin` or `--token-env` for credentials.

## Build From Source

```sh
go build ./...
```

The current pre-release exposes `hitch version`, `hitch list`, and remote HTTP `hitch install` for
the seven JSON-backed clients above. Scan, uninstall, stdio servers, Codex automatic setup, and
packaged install one-liners are planned in later PRs.

## License

MIT
