# ADR: inception-pipeline hands off to the labdrian role chain (C13-S0)

Status: accepted by the owner on 2026-10-23 (decisions 1a-5a). Supersedes the gentle-ai SDD handoff of `skills/inception-pipeline/SKILL.md` 2.1.0.

## Context

`inception-pipeline` is the first skill of every unit of work. It ends by gating on `gentle-ai sdd-status {change} --cwd {project-root} --json --instructions` and handing the change to the gentle-ai SDD engine. gentle-ai 4.2.0 answers `Error: unknown command "sdd-status"`, and role-agents v3 slice 5 deleted the `sdd-*` agents the skill forwards to (`docs/architecture/role-design/per-role-design-v3.md:93`). Every tier fails at its handoff. The owner chose the labdrian role chain (`skills/role-chain/SKILL.md`) as the handoff target.

## Decision

1. **Rewire.** Delete the gentle-ai SDD tail of the skill: the dispatcher gate, the entry-contract assembly and validation, the SDD-only gates (SDD Init Guard, strict-TDD forwarding to `sdd-apply`/`sdd-verify`, apply-progress continuity, Review Workload Guard, pre-archive receipt check), and the `sdd-time-estimation` step before the shaper (the estimator is a hop of the chain, after the shaper). Add a "Chain entry" section: goal id = chain id = the change-name; the first record is `prototyper → shaper` when a premise is open, otherwise `shaper → estimator`; every builder prompt states that strict TDD is active and names the runner; then load `skills/role-chain/SKILL.md`.
2. **Port.** The role-chain record (`engine/roles/handoff.go`) is the only port inception depends on; `labdrian roles append` is its adapter. The engine does not change.
3. **Closure-feedback is re-anchored, not deleted** (only its inputs change): the chain is finished when `labdrian roles resume` reports it terminal; review results come from `odd/review-results/<feature>/*.json`; `approved_tree` = `git show -s --format=%T <reviewed head>`; t1 = the landing commit's committer time; t0 = the `pipeline-state` `created_at`. The gentle-ai receipt inputs are removed. The actuals schema and validator do not change; roadmap-maker keeps its source of actuals.

## Owner decisions (2026-10-23)

| # | Question | Answer |
|---|---|---|
| 1 | Does inception also open a labdrian workflow (Goal v3, `workflow create`)? | (a) No: role-chain records only. A Goal projection is a separate goal. |
| 2 | Entry contract | (a) Retired: `sdd/{change}/entry` is no longer written; the v2 schema and validator stay as the checker of the archived contracts. |
| 3 | The `--exists-root` doc-walk test | (a) Deleted. |
| 4 | Closure-feedback | (a) Re-anchored to the chain, prose only, no new verb. |
| 5 | `archive-anchor-gate` | (a) History-only for now; a chain adapter is a separate goal. |

## Contract changes and their consumers

- `sdd/{change}/entry` is no longer written. The v2 schema, validator, fixtures and the CI loop over the archived `entry.json` files (`.github/workflows/ci.yml:171-190`) stay byte-identical. Every reader already declares the entry optional.
- The frontmatter description changes to name the role chain. It is a mirrored prompt on four runtimes; the skill version goes to 3.0.0 and the owner re-approves it.
- Closure-feedback inputs change; its output shape does not (consumer: roadmap-maker).
- The validator diagnostic text stays as historical text (accepted debt).

## Slices

| Slice | Scope | Risk |
|---|---|---|
| S1 rewire-handoff | The deletions and the "Chain entry" section; the pins that require the gentle-ai text are deleted; notes in the shared docs; owner approval | item 3 |
| S2 closure-on-chain | The closure-feedback inputs and t1 rewritten; pinned phrases kept where the rule still holds; owner approval | items 3, 6 |

## Rejected alternatives

- A v3 entry contract plus an engine `entry goal` verb: about twice the code, a second validator of one contract, and no reader.
- Writing a v2 entry with a placeholder `expected_native_next_recommendation`: the value would be fabricated.
- A `roles outcome` verb: `roles resume` already reports a terminal chain.
- Deleting closure-feedback: it cuts roadmap-maker off from actuals.
