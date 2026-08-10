# hitch — build spec

**hitch** installs an MCP server into every coding agent on your machine with one command, and
handles the credential correctly — including taking it back.

```
hitch install https://ballast.now/mcp $TOKEN
hitch install stripe --command npx --args "-y,@stripe/mcp"
hitch scan
hitch uninstall ballast
```

This document is the authoritative PRD. The coordinator reads `.solo/workflow.md` first, then this.

---

## 1. Why this exists

Every MCP client stores its server config in a different file, under a different key, with a
different schema for the same remote server. A user who wants one MCP in four harnesses today
hunts through four sets of docs and hand-edits four files.

`npx add-mcp <url>` (Neon) already solves the *URL* half across ~9 clients. It does not document
auth. **The token is the hard half**, and it is the half that carries risk:

- It must never be echoed, never land in shell history or `ps` output when avoidable.
- It gets written into up to a dozen files, so there must be a way to find every copy and revoke it.
- Some configs get committed to git, so writing a bearer token into a project-local file is a
  footgun that needs guarding, not a default.

**hitch's differentiators, in priority order:**

1. Credentials handled correctly end-to-end (write, locate, revoke).
2. Interactive per-harness confirmation — the user chooses where each server lands.
3. `scan` / `uninstall` — the inverse operation, which no comparable tool offers.
4. Both remote HTTP and stdio servers.

### Reference implementation

`~/Herd/ballast-cli/src/ballast/mcp_install.py` (748 lines) and its tests
(`tests/test_mcp_install.py`, `tests/test_mcp_commands.py`, 1,236 lines) are a **working,
production implementation of the adapter matrix and the security posture**. Ballast is a private
repo; this is a clean OSS reimplementation in Go, but the adapter table, the per-client schemas,
the detection markers, and the test cases are all directly transferable and should be treated as
the specification. **Read that module before writing PR2.** Do not copy Ballast-specific naming,
token-store, affordance, or API-base code — none of it belongs here.

---

## 2. Command surface

```
hitch install <url> [token]              # remote HTTP; name inferred from host
hitch install <name> --url <url>         # explicit name
hitch install <name> --command <cmd> --args "a,b,c" [--env K=V ...]   # stdio
hitch uninstall <name>
hitch scan [<name>]                      # where is this server configured, and what holds a credential
hitch list                               # which harnesses are installed on this machine
hitch prompt <url>                       # copy-paste setup text for clients we cannot write
hitch version
```

### Global flags

| Flag | Meaning |
|---|---|
| `-c, --client <name>` | Target explicit harnesses (repeatable). Skips the interactive picker. |
| `-y, --yes` | Non-interactive: accept every detected harness. Skips the picker. |
| `-p, --project` | Write to project-scoped config in the cwd instead of user-global. |
| `--dry-run` | Print exactly which files would change and how; write nothing. |
| `--token-stdin` | Read the token from stdin instead of argv. |
| `--token-env <VAR>` | Read the token from an environment variable. |
| `--header "K: V"` | Additional header (repeatable). For non-bearer auth schemes. |
| `--name <name>` | Override the inferred server name. |

### Name inference

From the URL host: strip a leading `mcp.`, take the first remaining label.
`https://ballast.now/mcp` → `ballast`; `https://mcp.context7.com/mcp` → `context7`;
`https://api.githubcopilot.com/mcp/` → `api` → so **when inference is ambiguous or yields a
generic label (`api`, `www`, `app`, `server`), prompt for confirmation** rather than guessing.
`--name` always wins. In `-y` mode an ambiguous inference is an error, not a guess.

### URL and credential validation

Remote install has a single pre-write validation gate. It runs after token/header resolution and before
any config file is read for writing:

- Scheme-less URLs normalize to `https://` instead of being rejected. `hitch install ballast.now/mcp
  TOKEN` writes `https://ballast.now/mcp`; any user-facing success, confirmation, or dry-run text must
  show the normalized URL so the user sees exactly what was or will be written.
- URL validation is local only: absolute `http` or `https` URL, non-empty host, no network probing or
  `/.well-known` discovery.
- If any credential/header is present, public `http://` is refused because it would transmit the
  credential in cleartext. `http://localhost`, `http://127.0.0.1`, and `http://[::1]` are allowed for
  local development. Refusal is the default; an insecure escape hatch may only be additive.
- Explicit credential sources are required to resolve to a non-empty value. Empty positional token,
  empty `--token-stdin`, and unset or empty `--token-env` all fail before any file is touched.
- An explicit `Authorization` header and bearer token input are mutually exclusive. Hitch must not
  silently overwrite one credential with another.

---

## 3. The interactive model (core UX, not a nicety)

**Do not assume the user wants every MCP in every harness.** Detection is the input to a choice,
not the decision itself.

Default `install` flow:

```
$ hitch install https://ballast.now/mcp

  Found 5 harnesses on this machine. Install "ballast" into which?

  [x] Claude Code      ~/.claude.json
  [x] Cursor           ~/.cursor/mcp.json
  [ ] Codex            ~/.codex/config.toml
  [x] Zed              ~/.config/zed/settings.json
  [ ] VS Code          ~/Library/Application Support/Code/User/mcp.json

  ↑/↓ move · space toggle · a all · enter confirm · esc cancel
```

Rules:

- **Pre-selection comes from remembered preference** (§7), defaulting to all-selected on first run.
- `--client` and `--yes` both bypass the picker entirely.
- **Non-TTY (piped, CI, agent-driven) must never hang.** With no TTY and neither `-y` nor
  `--client`, exit non-zero with a message naming both flags. This tool will be run by agents;
  a blocking prompt in a non-interactive context is a hang, and a hang is a bug.
- `uninstall` uses the same picker, but the list is **where the server is actually configured**,
  and each row states whether that file holds a credential:

```
  Remove "ballast" from which?

  [x] Claude Code      ~/.claude.json                  (holds a bearer token)
  [x] Cursor           ~/.cursor/mcp.json              (holds a bearer token)
  [x] Codex            ~/.codex/config.toml            (env var reference only)
  [!] Windsurf         ~/.codeium/windsurf/mcp_config.json   (unreadable — cannot verify)
```

- An **unreadable config is its own outcome**, never silently skipped and never rewritten. It may
  still hold a live credential; saying nothing turns a partial uninstall into an apparently clean
  one. Surface it in the picker as unselectable and repeat it in the summary.

Use `github.com/charmbracelet/huh` for the picker. It degrades to an accessible prompt when the
terminal can't do full TUI.

---

## 4. Harness matrix

Ported from the Ballast adapter table — each client stores the *same* remote server differently,
and getting any one of these wrong silently produces a config the client ignores.

### Remote HTTP (user-global paths)

| Client | Config path | Key | Entry shape |
|---|---|---|---|
| Claude Code | `~/.claude.json` (or `$CLAUDE_CONFIG_DIR/.claude.json`) | `mcpServers` | `{type: "http", url, headers}` |
| Cursor | `~/.cursor/mcp.json` | `mcpServers` | `{url, headers}` |
| Windsurf | `~/.codeium/windsurf/mcp_config.json` | `mcpServers` | `{serverUrl, headers}` |
| Zed | platform-specific `zed/settings.json` | `context_servers` | `{url, headers}` — **JSONC** |
| VS Code | platform-specific `Code/User/mcp.json` | `servers` | `{type: "http", url, headers}` |
| Gemini CLI | `~/.gemini/settings.json` | `mcpServers` | `{httpUrl, headers}` |
| opencode | `${OPENCODE_CONFIG_DIR:-${XDG_CONFIG_HOME:-~/.config}/opencode}/opencode.json` | `mcp` | `{type: "remote", url, headers}` |

VS Code path: macOS `~/Library/Application Support/Code/User/mcp.json`; Windows
`%APPDATA%/Code/User/mcp.json`; Linux `${XDG_CONFIG_HOME:-~/.config}/Code/User/mcp.json`.
Before those platform defaults, VS Code honours `VSCODE_PORTABLE` as
`$VSCODE_PORTABLE/user-data/User/mcp.json`, then `VSCODE_APPDATA` as
`$VSCODE_APPDATA/Code/User/mcp.json`. Source: `microsoft/vscode`
`src/vs/platform/environment/node/userDataPath.ts doGetUserDataPath()`.

Codex path: defaults to `~/.codex/config.toml`, but `CODEX_HOME` overrides the state directory, so
the config becomes `$CODEX_HOME/config.toml`. Source: `openai/codex`
`codex-rs/core/src/config/mod.rs`; the published config docs omit this override.

Zed path: macOS `~/.config/zed/settings.json` and deliberately ignores `XDG_CONFIG_HOME`; Windows
`%APPDATA%/Zed/settings.json` with capital `Zed`; Linux/FreeBSD
`${XDG_CONFIG_HOME:-~/.config}/zed/settings.json`. Source: `zed-industries/zed`
`crates/paths/src/paths.rs config_dir()`. `FLATPAK_XDG_CONFIG_HOME` is deliberately unsupported in
hitch because Zed uses it verbatim with no `zed` suffix, and a partial implementation would be
misleading.

opencode path: defaults through `xdg-basedir` as
`${XDG_CONFIG_HOME:-~/.config}/opencode/opencode.json`, including on macOS, but
`OPENCODE_CONFIG_DIR` takes precedence and points directly at the directory containing
`opencode.json`. Source: `sst/opencode` `packages/core/src/global.ts`.

Gemini CLI path: `~/.gemini/settings.json`; it does not honour XDG for this path, and no production
config-dir override was found. `GEMINI_CONFIG_DIR` appears in its test harness, not in the resolver.
Source: `google-gemini/gemini-cli` `packages/core/src/config/storage.ts getGlobalSettingsPath()`.

Path evidence tiers:

| Client | Tier | Citation | Limitation |
|---|---|---|---|
| Claude Code | SOURCE-VERIFIED | already verified in PR1; `CLAUDE_CONFIG_DIR` handled | Resolver source checked. |
| Codex | SOURCE-VERIFIED | `openai/codex` `codex-rs/core/src/config/mod.rs` | Resolver source checked; docs omit `CODEX_HOME`. |
| Zed | SOURCE-VERIFIED | `zed-industries/zed` `crates/paths/src/paths.rs` | Resolver source checked; docs do not state the macOS/XDG distinction. |
| VS Code | SOURCE-VERIFIED | `microsoft/vscode` `src/vs/platform/environment/node/userDataPath.ts` | Resolver source checked, except `--user-data-dir` is CLI runtime state and not applicable to hitch. |
| Gemini CLI | SOURCE-VERIFIED | `google-gemini/gemini-cli` `packages/core/src/config/storage.ts` | Resolver source checked; no production override found. |
| opencode | SOURCE-VERIFIED | `sst/opencode` `packages/core/src/global.ts` | Resolver source checked. |
| Cursor | VENDOR-DOCUMENTED | `cursor.com/docs/context/mcp` | Closed source: the documented path can change silently without a resolver we can check, so this is not at parity with SOURCE-VERIFIED. |
| Windsurf | VENDOR-DOCUMENTED | `docs.devin.ai/desktop/cascade/mcp` | Closed source: the documented path can change silently without a resolver we can check, so this is not at parity with SOURCE-VERIFIED. |

INHERITED-UNVERIFIED is empty for the PR2 remote HTTP adapter matrix.

### Prompt-tier (recognized, deliberately not written)

| Client | Why |
|---|---|
| Codex | The config is TOML. PR2 ships the seven JSON writers and reports manual Codex setup instead of risking a lossy TOML rewrite. |
| Claude Desktop | MCP config is stdio-only; remote HTTP needs the `mcp-remote` proxy and a Node runtime. Writing a proxy entry would silently depend on local tooling. |
| JetBrains | The MCP dialog has no Authorization-headers field. |

These return honest instructions via `hitch prompt`, not a broken config.

For PR2, Codex is also reported from `hitch install` when detected or explicitly selected. The exact
wording must include `hitch cannot configure Codex automatically yet` plus manual TOML instructions
using `[mcp_servers.<name>]`, `bearer_token_env_var = "HITCH_TOKEN_<NAME>"`, and an
`export HITCH_TOKEN_<NAME>=...` command. Explicit `--client codex` exits non-zero after printing
those instructions; auto-detected Codex does not fail otherwise-successful JSON writes.

Failed TOML approaches in PR2, deliberately not kept:

- Splicing between byte offsets deleted everything from hitch's table to EOF when the next header was
  an array-of-tables.
- Whole-document re-serialization destroyed all comments and formatting. It also silently shifted
  TOML local date/time values by the host's UTC offset: `1979-05-27T07:32:00` became `05:32:00`, and
  `local_date` lost a whole day. That failure is invisible under UTC, so CI could never catch it.
- The line-scan span finder matched header text inside multi-line strings, deleting the closing
  delimiter, and failed to match equivalent-but-differently-spelled table names like
  `[mcp_servers.x]` versus `[mcp_servers."x"]`, producing files that no longer parse.

### Detection

Presence-only — a config file exists, **or** the harness data directory exists. Never read file
contents to detect. **Claude Code needs a special case:** `~/.claude.json`'s parent is `$HOME`,
which always exists, so detect on `~/.claude` (or `$CLAUDE_CONFIG_DIR`) instead.

### stdio entry shapes (PR3)

Same clients, different shape — `{command, args, env}` under the same config key, with per-client
deviations. Verify each against that client's current docs while implementing; do not assume the
remote shape's key names carry over.

---

## 5. Credential handling — the non-negotiables

1. **Never print the token.** Not in success output, not in errors, not in `--dry-run`. `--dry-run`
   shows `"Authorization": "Bearer ***"`.
2. **Never echo on interactive entry.** When a remote install has no token from argv/stdin/env and
   a TTY is present, prompt with masked input.
3. **Prefer stdin/env over argv.** Positional token stays supported (it's the ergonomic headline)
   but the docs must state that argv is visible to `ps` and lands in shell history, and recommend
   `--token-stdin`. Do not remove the positional form — the one-liner is the product.
4. **Atomic 0600 writes.** Write to a temp file in the target directory opened `O_EXCL` with mode
   0600, then `os.Rename`. Chmod 0600 after. Resolve symlinks before writing so we replace the
   target, not the link.
5. **Never rewrite a config we cannot parse.** Malformed JSON/TOML is a clean error naming the
   file. Clobbering a user's real client config is far worse than refusing. Zed's JSONC comments
   are the common cause — say so in the error.
6. **Codex never persists the token.** It gets `bearer_token_env_var = "HITCH_TOKEN_<NAME>"` and
   the success message tells the user to export it. Its config is the most likely to be committed.
7. **Project scope + token = warn.** In `--project` mode with a credential, check whether the
   target file is gitignored. If it is not, warn loudly and require confirmation (or `-y`).
8. **One harness failing does not abort the others** — but the summary must state exactly which
   files were written, because a written file now holds a credential.

---

## 6. Scope: user-global vs project

**Default is user-global.** This is a deliberate inversion of `add-mcp`, which defaults to project
scope: project configs get committed, and hitch's whole premise is that it writes credentials.
`-p/--project` opts in, with the gitignore guard from §5.7.

Project-scoped paths (PR4): `.mcp.json` (Claude Code), `.cursor/mcp.json`, `.vscode/mcp.json`,
`.zed/settings.json`, `.gemini/settings.json`, `opencode.json`, `.codex/config.toml`. Confirm each
against current client docs while implementing.

---

## 7. Remembered preference

After a successful interactive run, persist the chosen harness set to
`~/.config/hitch/preferences.json` (0600, `XDG_CONFIG_HOME`-aware). Next run pre-checks that set
instead of everything. This is what makes the picker cheap for someone who always wants the same
two harnesses, and it is why the picker doesn't become friction for Ed, who wants all of them.

Store the selection only — never a token, never a URL. `hitch install --forget` resets it.

---

## 8. PR decomposition

Each PR is independently shippable and leaves `main` green.

**PR1 — skeleton + gate.** `go.mod` (module `github.com/artisan-build/hitch`), cobra root command,
`hitch version`, `hitch list` (detection only, no writes), `.golangci.yml`, `.github/workflows/ci.yml`
(gofmt + `go vet` + golangci-lint + `go test ./...`), README stub. Establishes the gate everything
else is judged against.

**PR2 — remote HTTP install (the core).** Adapter registry for the 7 JSON file-writer clients, name
inference, token resolution (argv / `--token-stdin` / `--token-env` / masked prompt), the
interactive picker, `-y`/`--client`/`--dry-run`, atomic 0600 writes, refuse-to-clobber, honest
multi-harness summary. **Table-driven tests per client against a temp HOME**, covering: fresh file,
existing file with unrelated keys preserved, existing entry updated idempotently, malformed file
refused, non-TTY without `-y` exits non-zero, token never appears in any output.

Codex is intentionally split out of PR2's writable adapter set. Keep Codex detection and `CODEX_HOME`
path handling, but surface it as manual/not-yet-implemented until a safe TOML editing strategy exists.

**PR3 — stdio servers.** `--command` / `--args` / `--env` across the same matrix, with per-client
stdio shapes verified against current docs. Same test depth.

**PR4 — project scope.** `-p/--project`, project config paths, gitignore guard for credentials.

**PR5 — scan + uninstall + prompt-tier.** `hitch scan` (three outcomes per config: has entry, no
entry, unreadable), `hitch uninstall` with the where-it-actually-is picker and credential labels,
`hitch prompt` for Claude Desktop / JetBrains. Removal preserves every other key and every other
server. Tests must cover the unreadable-config path explicitly.

**PR6 — distribution.** GoReleaser (darwin/linux × amd64/arm64, `CGO_ENABLED=0`), `release.yml`
on `v*` tags, `install.sh` curl one-liner, Homebrew formula for `artisan-build/homebrew-tap`, and
an `hitch-mcp` npm shim package that downloads the matching binary so `npx hitch-mcp <url> <token>`
works. **v0.0.1 is authorized per-version, and Ed does the tagging** — not the implementer and not
the coordinator. PR6 lands the machinery only. npm publishing must remain guarded behind both
`NPM_TOKEN` and `NPM_PUBLISH_ENABLED=true` until `npm/hitch-mcp/bin/hitch-mcp.js` has an automated
test; the redirect-handling bug shipped undetected because nothing ever ran the downloader.

---

## 9. Out of scope for v1 (do not build)

- OAuth / dynamic client registration flows. Bearer tokens and custom headers only.
- A server registry or catalog. hitch installs what you point it at.
- `/.well-known/mcp.json` endpoint discovery. This is the intended v2 wedge — design the URL
  handling so it can slot in later, but do not implement it.
- Windows support beyond correct path resolution. Build it right, don't test-matrix it.
- Any hosted component.

---

## 10. Definition of done

- `hitch install <url> <token>` configures every selected detected harness correctly, and each
  client actually loads the server afterward (verify at least Claude Code and Cursor by hand).
- `hitch uninstall <name>` removes every copy and reports anything it could not verify.
- No path prints or logs a token. Grep the test output to prove it.
- CI green: gofmt, `go vet`, golangci-lint, `go test ./...`.
- README shows the working install paths honestly. v0.0.1 supports curl and `go install`; brew and
  npx remain guarded until their publisher secrets exist.
