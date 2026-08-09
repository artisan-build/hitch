# hitch

Install an MCP server into every coding agent on your machine with one command — and handle the
credential correctly, including taking it back.

> **Status: pre-release.** The build is in progress; see [`docs/hitch-build.md`](docs/hitch-build.md)
> for the spec. Install one-liners land in PR6.

## What it does

Every MCP client stores server config in a different file, under a different key, with a different
schema for the same server. `hitch` detects the harnesses you actually have, **asks which ones you
want this server in**, and writes the correct config for each.

- **Remote HTTP and stdio servers.**
- **Credentials done properly** — never echoed, never printed, written atomically at `0600`, and a
  config that won't parse is reported rather than clobbered.
- **`hitch scan` and `hitch uninstall`** — find every copy of a server's config, and revoke it.

## Supported harnesses

Claude Code · Cursor · Codex · Windsurf · Zed · VS Code · Gemini CLI · opencode

Claude Desktop and JetBrains are recognized but deliberately not written to — see the spec for why.

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

The current pre-release exposes `hitch version`, `hitch list`, and remote HTTP `hitch install`.
Scan, uninstall, stdio servers, and packaged install one-liners are planned in later PRs.

## License

MIT
