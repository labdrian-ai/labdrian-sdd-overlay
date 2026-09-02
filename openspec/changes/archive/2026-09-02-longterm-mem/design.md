# Design: longterm-mem

Long-form rationale and schemas: Engram `sdd/longterm-mem/design-notes` (#3133).

## Technical Approach

One Go module `longterm-mem/` builds one binary (`~/.labdrian-overlay/bin/longterm-mem`): MCP stdio server (`query`, `promote`) plus CLI subcommands. Engram is read via read-only SQLite, the vault via `scripts/retrieve.py`; a purpose-built writer emits `wiki/memory/<address>.md`. `internal/vault.Runner` is the sole `os/exec` importer, bounded to the vault root (R-021). Owners: shell → binary; module → runtime configs, `install-state.json`, vault pages/sidecars; `engine runtime --component longterm-mem` (stdlib, read-only) → `registration.json` and status. Engram → vault only (#3124); supersession patches frontmatter only (#3123).

## Architecture Decisions

| # | Option | Tradeoff | Decision |
|---|---|---|---|
| D1 SQLite | modernc/mattn(CGO)/zombiezen | CI lacks C toolchain | `modernc.org/sqlite` v1.57.0; DSN `mode=ro&_query_only=true&_busy_timeout=2000` (`_query_only` key, `sqlite.go:351`). **Amended during apply**: `immutable=1` is not the primary DSN, but `Open` does retry with it when the primary read-only connection cannot be established — stale, never unsafe, always still `mode=ro&_query_only=true`. `Store.Degraded()` reports that state and `status` surfaces it, so a stale fallback is never mistaken for a healthy connection (verify run 2, WARNING-6) |
| D2 Toolchain | 1.21/1.25.0/1.26.1 | deps need ≥1.25.0 | `go 1.26.1` (as `tui`); CI job `test-longterm-mem` mirrors `test-tui` |
| D3 MCP | official/hand-rolled/mark3labs | hand-rolled drifts; mark3labs pre-1.0 | `modelcontextprotocol/go-sdk` v1.7.0: `AddTool[In,Out]`, `StdioTransport`, exits on stdin close (R-034) |
| D4 Seam | engine-writes/split-by-owner | engine bans TOML libs; one writer per file | `longterm-mem install\|status\|uninstall [--target] [--purge]` (shell): build → `register` → `engine runtime install --component longterm-mem`; engine gains `--component`, `--state-dir`, `LongtermMemAdapter`; `update` refused per component (`rollback` already absent, `main.go:356`); binary removed only on full uninstall/`--purge` |
| D5 Registry | constant/cwd/file | rows stay editable | `~/.labdrian-overlay/vaults.json`, seeded only when absent; `--vault` > env > row; `project` always explicit; missing row → exit 3 |
| D6 Sidecars | manifest-key/dot-file; one/two hashes | plugin-owned file; one hash launders edits | `.raw/.longterm-mem-manifest.json` by address with `body_hash`+`frontmatter_hash`; `.vault-meta/longterm-mem-sync-state.json`; R-033 patch updates `frontmatter_hash` only |
| D7 Page | title/address; per-type/uniform | retitle must not rename; other enum values need fabricated fields | `wiki/memory/<address>.md`, `[[c-NNNNNN\|Title]]`, `aliases`; all Engram types → `concept` + extras (`address engram_id engram_sync_id engram_type engram_revision project`); `related` from judged unsuperseded edges to promoted pages; `index.md` marker block; `log.md` newest-first; `allocate-address.sh` + `address_map`; `status` extends `seed\|developing\|mature\|evergreen` with `superseded`/`archived`; `LintPage()` tolerates both (wiki-lint checks presence only) |
| D8 Query | fusion/native; degrade/reject | re-ranking forbidden; silent degrade hides misconfig | vault rows then Engram FTS5 rows, native orders; linked pair once; rc 10 → `not_provisioned`; unconfigured → exit 3 |
| D9 Writers | rewrite/byte-splice; in-config/sidecar tag | rewrites reorder; unknown keys risky | byte-splice span editors (JSON member, TOML table); ownership = sidecar fingerprint; untagged → exit 6 untouched; `.bak`, tmp+rename, post-write validation (`json.Valid`, go-toml v2 parse-only) |
| D10 Layout | flat/per-spec | — | `internal/{engram,vaultreg,vault,query,promote,ops,mcpserver,register}`; `cmd/longterm-mem/` one file per subcommand |
| D11 Successor | edge direction/age | no live `supersedes` rows | newer observation by `created_at` wins; soft-delete without successor → `archived` |
| D12 Index | egress/local | bodies stay on-machine | always `--no-llm`; provision `setup-retrieve.sh`, refresh `contextual-prefix.py --all` + `bm25-index.py build`; rc≠0 → exit 5 |
| D13 Route | per-file rows/sentinel | source digests untracked | domain {skill, agent, opencode-agent, mcp}, additive; sentinel `longterm-mem/go.mod custom mcp`, zero copy targets; `cmd_apply` runs install once; unrouted `longterm-mem/**` rejected by both parsers |

## Data Flow

install: build → `register` → engine record. sync: Engram → writer → status patch → index → sync-state. query: Engram FTS5 ∥ `retrieve.py` → merge → JSON.

## Contracts

Exit codes 0 ok, 1 internal, 2 usage, 3 vault_not_configured, 4 engram_unavailable, 5 vault_subprocess_failed, 6 registration_conflict, 7 not_found. MCP `query{project,query,top?}`, `promote{project,engram_id}`.

## Testing Strategy

Module: temp DB from live DDL; `os/exec` allowlist scan; fixture vault with fake scripts; page goldens, `LintPage`, hash/sync/R-033 scenarios; in-memory MCP transports; byte-exact config goldens. Engine/shell: adapter status matrix, route tests; `-short` integration for R-015/R-034.

## Slice Map

| Risk | Slices (authored lines) | Split points |
|---|---|---|
| Low | 5 promotion-registration 280–330; 9 overlay-route 180–230 | — |
| Medium | 1 scaffold-module 330–370; 4 promotion-writer 360–410; 6 promotion-mutability 330–380; 7 promotion-sync 340–400 | — |
| High | 2 scaffold-vault 380–430; 3 query 380–440; 8 ops 420–480; 10 overlay-dispatch 470–540; 11 mcp-registration-json 430–490; 12 toml-uninstall 390–440 | 2 registry/runner+retrieve; 3 index/merge; 8 status+doctor/MCP+promote; 10 engine/shell; 11 splice+state/writers; 12 TOML/uninstall |

Auto-chain splits there (#3119).

## Threat Matrix

| Boundary | Applicability | Response / RED tests |
|---|---|---|
| Doc-like paths; VCS/PR | N/A — nothing copied/executed by route; no automation | — |
| Subprocess | Applicable | vault-root-bounded runner, argv only, timeouts; allowlist scan, outside-root, metacharacter spy |
| Manifest route | Applicable | unrouted `longterm-mem/**` rejected, mcp rows zero targets; bash + Go tests |
| Config mutation | Applicable | splice, validation, `.bak`; conflict untouched; invalid aborts |

## Migration / Rollout

None; rollback: reverse-order revert, `uninstall --target all --purge`.

## Open Questions

- [ ] D11 direction assumption; verify on the first real `supersedes` edge.
- [ ] `"type":"stdio"` acceptance in `~/.claude.json` (existing `codegraph` entry carries it).
