# Changelog

All notable changes to hitch are recorded here. Versions follow [semantic versioning](https://semver.org).

Each release ships the same version number three ways: the `v*` git tag, the GoReleaser GitHub
Release (binaries + `checksums.txt`), and the `hitch-mcp` npm shim. The shim resolves the release it
downloads from its own `package.json` version, so those numbers are not allowed to drift.

## [0.1.1] — unreleased

### Fixed

- **`npx hitch-mcp <subcommand>` no longer installs a junk MCP server.** The shim used to prefix
  `install` onto every invocation, so `npx hitch-mcp uninstall example` ran `hitch install uninstall
  example` — writing a server named `uninstall` pointing at `https://uninstall`, with the intended
  server name stored as its bearer token, instead of removing anything. The shim now forwards
  `install`, `list`, `prompt`, `scan`, `uninstall` and `version` through untouched and only defaults
  to `install` for arguments that are not a known subcommand (#22).

  Users on `hitch-mcp@0.1.0` who ran an `uninstall`, `scan`, `list` or `prompt` subcommand through
  npx should check their harness configs for a stray `uninstall`/`scan`/`list`/`prompt` server entry
  and remove it — `hitch scan` will report it as holding a credential.

- **The release workflow no longer fails after publishing the GitHub Release.** The npm version step
  now passes `--allow-same-version`, so it succeeds when the committed `package.json` version already
  matches the tag being released, instead of exiting 1 and skipping `npm publish`.

### Changed

- The committed `npm/hitch-mcp/package.json` version now tracks the release version (0.0.1 → 0.1.1)
  rather than trailing it. Running the shim straight from a checkout previously resolved
  `HITCH_VERSION` to `v0.0.1` and downloaded a long-superseded binary.
- npm metadata: added `keywords`, `author` and `publishConfig.access`.

## [0.1.0] — 2026-08-11

### Added

- `--claim` / `--claim-url`: exchange a single-use claim code for the bearer token over HTTPS at
  install time, so the durable credential never reaches shell history or `~/.npm/_logs/`. A failed
  exchange leaves every config byte-for-byte untouched (#15).

## [0.0.7] — 2026-08-11

### Changed

- The npm shim publishes from CI via npm OIDC trusted publishing. Every version from 0.0.7 onward
  carries a provenance attestation verifiable with `npm audit signatures` (#14).

## [0.0.6] — 2026-08-11

### Fixed

- Homebrew cask token template, so the guarded tap publisher resolves instead of erroring.

## [0.0.5] — 2026-08-11

### Changed

- The npm shim binds the release it downloads to its own published version, and the downloader is
  covered by tests (redirects, checksum mismatch, 404, redirect loops) (#13).

## [0.0.4] — 2026-08-10

### Fixed

- `scan` reports a Codex credential held by any server, not only the first (#11).

## [0.0.3] — 2026-08-10

### Added

- Codex TOML support for `scan` and `uninstall` (#9).
- Project scope: `-p`/`--project` writes project-local MCP config paths, with a gitignore guard (#8).

## [0.0.2] — 2026-08-10

### Added

- `scan`, `uninstall` and `prompt` — closing the revoke gap 0.0.1 shipped with (#5).
- stdio MCP servers across the seven JSON clients (#4).

## [0.0.1] — 2026-08-10

### Added

- First release: harness detection, the interactive per-harness picker, remote HTTP install across
  seven JSON clients plus Codex, and distribution via GoReleaser, Homebrew, `install.sh` and the
  `hitch-mcp` npm shim (#1, #2, #3).

[0.1.1]: https://github.com/artisan-build/hitch/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/artisan-build/hitch/compare/v0.0.7...v0.1.0
[0.0.7]: https://github.com/artisan-build/hitch/compare/v0.0.6...v0.0.7
[0.0.6]: https://github.com/artisan-build/hitch/compare/v0.0.5...v0.0.6
[0.0.5]: https://github.com/artisan-build/hitch/compare/v0.0.4...v0.0.5
[0.0.4]: https://github.com/artisan-build/hitch/compare/v0.0.3...v0.0.4
[0.0.3]: https://github.com/artisan-build/hitch/compare/v0.0.2...v0.0.3
[0.0.2]: https://github.com/artisan-build/hitch/compare/v0.0.1...v0.0.2
[0.0.1]: https://github.com/artisan-build/hitch/releases/tag/v0.0.1
