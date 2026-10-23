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
| T1 | S1-S4, S6, S7 | delegated writer (ultracode, owner L1) + independent verifier (item 3) | applied; independent verify and S7 (owner `skills approve`) pending | `6d268d40`. RED on `5cfa5ed1`: 6 of 6 subtests of `TestInceptionPipelineEntersTheRoleChain` failed. GREEN: 6 of 6 pass. |
| T2 | S5, S7 | delegated writer + independent verifier (items 3, 6) | open | |

## Log

- **L1** (2026-10-23, owner): "ataquemosla en ultracode"; earlier standing instruction: "ve en ultracode con opus high para las verify y sonnet max para implementaciones".
- **L2** (2026-10-23, owner): handoff target "A" (the labdrian role chain); design decisions 1-5 answered "a" each.
- **L4** (2026-10-23, owner, during T1 launch): "la prespec-malandra hay que revisarla porque funciona como la mierda tambien". Out of this feature's scope (C13 inception family, its own review); recorded here so it is not lost.
- **L3** (2026-10-23): design workflow `wf_7c6ebbb5-ced` (3 readers, 2 designs, judge; 3 claims corrected against `5b313073`); minimal rewire chosen; verified `gentle-ai sdd-status` fails on gentle-ai 4.2.0.
- **L5** (2026-10-23, builder): T1 applied in `6d268d40` on `feat/phase9-inception-chain` (base `5cfa5ed1`).
  - Test first: `engine/skills/inception_chain_entry_test.go` (S1, S2, S3 against the real skill). RED on the unchanged skill: 6 of 6 subtests failed (the five forbidden strings, no `## Chain entry`, description without "role chain", version `2.1.0`, `validate-entry-contract`/`--exists-root`/`entry-contract.schema.json` present). GREEN after the rewrite.
  - Skill 2.1.0 to 3.0.0: deleted the dispatcher gate, the entry-contract assembly and validation, the SDD Init Guard, strict-TDD forwarding, apply-progress continuity, the Review Workload Guard, the pre-archive receipt check, the version-matched-entry and artifact-truthfulness gates, the `sdd-time-estimation` step and three gotchas; added `## Chain entry`; kept the model, launch-dedup and skill-resolver gates. Closure-Feedback is byte-identical to `5cfa5ed1` except one sentence that pointed at the deleted pre-archive gate item (now "see `tools/archive-anchor-gate` (`--change`)").
  - Pins deleted (S4): the inception version and dispatcher pins in `main_test.go`; `token_domain_test.go` whole (both tests, helpers used nowhere else); the `--exists-root` doc walk in `evolution_test.go` with its three helpers and the `fmt`, `io/fs`, `regexp` imports. No actuals or shelltest pin needed an edit: they pin closure-feedback, which still holds.
  - Shared docs: `pre-sdd-contracts.md` and `sdd-orchestrator-workflow.md` say the entry is retired (pinned strings untouched); `tier-selection.md` loses the estimation and slim-slice steps. `skills.registry.yaml` does not carry the description; `.atl/skill-registry.md` (gitignored) already mirrors it.
  - Byte-identical (S3, S6): `git diff HEAD` over the schemas, validator code and fixtures, `tools/archive-anchor-gate`, `tools/actuals-record-validator`, `.github` is empty.
  - Checks: engine suite fails in 4 packages. `goal`, `memoryscope` and `settings/settingsfile` fail identically on `5cfa5ed1` in a detached worktree (Go 1.27.3 json error text, BOM char). `skills` fails 2 tests, both `[APPROVAL_STALE] inception-pipeline` (record `6e244eff...`, file `fc71cc87...`): `TestRegistryLockE2E_ValidateLeavesTheRealTreeUntouched`, `TestApprovalBaseline_PinnedToTheRepositoryRegistry`. Validator modules pass. `labdrian skills validate` exits 1 with that one finding (base: exit 0). S7 stays with the owner.
  - Left for T2 or outside this surface: the closure section still names SDD-engine inputs and `sdd/{change}/entry` (the `realized_slices` bullet); `README.md:168` still lists the skill in bundle 2.1.0; a comment in `pipeline_vocabulary_pins_test.go:22` cites the deleted token-domain test.
