# Workflow — hitch

Project profile for the `multi-agent-build` skill. The coordinator reads this FIRST, then
`docs/hitch-build.md` (the authoritative build spec).

hitch is a **Go** CLI (single distributable binary, `hitch`) that installs an MCP server into every
detected coding agent on a machine, handles the credential correctly, and can revoke it. Public
OSS under `artisan-build`. This is a Go repo, not PHP.

The behavioral reference is `~/Herd/ballast-cli/src/ballast/mcp_install.py` plus its two test files
— a working production implementation of the same adapter matrix. Read it before PR2. It is a
**private** repo: transfer the behavior and the test cases, never Ballast-specific code, naming,
token-store, affordances, or API-base logic.

## Phase & mode
- phase: pre-launch (greenfield)
- default mode: A-autonomous
- merge_policy: merge when CI is green; no human PR code review
- merge method: `gh pr merge --squash --auto` (fall back to watch-CI-then-direct-merge if auto is off)

## Hard gate (coordinator verifies on the committed SHA, clean tree)
- command: `gofmt -l . | (! grep .) && go vet ./... && golangci-lint run && go test -count=1 ./...`
- **`-count=1` is mandatory, not optional.** Without it Go serves cached results, and a `(cached)`
  line is a pass nobody observed being produced — the same shape as a mutation that never applied or
  a check that exited before running. **A verification step that did not execute must never be able
  to render as a pass.** This applies to every harness in this project, the coordinator's own
  included.
- monorepo: no.

## CI (the merge gate for Mode A)
- status: PR1 MUST add `.github/workflows/ci.yml` — testing (`go test ./...`) + static analysis
  (`go vet` + golangci-lint), on push + PR to main. Do NOT CI-gate merges (Mode A) until this
  exists and is green.
- minimum bar met by: gofmt check, `go vet`, golangci-lint (errcheck/govet/ineffassign/staticcheck/unused),
  `go test ./...`.

## Dependency install (fresh worktree)
- command: `go mod download`
- post-install: none. Requires Go 1.23+ and golangci-lint on PATH.

## Harness map (role → runtime; decorrelate by ROLE/FRAMING, not model lineage)
- Only **Claude (agent_tool_id 3)** and **OpenCode (agent_tool_id 2)** run reliably in this Solo env.
- implementer: OpenCode (`agent_tool_id 2`) — persistent agent in the PR worktree (`extra_args=["<path>"]`).
- quality reviewer: Claude (`agent_tool_id 3`), one-shot, ADVERSARIAL ("find what's wrong; default reject").
- acceptance judge: Claude (`agent_tool_id 3`) — judges against each PR's acceptance criteria in the
  spec, reading REAL gate/test output, not the implementer's claims.

## Toolchain conformance — the ride-along rule (STANDING, all projects)
Go equivalent of `composer ready`: run `gofmt -w .` and `go mod tidy` when finalizing EVERY PR, and
let whatever they change ride along in that PR as a single isolated commit titled `go tidy`. Keep it
in its own commit so a reviewer can separate the feature from the sweep. Changing `.golangci.yml`
itself gets its own dedicated PR.

## Ship details
- branch naming: `feat/<slug>`
- PR target repo: `artisan-build/hitch` (branch `main`)
- release / distribution: GoReleaser → GitHub Releases, Homebrew tap, `install.sh`, npm shim
  (`hitch-mcp`). Built in PR6.
- **DO NOT TAG A RELEASE.** Ed tags every version himself on explicit per-version approval. PR6
  lands the release machinery only; it must not push a `v*` tag.

## Plan & coordination
- plan location: `docs/hitch-build.md` (authoritative PRD; 6 PRs).
- Solo project: hitch.
- run-log: append transitions to a Solo scratchpad named `hitch run-log`.

## Tag-the-human conditions
- A required decision, a broken plan assumption, a bail, or 3 failed attempts on one PR.
- Any temptation to widen scope past §9 of the spec (OAuth, registry, well-known discovery).
- Any per-client config path or entry shape that current client docs contradict — report the
  conflict rather than picking one.

## Stack notes / quirks
- **The interactive picker is a core requirement, not a flag** (spec §3). Users do not necessarily
  want every MCP in every harness. Detection feeds a choice; it is not the decision.
- **A non-TTY run must never block on a prompt.** Agents will drive this CLI. With no TTY and
  neither `-y` nor `--client`, exit non-zero naming both flags. A hang here is a bug.
- **No code path may print a token** — including errors and `--dry-run`. Tests must assert this.
- Writes are atomic + 0600; a config that will not parse is REPORTED, never rewritten.
- Claude Code detection must key on `~/.claude` (or `$CLAUDE_CONFIG_DIR`), not on `~/.claude.json`'s
  parent — that parent is `$HOME` and always exists.
- Every adapter needs its own table-driven test against a temp HOME. Eight clients × (fresh /
  existing-preserved / idempotent-update / malformed-refused) is the minimum bar.
