# inception-pipeline hands off to the labdrian role chain (Phase 9, C13-S0)

Objective: make `inception-pipeline` work again by handing off to the role chain instead of the gentle-ai SDD engine. Branch `feat/phase9-inception-chain` from `main` (`5b313073`). Delivery: `ask-on-risk`, one PR per slice. Runners: `cd engine && go test ./...`, `cd tools/entry-contract-validator && go test ./...`, `cd tools/actuals-record-validator && go test ./...`, `labdrian skills validate`. ADR: `docs/architecture/adr-c13-inception-role-chain.md`.

## Specs

- **S1** — Decision 1 of the ADR: `skills/inception-pipeline/SKILL.md` contains none of `gentle-ai sdd-status`, `nextRecommended`, `sdd-apply`, `sdd-verify`, `team-lite`; it contains a "Chain entry" section naming `labdrian roles append` and `skills/role-chain/SKILL.md`; the frontmatter description names the role chain; version `3.0.0`.
- **S2** — Owner decision 1 (a): inception writes role-chain records only; it does not run `labdrian workflow create`.
- **S3** — Owner decision 2 (a): `sdd/{change}/entry` is no longer written; the v2 schema, validator, fixtures and the CI loop over the archived `entry.json` files are byte-identical.
- **S4** — Owner decision 3 (a): the `--exists-root` doc-walk test (`tools/entry-contract-validator/evolution_test.go:626-667`) is deleted, together with the pins that require inception's gentle-ai text (`main_test.go:437-447`, `token_domain_test.go`).
- **S5** — Owner decision 4 (a): closure-feedback reads `labdrian roles resume`, `odd/review-results/<feature>/*.json`, `git show -s --format=%T <reviewed head>`; it no longer names `gentle-ai.review-receipt` or `review-transactions/v2`; the actuals schema and validator are unchanged.
- **S6** — Owner decision 5 (a): `tools/archive-anchor-gate` is unchanged.
- **S7** — The owner re-approves the skill (`skills approve`); `skills validate` passes.

## Tasks

| ID | Specs | Route | Status | Commit / evidence |
|---|---|---|---|---|
| T0 | — | inline | done | ADR and this document |
| T1 | S1-S4, S6, S7 | delegated writer (ultracode, owner L1) + independent verifier (item 3) | open | |
| T2 | S5, S7 | delegated writer + independent verifier (items 3, 6) | open | |

## Log

- **L1** (2026-10-23, owner): "ataquemosla en ultracode"; earlier standing instruction: "ve en ultracode con opus high para las verify y sonnet max para implementaciones".
- **L2** (2026-10-23, owner): handoff target "A" (the labdrian role chain); design decisions 1-5 answered "a" each.
- **L3** (2026-10-23): design workflow `wf_7c6ebbb5-ced` (3 readers, 2 designs, judge; 3 claims corrected against `5b313073`); minimal rewire chosen; verified `gentle-ai sdd-status` fails on gentle-ai 4.2.0.
