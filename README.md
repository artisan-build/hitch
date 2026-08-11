# hitch

Install an MCP server into coding agents on your machine with one command, while handling the
credential carefully — and find and remove every copy of it later.

> **Status: pre-release.** v0.0.6 ships `curl | sh` and `go install`; Homebrew and npm publishing are
> wired but waiting on publisher tokens.

## Current Release: v0.0.6

Every MCP client stores server config in a different file, under a different key, with a different
schema for the same server. `hitch` detects the harnesses you actually have, **asks which ones you
want this server in**, and writes the correct config for each.

- **Remote HTTP and stdio servers** into the seven JSON-backed clients listed below.
- **Credentials done properly** — never echoed, never printed, written atomically at `0600`, and a
  config that won't parse is reported rather than clobbered.
- **Revoke what you installed** — `hitch scan` finds every copy of a server across all clients, and
  `hitch uninstall` removes it while leaving the rest of the file byte-for-byte unchanged.
- **Project-scoped installs** with `-p/--project`, which warn before a credential lands in a file
  that isn't gitignored.
- **Interactive and non-interactive** — choose detected harnesses, pass `--yes`, or target explicit
  clients with `--client`. Nothing blocks when there is no terminal.
- **Manual instructions for the clients hitch won't write** with `hitch prompt`.
- **Detection and version commands** with `hitch list` and `hitch version`.

## Supported harnesses

Written by hitch: Claude Code · Cursor · Windsurf · Zed · VS Code · Gemini CLI · opencode

Codex is detected but not configured automatically. When Codex is detected or selected, hitch prints
manual `[mcp_servers.<name>]` instructions using `bearer_token_env_var` and an
`export HITCH_TOKEN_<NAME>=...` command. `hitch scan` verifies user-global `~/.codex/config.toml` and
`hitch uninstall` removes a matching entry without rewriting the rest of the TOML — and where the file
uses a layout hitch cannot splice safely, it reports that it cannot verify the file rather than
editing it.

Claude Desktop and JetBrains are recognized but deliberately not written to. See the spec for why.

## Install hitch

Install from the GitHub Release:

```sh
curl -fsSL https://raw.githubusercontent.com/artisan-build/hitch/main/install.sh | sh
```

Install from source with Go:

```sh
go install github.com/artisan-build/hitch@v0.0.6
```

Coming soon:

- Homebrew: the GoReleaser cask is configured for `artisan-build/homebrew-tap`, but publishing is
  skipped until `HOMEBREW_TAP_GITHUB_TOKEN` is configured.
- npm: the `hitch-mcp` shim package is configured, but publishing is skipped until `NPM_TOKEN` is
  configured.

## Install a remote MCP server

```sh
hitch install https://example.com/mcp --token-stdin -c claude-code
```

You may pass the token as the optional second positional argument, but that can be visible to `ps`
and saved in shell history. Prefer `--token-stdin` or `--token-env` for credentials.

Useful flags: `--name` to override the inferred server name, `--header 'K: V'` for extra HTTP
headers, `--dry-run` to print the planned writes without touching a file, and `--yes` to accept every
detected harness without prompting.

## Install a stdio MCP server

```sh
hitch install stripe --command npx --args "-y,@stripe/mcp" --env STRIPE_API_KEY=sk_...
```

`--args` is a comma-separated argument list; `--env` is repeatable and takes `K=V`.

## Find and remove a server

```sh
hitch scan stripe
hitch uninstall stripe
```

`scan` reports, for every client, whether the server is present, whether that entry holds a
credential, and which configs it could not read. `uninstall` either removes only that server's entry
cleanly, or reports that it cannot verify the file and leaves it unchanged.

## Manual setup for the rest

```sh
hitch prompt https://example.com/mcp
```

Prints ready-to-follow instructions for Claude Desktop, JetBrains, and manual Codex setup. Codex is
manual-install only; `scan` and `uninstall` are supported for user-global Codex config.

## Install into a project instead of your home directory

```sh
hitch install https://example.com/mcp --token-stdin -p -c cursor
```

`--project` writes the client's project-local config at the repository root, not the directory you
happen to be standing in. Because project configs get committed, hitch checks whether the target is
gitignored first: if it isn't, it warns, names the file, and refuses unless you pass `--yes`. Default
scope stays user-global — `--project` is opt-in.

## Not planned

- Automatic Codex configuration. Codex stays manual-install; `hitch prompt` gives you the entry to
  paste, and `scan`/`uninstall` find and remove it afterwards.
- Project-scoped Codex config. No vendor-documented path exists for it, so hitch does not guess one.

## Build From Source

```sh
go build ./...
```

## License

MIT
