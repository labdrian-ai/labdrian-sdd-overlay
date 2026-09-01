# longterm-mem

Packaged mid/long-term memory layer for the runtimes covered by
`labdrian-sdd-overlay` (Claude Code, opencode, codex).

- **Engram** — mid-term memory: current-project decisions, bugs, conventions.
  `longterm-mem` reads it through a read-only SQLite connection; it never
  shells out to Engram's own `search`/`save` CLI.
- **claude-obsidian-based vault** — long-term memory: a per-project "neural net"
  of core + emerging knowledge, meta-cognition of how that project evolves. The
  vault is resolved per project from configuration (`labdrian-brain` is the
  default for `labdrian-sdd-overlay`); cross-project querying is out of scope
  for the first wave.

Goal: one mutable source of truth per project that the agent team can query
before a fix/feature/core change, so it doesn't reprocess or re-derive things,
or break canonical process decisions that already exist.

## Module boundary

`longterm-mem/` is a standalone Go module (its own `go.mod`, `go 1.26.1`),
outside `engine/`'s zero-dependency boundary — it is free to declare
third-party dependencies (`modernc.org/sqlite`, the MCP `go-sdk`) without
affecting `engine/`'s own dependency manifest.

## Install / status / uninstall

`overlay longterm-mem install|status|uninstall [--target claude|opencode|codex|all] [--purge]`
builds the binary, deploys it to a fixed path, and records/reports its
per-runtime registration through the same `engine runtime` lifecycle surface
the other overlay components use (D4). This is also run automatically, once
per `overlay apply` invocation, whenever `overlay.manifest` carries an
`mcp`-routed row (currently just `longterm-mem/go.mod`, D13) — you do not
normally need to run `install` by hand after a fresh `overlay apply`.

- **`install`**: `go build`s the binary, copies it to the fixed path below,
  then per requested target records the registration via
  `engine runtime install --component longterm-mem --target <t>` and reports
  a per-runtime status (`supported`, `partial`, `unsupported`, or
  `restart_required`) for Claude Code, opencode, and codex.
- **`status`** / **`uninstall`**: never build. They talk to the same engine
  runtime lifecycle surface directly. No `update`/`rollback` action is
  offered for this component — reinstall to pick up a new binary.
- **`--purge`** (uninstall only): removes the binary immediately regardless
  of how many targets are still tracked installed.
- The binary is removed only once uninstalling leaves zero tracked
  install-state targets (i.e. `--target all`, or the last remaining single
  target), or when `--purge` is passed — uninstalling one target while
  another remains installed always leaves the binary in place.

The binary is installed at one fixed, documented, persistent path:

```
~/.labdrian-overlay/bin/longterm-mem
```

(`$STATE_DIR/bin/longterm-mem`, `STATE_DIR` defaults to `~/.labdrian-overlay`
and is overridable via the `STATE_DIR` environment variable — mainly for
tests). Once `install` places it there it stays there, invocable, until an
`uninstall` removes it (R-015).

**Current scope of `install`**: the overlay-side build/copy/record/report
loop above is fully wired end-to-end. The two module-owned CLI subcommands it
also calls per target, `longterm-mem vaults seed` and
`longterm-mem register --target <t>` (writing the actual MCP entry into
each runtime's own config file — `~/.claude.json`, `opencode.json`,
`config.toml`), do not exist yet: `install` calls them and tolerates their
current "unknown subcommand" refusal with a warning, so the rest of the
install path (binary build/copy, `engine runtime install`'s registration
bookkeeping and per-runtime status report) still completes. Those two
subcommands land in later slices (`vaults`, `register`/`unregister`); once
they do, `install`/`uninstall` start actually registering/unregistering
without any change on the overlay side.

## CLI surface (as shipped)

The binary (`longterm-mem <subcommand> [flags]`) currently dispatches:

| Subcommand | Flags | Purpose |
|---|---|---|
| `query` | `--project P` `"<text>"` `[--top N]` `[--vault DIR]` `[--json]` | Query vault + Engram FTS5, merged and ranked. |
| `index` | `--project P` `[--vault DIR]` `[--rebuild]` | Provision/refresh the vault's local retrieval index (no LLM). |
| `sync` | `--project P` `[--vault DIR]` | Promote eligible Engram observations into vault pages. |
| `status` | `--project P` `[--vault DIR]` `[--json]` | Report vault/index/sync-state health. |
| `doctor` | `--project P` `[--vault DIR]` `[--json]` | Deeper diagnostics: prerequisites, wiki-lint, registration consistency. |
| `promote` | `--project P` `--id N` `[--vault DIR]` | Explicitly promote one Engram observation by id. |
| `mcp` | — | Run the MCP stdio server (`query`, `promote` tools). |

Global env overrides: `LONGTERM_MEM_ENGRAM_DB` (Engram database path).

Not yet implemented (planned, see `openspec/changes/longterm-mem`):
`register`, `unregister`, `vaults` — the module-owned CLI surface that
writes/removes each runtime's MCP config entry and manages `vaults.json`
seeding directly.

## MCP registration

`longterm-mem mcp` is the stdio MCP server the `install` path above wires
each runtime to use once `register` (a later slice) actually writes the
entry. Engine records its own view of each runtime's registration
(`~/.labdrian-overlay/longterm-mem-registration.json`) independently of the
module-owned `install-state.json` a later slice will add; `overlay
longterm-mem status` reports the engine-owned view today.

Status: install/status/uninstall exist end-to-end at the shell+engine layer
(this document). The module-owned MCP config writers (`register`,
`unregister`) and `vaults` CLI land in later slices; this document will be
updated again once they do.
