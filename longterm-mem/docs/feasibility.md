# longterm-mem — feasibility exploration

Date: 2026-08-28. Read-only mapping of the three systems involved: Engram, the
claude-obsidian plugin, and the overlay's runtime deployment layer.

## Verdict

**Feasible.** Both memory backends can be consumed headless by a Go program, and
all three runtimes (Claude Code, opencode, codex) already consume MCP servers, so
the delivery shape is known. What does not exist yet is the glue: the overlay has
no MCP registration code, no slot for a non-skill component, and no vault has
ever been indexed.

## Backend A — Engram (mid-term memory)

Headless: **YES, unconditional.**

- Storage: plain SQLite 3 (WAL) at `~/.engram/engram.db`. FTS5 already built on
  `observations` and `user_prompts`. `observations` carries `embedding BLOB`,
  `topic_key`, `scope`, `project`, `deleted_at` (soft delete — filter it).
  `memory_relations` is the supersession/conflict graph.
- HTTP API on `127.0.0.1:7437` (already running), JSON, unauthenticated by
  default. Routes are undocumented in the README (extracted from the binary):
  `/search`, `/observations*`, `/sessions*`, `/context`, `/stats`, `/export`,
  `/import`, `/conflicts*`, `/sync*`, `/project/current`.
- `engram mcp --tools=agent` over stdio for MCP framing.
- `engram obsidian-export --vault <path> [--project --since --watch]` exists —
  Engram can already write itself into an Obsidian vault. Candidate bridge for
  "promote mid-term to long-term"; evaluate before building a custom one.

Caveats:
- `engram search`/`save` CLI has **no `--json`** — never shell out to it; use
  SQLite (read-only) or HTTP.
- `GET /project/current` resolves the *server's* cwd, not the caller's — always
  pass `project` explicitly.
- HTTP surface is unversioned; the DB schema is the more stable contract.

## Backend B — claude-obsidian vault (long-term memory)

Headless: **PARTIAL — design yes, data no.**

- Retrieval pipeline is four pure-stdlib Python scripts inside the vault
  (`scripts/retrieve.py`, `bm25-index.py`, `rerank.py`, `contextual-prefix.py`).
  No pip, no venv, no API key, no Obsidian CLI (transport is `filesystem`).
- Index on disk is plain JSON: `.vault-meta/bm25/index.json` (Okapi BM25,
  documented schema) + `.vault-meta/chunks/<page-address>/chunk-NNN.json`.
  Optional `.vault-meta/embed-cache.json` for rerank.
- Query entrypoint: `python3 scripts/retrieve.py "<q>" --top 5` → JSON with
  `page_address`, `absolute_path`, `bm25_score`, `rerank_score`, `snippet`.
  Exit code **10 = not provisioned** (clean feature-detect signal).
- Rerank uses ollama + `nomic-embed-text` — both present and reachable on this
  machine; absent → silent no-op, BM25 order returned.
- DragonScale Mechanism 2 gives every page a rename-stable id `c-NNNNNN`
  (`.raw/.manifest.json` → `address_map`). This is the natural join key between
  the vault and any external index or Engram observation.
- Frontmatter schema is prose-only (`WIKI.md`), no JSON Schema. A Go consumer
  must hand-model `type/title/created/updated/tags/status/related/sources`.

Blockers / gaps:
- **No vault has ever been indexed.** Zero `.vault-meta/chunks/` and zero
  `bm25/index.json` under `$HOME` across `labdrian-brain`, `trading-brain`,
  `crisis-donation-brain`. Every `retrieve.py` call returns exit 10 today.
  Unblock: `bash bin/setup-retrieve.sh --no-llm` inside each vault.
- **Nothing refreshes the index.** No hook does it; documented as manual.
  `longterm-mem` must own re-indexing (mtime watch → `contextual-prefix.py
  --all` + `bm25-index.py build`).
- Scripts resolve `VAULT_ROOT` from `__file__` → invoke per-vault by path, not
  as a global binary. Alternative: read `index.json` directly in Go for queries
  (BM25 is ~40 lines), shell out only for builds.
- Plugin hooks (`hooks.json`) are Claude-Code-only: hot.md restore on
  SessionStart/PostCompact, auto-commit on Write/Edit, hot.md rewrite on Stop.
  None of that runs on opencode/codex.

## Delivery layer — labdrian-sdd-overlay

Integration: **PARTIAL.**

Reusable as-is:
- `engine/runtime` `Adapter` interface (Install/Update/Status/SyncCheck/
  Rollback/Uninstall), honest capability statuses (`supported | partial |
  unsupported | restart_required`), config-root resolution for all three
  runtimes, `--target all` fan-out.
- Ownership-tagged, atomic, backed-up config merge pattern
  (`engine/settings`, `codex.go`).
- Pattern for standalone Go modules under `tools/` with their own `go.mod` and
  third-party deps.
- All three MCP config shapes are already in use on this machine:
  - Claude: `~/.claude.json` → `mcpServers.<name> = {command, args}`
  - opencode: `~/.config/opencode/opencode.json` → `mcp.<name> = {type:
    "local", command: [argv...], enabled}` (single argv array)
  - codex: `~/.codex/config.toml` → `[mcp_servers.<name>] command / args`

Missing:
1. No MCP registration writer for any runtime. Three formats, three writers.
   Codex needs TOML; `engine/` is dependency-frozen (ADR-15, `zero_fetch_test.go`
   bans `net`, `os/exec`, third-party imports) → the writer must live outside
   `engine/`, or hand-roll a section-preserving TOML editor.
2. No persistent-binary install path other than `~/.claude/bin/gentle-ai-overlay`.
   `tools/*` binaries are built to mktemp and deleted.
3. No manifest/registry slot for a non-skill component. `overlay.manifest`
   routes are a hardcoded 3-way case (`skill | agent | opencode-agent`);
   `skills.registry.yaml` parser is strict and skills-only. Rows under
   `longterm-mem/**` with no route today misresolve to `skills/longterm-mem/**`.
4. `bin/labdrian-overlay` never calls `engine runtime ...` — the opencode parity
   plugin is not even installed on this machine because of it. A new component
   needs a real `labdrian <verb>` or it ships inert.
5. Uninstall/ownership semantics for MCP entries are unspecified across the three
   formats.

## Recommended shape

- `longterm-mem/` = its own Go module (like `tools/*`), free to take an MCP SDK
  and a SQLite driver (`modernc.org/sqlite`, read-only + WAL) without touching
  the engine's zero-dep invariant.
- Exposed as an **MCP stdio server** (spawned per session by each runtime, like
  Engram — no daemon, live queries during the conversation) plus CLI subcommands
  for `index`, `sync`, `status`, `doctor`.
- Installed to a fixed path by a new `bin/labdrian-overlay` subcommand; a fourth
  adapter-style writer per runtime registers the MCP entry with an ownership
  marker following `codex.go`'s manifest + atomic-write pattern.
- New `overlay.manifest` route (e.g. `mcp`) so the component is tracked and
  deployed, not silently misrouted.
- Join model: Engram `observation.id`/`topic_key` ↔ vault `page_address`
  (`c-NNNNNN`). Promotion path mid-term → long-term prototyped first with
  `engram obsidian-export`, replaced only if it proves insufficient.

## Open questions

- Does `engram obsidian-export` output land inside the plugin's `wiki/` contract
  (frontmatter, `address_map`) or does it need a post-processing step?
- Should `longterm-mem` own `.vault-meta` index refresh for all vaults, or only
  the one it is pointed at?
- Which vault is the canonical long-term store for the overlay's own projects
  (`labdrian-brain` is the obvious default)?
