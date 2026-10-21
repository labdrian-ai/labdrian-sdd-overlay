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
| T1 Normalized `Call` record and Claude Code + Pi log readers (dedupe by message id, main vs subagent, `agentType` from meta.json) | S1 S3 S4 S5 S6 | delegated writer | pending | |
| T2 Aggregation: totals, ratios, percentiles, groupings by model/project/day/role | S2 S4 | delegated writer | pending | |
| T3 CLI `telemetry report` (table and `--json`), golden fixtures, determinism test | S2 S5 S6 | delegated writer | pending | |
| T4 Cost: Pi `cost` as recorded, optional price table, explicit "unavailable" otherwise | S3 | delegated writer | pending | |
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
