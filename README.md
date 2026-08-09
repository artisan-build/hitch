# hitch

Install an MCP server into every coding agent on your machine with one command — and handle the
credential correctly, including taking it back.

```sh
hitch install https://example.com/mcp $TOKEN
```

> **Status: pre-release.** The build is in progress; see [`docs/hitch-build.md`](docs/hitch-build.md)
> for the spec. Nothing is installable yet.

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

## License

MIT
