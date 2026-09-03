# hitch

Install an MCP server into coding agents on your machine with one command, while handling the
credential carefully — and find and remove every copy of it later.

> **Status: pre-release.** Installable four ways — `curl | sh`, `go install`, Homebrew, and `npx`.

## Current Release: v0.1.1

See [CHANGELOG.md](CHANGELOG.md) for what each release changed.

Every MCP client stores server config in a different file, under a different key, with a different
schema for the same server. `hitch` detects the harnesses you actually have, **asks which ones you
want this server in**, and writes the correct config for each.

- **Remote HTTP and stdio servers** into the seven JSON-backed clients listed below.
- **Credentials done properly** — never echoed, never printed, written atomically at `0600`, and a
  config that won't parse is reported rather than clobbered.
- **Single-use claim codes** — `--claim` exchanges a short-lived code for the token at install time,
  so the durable credential never lands in your shell history or npm's logs, only in configs that
  `scan` and `uninstall` can reach.
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
go install github.com/artisan-build/hitch@v0.1.1
```

Install with Homebrew:

```sh
brew install artisan-build/tap/hitch
```

Or run it without installing anything, via npm:

```sh
npx hitch-mcp https://example.com/mcp --token-stdin -c claude-code
```

The npm package is a shim: it downloads the released binary for your platform, verifies it against
the release checksums, and runs it. Any hitch subcommand works through it — `npx hitch-mcp scan`,
`npx hitch-mcp uninstall <name>` — and arguments that are not a subcommand default to `install`, so
`npx hitch-mcp <url>` installs. Its version always matches the release it fetches, and it is
published from CI via npm trusted publishing, so every version from 0.0.7 onward carries a
provenance attestation you can verify with `npm audit signatures`.

> **`hitch-mcp@0.1.0` mishandles subcommands.** It prefixed `install` onto every invocation, so
> `npx hitch-mcp uninstall example` wrote a server named `uninstall` pointing at `https://uninstall`
> instead of removing anything. Fixed in 0.1.1; if you ran a subcommand through npx on 0.1.0, run
> `hitch scan` and remove any stray entry it reports.

## Install a remote MCP server

```sh
hitch install https://example.com/mcp --token-stdin -c claude-code
```

You may pass the token as the optional second positional argument, but passing the token as an
argument leaves **copies hitch cannot remove** — in your shell history, and (if you used `npx`) in
`~/.npm/_logs/`. `hitch scan` and `hitch uninstall` find and remove the copies in your harness
configs; they cannot reach those two. Prefer `--claim`, or `--token-stdin`, or `--token-env`, or the
masked prompt.

Useful flags: `--name` to override the inferred server name, `--header 'K: V'` for extra HTTP
headers, `--dry-run` to print the planned writes without touching a file, and `--yes` to accept every
detected harness without prompting.

### Install with a single-use claim code

If the server operator handed you a one-liner carrying a claim code instead of a token:

```sh
hitch install https://app.example.com/mcp --claim A1B2-C3D4 --claim-url https://app.example.com/mcp/claim
```

hitch exchanges the code for the token over HTTPS first, and only then writes any config — a failed
exchange leaves every file byte-for-byte untouched. What lands in your shell history and npm's logs
is the **code**, not the token, and a claim code is worthless once its token has been used — so those
copies go inert almost as soon as they exist, while every copy of the actual credential stays where
`scan` and `uninstall` can reach it.

The claim response may suggest a server name; `--name` always wins over it. The response can never
change the URL that gets installed — hitch installs exactly the URL you passed. `--dry-run --claim`
makes no claim request at all: it previews the writes and leaves the code unspent. `--claim-url` is
always explicit; hitch never guesses a claim endpoint from the server URL. Server authors implement
the exchange in a single route — see [docs/claim-contract.md](docs/claim-contract.md).

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
