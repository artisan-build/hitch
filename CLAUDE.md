# hitch

A Go CLI that installs an MCP server into every detected coding agent on a machine, handles the
credential correctly, and can revoke it. Public OSS under `artisan-build`.

## Workflow

Feature builds: see `.solo/workflow.md` and the `multi-agent-build` skill.
The authoritative spec is `docs/hitch-build.md` — read it before changing behavior.

## Standing rules

- **No code path prints a token.** Not in success output, not in errors, not in `--dry-run`.
- **Never rewrite a config that won't parse.** Report the file and stop; clobbering a user's real
  client config is worse than refusing.
- **Writes are atomic and `0600`.** Temp file in the target dir opened `O_EXCL`, then rename.
- **A non-TTY run never blocks on a prompt.** Agents drive this CLI; with no TTY and neither `-y`
  nor `--client`, exit non-zero naming both flags.
- **Detection feeds a choice, not a decision.** The interactive per-harness picker is a core
  requirement, not a flag.
- **Do not tag releases.** Versions are tagged by hand on explicit approval.
