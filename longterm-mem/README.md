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
`vaults.json` seeding); registry seeding is lazy and belongs to the commands that write: the first `sync`
(without `--dry-run`), `promote`, `promote reconcile` or `index`, or the MCP `promote` tool, that has to find a
vault makes `~/.labdrian-overlay/vaults.json` with the one row every machine starts with. `status`, `doctor`,
`query`, `sync --dry-run` and the MCP `query` tool only read: on a machine with no registry they behave as if it held
that row, and make nothing (`vaultreg.Vaults.ToRead`, `ToWrite`; `internal/vaultreg`).

## CLI surface (as shipped)

The binary (`longterm-mem <subcommand> [flags]`) currently dispatches:

| Subcommand | Flags | Purpose |
|---|---|---|
| `query` | `[--project P]` `"<text>"` `[--top N]` `[--exclude-types T,...]` `[--goal ID]` `[--vault DIR]` `[--json]` | Query Engram FTS5 (always) and, when named in `sources`, the embedding index and the vault; results are unioned and ranked. The CLI queries FTS only. `--goal` returns that goal's records and the records of no goal, never another goal's (see [Goals](#goals)). |
| `index` | `[--project P]` `[--vault DIR]` `[--rebuild]` | Provision/refresh the vault's local retrieval index (no LLM). |
| `index --embeddings` | `[--project P]` `[--embed-model M]` `[--embed-dimension N]` `[--embed-input-limit C]` `[--embed-endpoint URL]` `[--allow-remote-embedder]` | Build or incrementally update the embedding index over Engram rows against a loopback Ollama backend (`nomic-embed-text`, 768 dims by default). Unchanged rows are fingerprinted and reused, so a re-run after a few new observations takes well under a second. |
| `sync` | `[--project P]` `[--vault DIR]` `[--dry-run]` | Promote eligible Engram observations into vault pages; `--dry-run` reports without writing. An observation is eligible only if it is pinned, explicitly targeted, or carries a curated `topic_key` whose first path segment is not `sdd`, `review`, or `delivery` — an untopiced observation is not automatically eligible. A page already promoted under a previous, looser eligibility rule is neither retracted nor refreshed by `sync` once its observation stops being eligible under the current rule; use an explicit `promote` or a pin to keep such a page current; `doctor` names those pages (see [Frozen pages](#frozen-pages)). |
| `status` | `[--project P]` `[--vault DIR]` `[--json]` | Report Engram reachability, vault provisioning, last sync, and when the embedding index was built. |
| `stale` | `[--project P]` | Report the memories this repository's history disagrees with (removed or moved paths). Reports only; never deletes. A path that climbs out of the project (`a/../../x.go`) is unknown: it is not asked of git and the rest of the report prints. |
| `doctor` | `[--project P]` `[--vault DIR]` `[--json]` | Deeper diagnostics: prerequisites, wiki-lint, registration consistency, embedding index presence/freshness and backend reachability, and an advisory check that names promoted pages `sync` no longer refreshes (see [Frozen pages](#frozen-pages)). Each check answers `PASS`, `FAIL` or `warn`; only a `FAIL` makes the exit code 9, so a `warn` never changes it. |
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
3. **Name of the main checkout's directory** — available even with no
   remote, so a linked worktree and the main checkout answer identically.
   It is a loose name: another repository can have a directory of the same
   name, so it is never adopted from the identity ledger, and a renamed
   repository with no remote starts a new identity unless it declares its
   name in `.longterm-mem-project`.

This is the shared resolver of the `identity` module, the same chain the
engine uses (see `docs/architecture/adr-c23-m2-project-identity.md`). Every
rule answers the same identity from a repository's main checkout and from
any of its linked worktrees. That is the point: one repository addressed
under several names becomes several memories that never find each other.
Outside a git repository the identity is the working directory's name;
`stale`, which checks memories against a repository's history, refuses
there. Memory stored under the old common-directory path stays reachable
through adoption.

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

### Goals

A record created under a goal carries it (`docs/architecture/adr-c23-m5-goal-filter.md`
in the overlay repository): an Engram observation's `topic_key` begins with
`goal:<id>/` (for example `goal:g-42/architecture/auth`; the id ends at the
first `/`), and `promote` tags its page `goal:<id>` after `memory` and the
type. Any other topic belongs to no goal, and a page promoted from it has the
same tags as before goals.

`query --goal <id>` (the MCP `goal` argument) keeps that goal's records and the
records of no goal, and leaves out another goal's: `engram-fts` in its SQL,
before the limit; `engram-embed` on each observation's topic, before its cut;
`vault` on each page's frontmatter `tags`, after asking the vault for three
times `top` candidates (at most 100). A vault page whose tags cannot be read is
left out under a goal and named in a `vault_page_tags_unreadable` diagnostic.
Without a goal nothing changes.

### Frozen pages

`sync` refreshes a promoted page only while its observation is eligible: pinned,
or carrying a curated `topic_key`. A page whose observation is not eligible
(promoted by an explicit `promote --id N` of an untopiced observation, or under
a looser rule that no longer applies) stays exactly as it was written, however
the observation changes later. Such a page is *frozen*, and nothing on it says so.

`doctor` says so, in its `promoted-pages-eligible` check. Each finding names the
page address, the observation id, why the observation is not eligible (it has no
`topic_key`, or its first segment, after any `goal:<id>/` prefix, is empty or one
of `sdd`, `review` and `delivery`)
and the two ways out:

- `longterm-mem promote --id N` refreshes the page now, once;
- pinning observation N in Engram makes `sync` keep the page current from then on.

The check is advisory. It answers `warn`, never `FAIL`: the text form prints
`[warn] promoted-pages-eligible (...)`, `--json` carries `"status": "warn"`, and
the exit code is the one `doctor` would have answered without it (9 for a
`FAIL`, 0 otherwise). Nothing records that a page was promoted explicitly, so an
explicitly promoted page whose observation is not eligible is named too, until
the observation is pinned. A page whose observation was deleted, or is a row of
another project, is not named; when the pages or the observations cannot be
read, the check says it cannot judge, as a warning.

## MCP registration

`longterm-mem mcp` is the stdio MCP server that `register` wires each runtime
to. It exposes three tools. `query` and `promote` take an explicit `project`
field and get no working-directory default (the server's cwd is the host
runtime's, not the project's):

- `query` — `project`, `query`, optional `top`, `exclude_types`, `sources`
  and `goal`. `sources` names which arms answer: `engram-fts` (the default and
  the only default), `engram-embed` (the embedding index; requires a prior
  `index --embeddings`, and the response's `coverage` reports how many live
  rows the index covers), `vault`. `goal` narrows every arm to that goal's
  records and the records of no goal (see [Goals](#goals)). Responses are
  byte-budgeted and carry `diagnostics` naming anything that degraded.
- `get` — read one Engram observation whole by `engram_id`.
- `promote` — explicitly promote one observation into the vault.

Two records describe a registration: the engine-owned view
(`~/.labdrian-overlay/longterm-mem-registration.json`, reported by `overlay
longterm-mem status`) and the module-owned `install-state.json` under the
state directory, which `register`/`unregister` maintain and `install`
adopts when it finds an entry it wrote but no longer has a record of.
