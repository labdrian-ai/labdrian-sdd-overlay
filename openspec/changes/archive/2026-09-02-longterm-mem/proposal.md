# Proposal: longterm-mem — Packaged Mid/Long-Term Memory Layer

## Intent

A project under `labdrian-sdd-overlay` has two memories that do not talk to each other: Engram (mid-term, per-project observations) and a claude-obsidian vault (long-term, curated core and emerging knowledge). Nothing promotes a settled decision from the first into the second, nothing keeps a promoted page current or retracted, and no runtime can query both in one call. Agents therefore re-derive, or silently break, canonical decisions while doing a fix, a feature, or a core change.

Outcome: one mutable, per-project source of truth that any agent on Claude Code, opencode, or codex CAN consult through a single MCP `query`, fed selectively from Engram by an on-demand `sync`, and kept honest by update-in-place, local-edit precedence, and supersession propagation. 35 EARS requirements, brief Rev 2 (#3103, #3109, #3110).

### Non-Goals (settled decisions, not open)

- No automatic consultation from agent workflows (SDD phases, inception, session start) — #3106.
- No cross-project querying; one vault per project, `labdrian-brain` default for this project — #3102.
- No `Update`/`SyncCheck`/`Rollback` wiring for the installed component; `Install`/`Status`/`Uninstall` only — #3106.
- No background daemon; MCP server per session, CLI on demand — #3098, R-034.
- No `engram obsidian-export` on any promotion path — #3099.
- No back-sync of any kind from vault edits to Engram — #3124. Deferred candidate for a later wave: "local edit → Engram trace".

## Scope

### In Scope — six capability areas over the 12 review slices of entry #3120 (ids and order kept)

| # | Slice id (`longterm-mem-…`) | Requirements | Capability area |
|---|---|---|---|
| 1 | scaffold-module | R-001, R-021, R-002, R-020 | Memory access: module outside `engine/`, read-only Engram SQLite, `deleted_at`/project scoping, no `engram` CLI shelling; README per-project rewording (CHK-05) |
| 2 | scaffold-vault | R-003, R-022, R-023, R-004, R-024 | Memory access: vault registry (override, default, reject), `retrieve.py` invoke/parse, exit-10 mapping |
| 3 | query | R-005, R-025, R-006, R-026 | Query: index lifecycle CLI, unified library-level fan-out/merge, degrade path |
| 4 | promotion-writer | R-007, R-027 | Promotion: eligibility predicate, contract-conformant page emission |
| 5 | promotion-registration | R-028, R-029 | Promotion: DragonScale address, `.raw/.manifest.json`, `index.md`/`log.md` |
| 6 | promotion-mutability | R-008, R-030 | Promotion: update-in-place, local-edit precedence |
| 7 | promotion-sync | R-009, R-031, R-033 | Promotion: `sync`, index rebuild + sync-state, supersession/soft-delete propagation |
| 8 | ops | R-010, R-011, R-012, R-034, R-032 | Ops: `status`, `doctor`, MCP stdio server, no daemon, explicit `promote` (CLI + MCP) |
| 9 | overlay-route | R-013, R-035 | Overlay integration — surfaces: `bin/labdrian-overlay` `route_resolve`, `engine/skills/ondisk.go` `nonSkillRoutes`, `engine/installer/route_test.go` contract (two parsers plus one test contract per #3121, not three parsers). Parallel with 1–8 |
| 10 | overlay-dispatch | R-014, R-015 | Overlay integration: build/copy/register wiring, persistent binary path |
| 11 | mcp-registration-json | R-016, R-017 | MCP registration: Claude Code, opencode, install-state sidecar |
| 12 | mcp-registration-toml-uninstall | R-018, R-019 | MCP registration: codex TOML, ownership-tagged uninstall |

Delivery: feature-branch-chain on tracker `feat/longterm-mem`, each slice ≤400 authored lines (generated `go.mod`/`go.sum` lock lines excluded), no size exception (#3119).

### Out of Scope

- The Non-Goals above; changes to Engram's save pipeline or the vault's retrieval scripts; third-party dependencies inside `engine/`.
- A "human mirror" export command; `sync-manifest` preservation of `mcp` rows (item 21's obligation when it starts).

## Capabilities

### New Capabilities

- `longterm-mem-memory-access`: standalone module, read-only Engram access and scoping, per-project vault resolution, vault query invoke/parse (R-001–R-004, R-020–R-024)
- `longterm-mem-query`: index lifecycle CLI and unified library-level `query` (R-005, R-006, R-025, R-026)
- `longterm-mem-promotion`: eligibility, page emission, addressing/registration, mutability, precedence, `sync`, supersession, explicit `promote` (R-007–R-009, R-027–R-033)
- `longterm-mem-ops`: `status`, `doctor`, MCP stdio server, no-daemon property (R-010–R-012, R-034)
- `longterm-mem-install`: `bin/labdrian-overlay install|status|uninstall longterm-mem` dispatch and the persistent binary path (R-014, R-015)
- `longterm-mem-mcp-registration`: ownership-tagged writers for three runtimes and safe uninstall (R-016–R-019)

### Modified Capabilities

- `overlay-agent-route`: route column domain gains `mcp`; an unrouted or unrecognized `longterm-mem/**` row is rejected, never defaulted to `skill` (R-013, R-035)
- `skills-ondisk-validation`: `nonSkillRoutes` recognizes `mcp` so `DeployableManifestPaths` keeps those rows out of the on-disk gate (R-013)
- `runtime-lifecycle`: `engine runtime` gains the `longterm-mem` component for `install|status|uninstall` (registration record and honest per-runtime status); no `Update`/`Rollback` (R-014)

## Approach

- **Module**: `longterm-mem/` is its own Go module outside `engine/` (the ADR-15 compile-time gate `engine/skills/zero_fetch_test.go` stays green), following the `tui/` and `tools/*` precedent; free to take an MCP SDK, a SQLite driver, and a TOML library.
- **Surfaces**: one binary. `longterm-mem mcp` serves `query` and `promote` over stdio; CLI `index`, `sync`, `status`, `doctor`, `promote`. Engram is read via a read-only SQLite connection (never the `engram` CLI); the vault via `scripts/retrieve.py`; the index via `setup-retrieve.sh`, `contextual-prefix.py`, `bm25-index.py` — subprocesses run only inside the one resolved vault.
- **State files** (overlay precedent `~/.labdrian-overlay/state.json`, atomic tmp+mv): binary at a fixed path under `~/.labdrian-overlay/bin/`; vault registry `~/.labdrian-overlay/vaults.json` keyed by project name, pre-seeded `labdrian-sdd-overlay → ~/labdrian-brain`; a sidecar precedence store inside the vault keyed by `page_address` holding `{content_hash, engram_id, promoted_revision, last_promoted_at}`; `.vault-meta/longterm-mem-sync-state.json`.
- **Promotion writer** (purpose-built; replaces the rejected export): universal frontmatter, `type` mapped onto the wiki enum with `engram_type`/`engram_id`/`project` extras, `related` from `memory_relations`, address via `allocate-address.sh`, `.raw/.manifest.json` plus `index.md`/`log.md` registration, id-first filename. Strictly unidirectional Engram → vault.
- **Canon wins (#3123)**: supersession or soft-delete patches only frontmatter `status`/`related`/`superseded_by`, even on a locally edited page; the body is never rewritten; R-030's hash check guards body overwrites only; the patch updates the sidecar hash so the next `sync` does not misread it as a human edit.
- **Install split**: `bin/labdrian-overlay` builds and copies the binary (`engine/` bans `os/exec`); the registration writers (JSON for Claude Code and opencode, section-preserving TOML for codex) live in the module, with ownership tracked in a sidecar install-state manifest rather than inside any runtime's config schema; `engine runtime` records and reports.
- **Design-phase decisions, presented not made**: SQLite driver — `modernc.org/sqlite` (pure Go, no CGO, simplest cross-compile, heavier and slower) vs `mattn/go-sqlite3` (CGO, fast and mature, adds a C-toolchain build requirement new to this repo) vs `zombiezen.com/go/sqlite` (pure Go via WASM/wazero, different runtime overhead); Go toolchain — `go 1.21` (engine, tools) vs `go 1.26.1` (tui).

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `longterm-mem/` (`go.mod`, `cmd/`, internal packages, tests) | New | Module, MCP server, CLI, promotion writer, registration writers |
| `longterm-mem/README.md` | Modified | Per-project wording (CHK-05) |
| `overlay.manifest` | Modified | `mcp`-routed rows |
| `bin/labdrian-overlay` | Modified | `route_resolve` gains `mcp`; `install|status|uninstall longterm-mem` |
| `engine/skills/ondisk.go` (+ tests) | Modified | `nonSkillRoutes` gains `mcp` |
| `engine/installer/route_test.go` | Modified | Route-domain contract |
| `engine/runtime/*` | Modified | `longterm-mem` component record and status |
| User machine: `~/.labdrian-overlay/{bin/,vaults.json}`, `~/.claude.json`, `~/.config/opencode/opencode.json`, `~/.codex/config.toml`, vault `wiki/`, `.raw/`, `.vault-meta/` | Runtime | Written only by install, `sync`, `promote`, `index` |

## Risks (ranked from #3121)

| Risk | Likelihood | Mitigation (slice) |
|---|---|---|
| Codex section-preserving TOML writer: no stdlib support, no overlay precedent | High | Isolated slice 12; TOML library admitted by module placement; byte-level round-trip fixtures |
| Engram `pinned` column unconfirmed by a live schema read | Med | Design runs `.schema observations` before the R-007 SELECT filters land in slice 4 |
| New persistent-binary installer surface (only `ENGINE_BINARY` exists today) | Med | Own slice 10; additive on item 24's state model; install-persistence test |
| Go toolchain version inconsistent across modules | Low | Explicit design choice; CI toolchain verified in slice 1 |
| Vault write path clobbers a human edit or reproduces the "disconnected island" defect | High | Slices 4–7: wiki-lint pass on fixtures, hash sidecar with skip-and-diagnose, status-only patch; tests run on scratch vault copies, never on `~/labdrian-brain` |
| Corrupting unrelated MCP entries in three user config files | Med | Slices 11–12: fixture configs, byte-identical assertions, `.bak`, untagged-conflict refusal |
| External Python/shell script contract drift | Med | `doctor` `runtime-prerequisites`; subprocess failure exits non-zero (R-025) |

## Strict-TDD posture

RED then GREEN per requirement. Focused: `cd longterm-mem && go test ./...` and `bash -n bin/labdrian-overlay`; broad adds `cd engine && go test ./...`, `cd tui && go test ./...`, `bash -n bin/overlay`. Fixtures: temporary SQLite DB, scratch vault copy, fixture config files. `TestZeroFetchImportAllowlist` runs unmodified from slice 1.

## Acceptance evidence per capability area (brief Minimum PASS Evidence)

| Area | Evidence |
|---|---|
| Memory access | `go build ./longterm-mem/...` with `engine/go.mod` require-free and the zero-fetch test passing; read-only connection (default and override); `deleted_at`/project filter; subprocess-spy for `engram search`/`save`; vault override/default/reject; `--top` default and override; exit 10 → `not_provisioned` |
| Query | Index rebuild and first-provision; failing-script non-zero exit; library-level grouping, `engram_id`↔`page_address` dedupe, required-`project` rejection; Engram-only degrade path |
| Promotion | Five-criteria eligibility; frontmatter/type/extras with wiki-lint pass; `related` resolvability; filename stability on retitle; address allocate/reuse and manifest entries; `index.md`/`log.md`; double-promotion and retitle; local-edit skip with diagnostic; three-scenario `sync`; sync-state timestamp; supersession, soft-delete, and negative case; explicit promote incl. invalid id |
| Ops | `status` under healthy/not-provisioned/never-synced; `doctor` per named check; MCP handshake lists `query` and `promote`, `query` round trip over stdio; no residual process after each subcommand |
| Overlay integration | `mcp` fixture row on both parsers plus the route contract; unrouted row rejected; install builds and copies then registers with per-target status; `status`/`uninstall` skip the build; binary persists at the fixed path |
| MCP registration | Three-scenario merge per runtime (additive, idempotent, untagged refusal); TOML byte-level round-trip; per-runtime uninstall (selective, untagged preserved, partial-vs-full binary policy) |

## Rollback Plan

Code: revert the tracker merge in reverse slice order; child PRs never reach `main` directly. User machines: `bin/labdrian-overlay uninstall longterm-mem --target all` removes only tagged entries and, on full uninstall, the binary; `~/.labdrian-overlay/vaults.json` and sidecars are inert. Vault: promoted pages live in the vault's own git history (plugin auto-commit), so `git revert` inside the vault restores the pre-promotion state; `.vault-meta/` is regenerable.

## Dependencies

- Roadmap items 18, 19, 24 — all delivered on `main` (#2067, splice #3117).
- On the user machine: `python3`, the vault's `scripts/*` and `bin/setup-retrieve.sh`, optional ollama — surfaced by `doctor`, never assumed.

## Open items for sdd-design (explicit; none assumed)

1. SQLite driver (three candidates above) and its CI/build consequence.
2. Go toolchain version for `longterm-mem/go.mod`.
3. MCP SDK selection for the stdio server.
4. Live confirmation of `observations.pinned` and the `memory_relations` columns before the R-007/R-033 queries are written.
5. Exact division between `engine runtime install|status|uninstall longterm-mem` (record/report; no `os/exec`, no third-party deps) and the module-resident writers that mutate the three config files — R-014's scenario requires `engine runtime` to report per-runtime status.
6. Sidecar names and locations: the precedence store (`.raw/.longterm-mem-manifest.json` vs a key inside `.raw/.manifest.json`) and the install-state manifest under `~/.labdrian-overlay/`.
7. The exact binary filename under `~/.labdrian-overlay/bin/` and how `vaults.json` is pre-seeded (shipped at install vs written on first run).
8. Engram `type` → wiki `type` enum mapping table, the id-first filename pattern, and the wikilink form used in `related`.
9. Untagged-entry detection per format (entry present but absent from the install-state manifest) and `--target all` fan-out for a component that is not a file copy.

## Proposal question round

Ran interactively before this phase; outcomes recorded verbatim as #3123 (canon wins, status-only patch) and #3124 (unidirectional promotion). The resulting assumptions are the Non-Goals and the "Canon wins" bullet above. A second round is available on request through the orchestrator.

## Success Criteria

- [ ] An agent on any of the three runtimes calls MCP `query` (with `project`) and receives one source-grouped list spanning Engram and the project's vault; nothing consults it automatically.
- [ ] `longterm-mem sync` promotes newly eligible and revised observations and leaves the vault queryable without a manual re-index.
- [ ] Retraction in Engram is reflected as page `status` within one `sync`, including on locally edited pages; the body of a locally edited page is never rewritten.
- [ ] Install and uninstall across Claude Code, opencode, and codex leave every unrelated MCP entry byte-identical.
- [ ] Every slice stays ≤400 authored lines and `engine/go.mod` stays require-free.
