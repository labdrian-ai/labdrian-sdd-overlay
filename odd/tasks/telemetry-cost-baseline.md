# ODD Task — Telemetry: cost baseline dashboard (own-system Phase 1)

Objective: a read-only `telemetry/` Go module that turns agent session logs into a reproducible cost/efficiency report, the measurement base for the own-system relaunch.
Branch: `feat/telemetry-cost-baseline` from `main` `b1055816` (worktree `~/labdrian-sdd-overlay-worktrees/telemetry`). Delivery: `stacked-to-main`, slices of about 400 authored lines.
Runner: `go test` from `telemetry/`. Checks: `gofmt -l .`, `go vet ./...`, `go test -count=1 ./...`; staticcheck and deadcode as in `ci.yml` when the toolchain is available offline. TDD strict.

## Specs

- **S1** (user 2026-10-21, verbatim): "un módulo `telemetry/` dentro del overlay, en una rama de trabajo". The module is read-only over the agent session logs: Claude Code `~/.claude/projects/**/*.jsonl` (main and `subagents/` files) and Pi `~/.pi/agent/sessions/*/*.jsonl`. It never writes to, moves or edits those logs, and "no toca nada existente" in this repository.
- **S2** (user, verbatim): "Convertiría en tablero reproducible el análisis que hice a mano (costo por llamada, por fase y por modelo, y costo por cambio entregado)." The report covers, at least: calls, context tokens (fresh input, cache read, cache write), output tokens, the cache-read share, context tokens read per output token, context-per-call percentiles, split main thread vs subagent, and per model, per project and per day.
- **S3** Cost. Pi logs carry a `cost` field and are reported as recorded. Claude Code logs carry no cost: the report shows tokens only and says cost is unavailable unless the user supplies a price table; it never invents a price. *(assumption: price table is an optional JSON file chosen by the user.)*
- **S4** "por fase": attribution to roles. Subagent `*.meta.json` records `agentType`, `description`, `model` and `requestShape`; the report groups subagent calls by `agentType`. *(assumption: phase = agent role until a later task defines a finer mapping.)*
- **S5** Privacy. Output contains aggregates, ids and counts only; never prompt, response or tool content. No network access.
- **S6** Deterministic and reproducible: the same input logs produce byte-identical output (human table and `--json`). Streaming read, bounded memory; the 2.1 GB corpus measured by hand in about 10 s with a throwaway script.
- **S7** Repository conventions: own `go.mod` (`github.com/labdrian-ai/labdrian-sdd-overlay/telemetry`, `go 1.21`, standard library only), own CI job like `tools/*`, fixtures under `testdata/`, README. Engine and longterm-mem are not imported.
- **Deferred, not in this feature** — "costo por cambio entregado": linking sessions to delivered changes (Claude records carry `gitBranch`). Needs a user definition of "delivered change" (merged PR? commit on `main`?). Tracked as open decision D1.

## Baseline to reproduce (hand analysis 2026-10-21; will drift as logs grow)

Claude Code: 84,977 calls; 23.63B context tokens (97.7% cache read); 78.7M output; main 23,848 calls (ctx/call avg 432k), subagents 61,129 calls (avg 218k, p5 52k). Pi: 2,796 calls, $73.67. Acceptance for S2 is on fixtures with known totals, not on the live corpus.

## Tasks

| ID | Specs | Route | Status | Commit |
|---|---|---|---|---|
| T1 Normalized `Call` record and Claude Code + Pi log readers (dedupe by message id, main vs subagent, `agentType` from meta.json) | S1 S3 S4 S5 S6 | delegated writer | done (verified) | see git log |
| T2 Aggregation: totals, ratios, percentiles, groupings by model/project/day/role | S2 S4 | delegated writer | done (verified) | see git log |
| T3 CLI `telemetry report` (table and `--json`), golden fixtures, determinism test | S2 S5 S6 | delegated writer | done (verified) | see git log |
| T4 Cost: Pi `cost` as recorded, optional price table, explicit "unavailable" otherwise | S3 | delegated writer | done (verified) | see git log |
| T5 Role/phase attribution report | S4 | delegated writer | pending | |
| T6 CI job, README, ADR, close | S7 | inline | pending | |

Route declaration: T1–T5 are dependent units written one at a time. Writer trigger evidence: the runtime signalled the context backstop (parent context far past 150k tokens). T6 is small and inline. Each writer gets `## Allowed edit surfaces` `telemetry/**` only.

## Open decisions

- **D1** Definition of "delivered change" for cost per change (after T5).
- **D2** Whether `telemetry` becomes a `labdrian` verb or stays a standalone binary (default: standalone, like `tools/*`).

## Log

- **L1** (user, 2026-10-21): "bien vamos a arrancamos por ahi pero te tenes que mover al proyecto cierto"
- **L2** (user, earlier, goal): "mejorar todo o que tenemos hacerlo mas eficiente mas productivo mas rapido mas beneficioso con menos recuersos para construir software a medida cada vez mejor realizar a menor costo"
- **L3** (parent, 2026-10-21): main checkout `~/labdrian-sdd-overlay` is on `feat/phase9-b62` and another session committed there minutes earlier, so it is left untouched; `~/labdrian-standalone` is 1,375 commits behind `main`. Work happens in its own worktree from `main`.
- **L4** (parent, 2026-10-21): T1 verified. Writer reported RED/GREEN, gofmt/vet/build/test clean. Parent re-ran the suite and ran the readers over the real corpus from a scratch module outside the repo (aggregates only): Claude 84,977 calls, 23.63B ctx, main 23,848 / sub 61,129, ctx/call 432,138 / 217,999 and 7.8 s: identical to the hand analysis. Pi gave 2,828 calls / $78.86 vs 2,796 / $73.67 by hand: unexplained difference, probably the live session growing since; not verified. Diff is 520 lines (target ~400), accepted by the parent. Writer could not save to Engram (server too old).
- **L5** (parent, 2026-10-21): T2 verified. Writer: RED/GREEN, gofmt/vet/build/test clean, 374 lines, Risk none. Parent re-ran tests and fed the real corpus (87,831 calls) through Aggregator from a scratch module outside the repo: two runs byte-identical (S6), ~9 s, 95 KB JSON with by_source/role/model/project/day/agent_type. One call with 3,083,209 context tokens seen in the distribution max: not investigated.
- **L6** (parent, 2026-10-21): T3 verified. Writer: RED/GREEN, 438 lines, Risk item 1 (scratch dir, cleaned by parent). Parent re-ran the suite and ran the binary on the real corpus (aggregates only): exit codes 0/1/2 as specified. Two consecutive runs on live logs differed once because logs grew (Pi 2,885 -> 2,894 calls); with no growth stdout was byte-identical; fixtures determinism test covers S6.
- **L7** (parent): first findings from the report: `general-purpose` subagents hold 38.8% of all context tokens (33.2k calls, p5 74k) vs `sdd-apply` 9.3% and `sdd-verify` 1.2%; main thread 44.7%; the max single-call context is 3.08M tokens (not investigated). Known limitation: readers use filepath.WalkDir, so symlinked project directories inside a root are not followed (default roots are real directories); candidate follow-up.
- **L8** (parent, 2026-10-21): T4 verified. Writer: RED/GREEN, Risk item 3 (shared Totals/JSON/table). Parent differential on the real corpus, previous binary (a401a38a) vs new, without --prices: stdout identical for `report --top 4`, `--json`, `--top 1`; stderr identical modulo live-growth counters; exit codes equal. Placeholder table (1/1/1/1, not committed) confirmed coverage reporting: 43,413 estimated, 41,570 unpriced calls; invalid table and missing file exit 1. No real prices are shipped; the user must supply them.
