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
  for the first wave. The vault is **opt-in per query**: it is consulted only
  when a caller names it in `sources` (see `openspec/decisions/vault-value.md`
  for why). Engram is the default retrieval store, through two arms: trigram
  FTS5, always on, and an embedding index over the same rows, opt-in.

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

**`install` end to end**: after the overlay-side build/copy/record/report
loop above, `install` calls `longterm-mem register --target <t>` per
requested target, which writes the MCP entry into that runtime's own config
file (`~/.claude.json`, `opencode.json`, `config.toml`), and `uninstall`
calls `longterm-mem unregister`. Both are ownership-tagged, idempotent, and
refuse an untagged same-named entry. The only planned module-owned
subcommand that does not exist yet is `vaults` (direct management of
`vaults.json` seeding); registry seeding is lazy inside `vaultreg.Resolve`.

## CLI surface (as shipped)

The binary (`longterm-mem <subcommand> [flags]`) currently dispatches:

| Subcommand | Flags | Purpose |
|---|---|---|
| `query` | `[--project P]` `"<text>"` `[--top N]` `[--exclude-types T,...]` `[--vault DIR]` `[--json]` | Query Engram FTS5 (always) and, when named in `sources`, the embedding index and the vault; results are unioned and ranked. The CLI queries FTS only. |
| `index` | `[--project P]` `[--vault DIR]` `[--rebuild]` | Provision/refresh the vault's local retrieval index (no LLM). |
| `index --embeddings` | `[--project P]` `[--embed-model M]` `[--embed-dimension N]` `[--embed-input-limit C]` `[--embed-endpoint URL]` `[--allow-remote-embedder]` | Build or incrementally update the embedding index over Engram rows against a loopback Ollama backend (`nomic-embed-text`, 768 dims by default). Unchanged rows are fingerprinted and reused, so a re-run after a few new observations takes well under a second. |
| `sync` | `[--project P]` `[--vault DIR]` `[--dry-run]` | Promote eligible Engram observations into vault pages; `--dry-run` reports without writing. An observation is eligible only if it is pinned, explicitly targeted, or carries a curated `topic_key` whose first path segment is not `sdd`, `review`, or `delivery` — an untopiced observation is not automatically eligible. |
| `status` | `[--project P]` `[--vault DIR]` `[--json]` | Report Engram reachability, vault provisioning, last sync, and when the embedding index was built. |
| `stale` | `[--project P]` | Report the memories this repository's history disagrees with (removed or moved paths). Reports only; never deletes. |
| `doctor` | `[--project P]` `[--vault DIR]` `[--json]` | Deeper diagnostics: prerequisites, wiki-lint, registration consistency, embedding index presence/freshness and backend reachability. |
| `promote` | `[--project P]` `--id N` `[--vault DIR]` | Explicitly promote one Engram observation by id. `promote reconcile <address>` adopts one already-promoted page whose precedence entry cannot tell longterm-mem's own write from a human edit, so promotion stops refusing it; deliberately one address at a time. |
| `mcp` | — | Run the MCP stdio server (`query`, `get`, `promote` tools). |
| `register` | `--target claude\|opencode\|codex\|all` `[--binary PATH]` `[--config-root DIR]` `[--state-dir DIR]` | Write this binary's MCP entry into the runtime's config file (ownership-tagged, idempotent). |
| `unregister` | `--target claude\|opencode\|codex\|all` `[--config-root DIR]` `[--state-dir DIR]` | Remove the entry `register` wrote; an entry it does not own is left untouched and reported as `unmanaged`. |

### Project identity

`--project` is optional. Omitted, the project is resolved from the working
directory by a chain, first match wins (`internal/projectid`):

1. **Declared** — a `.longterm-mem-project` file at the repository root
   holding exactly one non-empty line with the project's name. It beats
   both derived rules; it is the only rule a human controls. An empty or
   multi-line file is rejected, never silently skipped.
2. **Normalized origin remote** — reduced to `host/owner/name`, so
   `git@github.com:acme/widgets.git`, `https://github.com/acme/widgets`
   and `https://github.com/acme/widgets.git` all collapse to one value.
3. **Realpath of the git common directory** — always available, even with
   no remote; absolute and symlink-resolved so a linked worktree and the
   main checkout answer identically.

Every rule answers the same identity from a repository's main checkout and
from any of its linked worktrees. That is the point: one repository
addressed under several names becomes several memories that never find each
other. Outside a git repository, an omitted `--project` is refused with the
reason resolution failed — an unresolved project must never become an empty
one.

When `--project` **is** given and the working directory is inside a git
repository that resolves to a different identity, the command prints a
`WARN` and proceeds. It warns rather than refuses because addressing
another project on purpose is legitimate (CI, an overlay repository
operating on the projects it manages); what must not happen is doing it
*silently*.

The MCP tools keep their explicit `project` field and get no working-
directory default: the server's working directory is the host runtime's,
not the project's.

Global env overrides: `LONGTERM_MEM_ENGRAM_DB` (Engram database path),
`LONGTERM_MEM_VAULT` (vault path), `LONGTERM_MEM_VAULTS_FILE` (vault
registry path), `LONGTERM_MEM_STATE_DIR` (state directory, default
`~/.labdrian-overlay/longterm-mem`).

## MCP registration

`longterm-mem mcp` is the stdio MCP server that `register` wires each runtime
to. It exposes three tools. `query` and `promote` take an explicit `project`
field and get no working-directory default (the server's cwd is the host
runtime's, not the project's):

- `query` — `project`, `query`, optional `top`, `exclude_types`, and
  `sources`. `sources` names which arms answer: `engram-fts` (the default and
  the only default), `engram-embed` (the embedding index; requires a prior
  `index --embeddings`, and the response's `coverage` reports how many live
  rows the index covers), `vault`. Responses are byte-budgeted and carry
  `diagnostics` naming anything that degraded.
- `get` — read one Engram observation whole by `engram_id`.
- `promote` — explicitly promote one observation into the vault.

Two records describe a registration: the engine-owned view
(`~/.labdrian-overlay/longterm-mem-registration.json`, reported by `overlay
longterm-mem status`) and the module-owned `install-state.json` under the
state directory, which `register`/`unregister` maintain and `install`
adopts when it finds an entry it wrote but no longer has a record of.
