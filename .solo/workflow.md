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

### Proving a test is wired to CI — it takes a PR, not a push
`ci.yml` triggers on **`push: [main]`** and **`pull_request: [main]`** only. **Pushing a branch raises
no run at all.** So the obvious way to check the wiring — push the branch, look for a run — shows
nothing and proves nothing; do not read that silence as a broken workflow.

**To prove a test can fail CI:** branch from the PR head, apply a mutation, push, and **open a draft
PR to `main`**. Watch it go red, then close the PR and delete the branch. A YAML reference is exactly
the kind of thing that looks correct and does nothing (wrong path, wrong working directory, a step
without `set -e`, a job nothing depends on) — two real `install.sh` defects survived to review that way.

**The mutation must fail at the step you are testing.** A red run for the wrong reason proves nothing.
In PR5 a proof mutation inserted `return x, nil` as a function's *first* statement; CI went red at
**Vet** (unreachable code) and never reached **Test**, so it demonstrated nothing about the test
wiring. Rewrite the *final* return instead — vet-clean — and confirm the failing step is `Test` and
that the log names the test you predicted.

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
- **DO NOT TAG A RELEASE.** v0.0.1 is authorized per-version, and Ed tags it himself. Implementers
  and coordinators land the release machinery only; they must not create or push a `v*` tag.

## Plan & coordination
- plan location: `docs/hitch-build.md` (authoritative PRD; 6 PRs).
- Solo project: hitch.
- run-log: append transitions to a Solo scratchpad named `hitch run-log`.

## The attempt cap binds on convergence, not on a counter
The nominal cap is 3 rework rounds. **It exists to stop grinding on a problem that is not converging —
so apply it to what the rounds are FINDING, not to their number.**

- **Keep going** while each round closes real ground and surfaces a *distinct new class* of defect.
- **Bail** as soon as a round repeats a class already seen, or produces no new finding — that is
  non-convergence, and it means the PLAN needs revisiting, not the code.

PR5 ran **five** rounds against this cap and was right to: each round found a different live
credential-survival path or a core requirement nothing held (config corruption → key reordering →
whitespace scar on foreign members → duplicate-key credential survival → uninstall picker → install
picker). A bare "three attempts" would have stopped two rounds before the picker gap, which is the
one that mattered most. **Record the overrun honestly in the run-log and the PR body rather than
rounding it down.**

## Tag-the-human conditions
- A required decision, a broken plan assumption, a bail, or rounds that stop converging (above).
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
