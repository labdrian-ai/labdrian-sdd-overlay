---
name: inception-pipeline
description: "First skill for start project, bootstrap, feature, chore, fix, hotfix, change request; arrancá el proyecto, empezar proyecto, nueva feature, arreglo chico. Selects tier and enters the role chain. Invoke project-inception directly only for trilogy."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "3.0.0"
---

## Activation Contract

Use first for any work; direct project-inception only for the trilogy.

## Hard Rules

Delegate producers; never execute their work or chain hops. Artifacts are English. Use parent-resolved memorycard capabilities and locators; never ask the storage question. Legacy `engram` is historical: never write or export Engram. Load `../_shared/pre-sdd-contracts.md`.

## Decision Gates

Record the first rule and rationale in `sdd/{change}/pipeline-state`:
1. No `project/{project}/manifest/context`: Tier 1.
2. Verified manifest and architect exist; self-contained single surface, no new architecture, schema, integration or production risk: Tier 3.
3. Same foundation; sequenced feature or cross-cutting work: Tier 2.
4. Ambiguous: default up (3→2, 2→1); read `references/tier-selection.md`; ask one question at most.

## Execution Steps

`change-name` comes once from requirements-from-transcripts; inherit it verbatim. Tier 1: delegate requirements if a transcript/brief exists, then project-inception `inception_mode: full` with optional `requirements_brief_ref: {change}`. Tier 2: requirements, then project-inception `inception_mode: reuse` (read-only foundation; missing one falls back to `full`; roadmap-maker `incremental-insert`). Tier 3: lightweight requirements, then roadmap-maker `incremental-insert`, `tier: 3`, preserving actuals/history.

Delegations carry `model`, unique `(phase, fingerprint)` and skill paths. Builders use strict TDD and the project-instruction or build-file runner; ask once if absent.

## Chain entry

Load `skills/role-chain/SKILL.md`; it owns subsequent hops. Inception writes role-chain records only; it opens no workflow. Only the parent appends with `labdrian roles append --project {project} --goal {change} --chain {change} --stdin`; goal and chain equal `change-name`. First: `shaper → estimator`, or `prototyper → shaper` with `exploration` if a premise is open.

   ```json
   {"version":1,"project_id":"acme-shop","goal_id":"add-wishlist","chain_id":"add-wishlist","seq":1,"from_role":"shaper","to_role":"estimator","prev_sha256":"0000000000000000000000000000000000000000000000000000000000000000","payload_kind":"feature-doc","payload_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","evidence":[],"context":{"summary":"Feature document odd/tasks/add-wishlist.md shaped: 4 tasks, 1 slice.","decisions":[],"open_questions":[]},"status":"completed"}
   ```

The parent names `odd/tasks/<feature>.md`; shaper writes Specs and Tasks; estimator receives its sha256. Replace example IDs and digest; `prev_sha256` has 64 zeros. Builder records progress in its Log and delivery adds its final Log entry in place; `progress` and `close-report` are handoff kinds, not files. Bind closure to it and result/verifier files.

## Closure-Feedback (plan → validate → execute; single writer)

Run `labdrian roles resume --project {project} --goal {change} --chain {change}`. `"terminal": false`, or an error because the chain has no records, means it runs: wait, do not fire. `"terminal": true` is a handoff, not proof of delivery or landing: an unlanded chain is not closed; wait for landing. The parent names the landing commit (merge, squash or fast-forward); closure records it as `landing_commit`. Require the delivery outcome and an explicitly recorded `landing_commit` before t1; else omit t1 and disclose. A tree hash MUST be used to **verify** a commit, never to **discover** one. When the anchor is ambiguous, omit t1; never resolved by position.

Before measuring or writing closure, read `references/closure-feedback.md`. Only closure-feedback writes `sdd/{change}/actuals`; stop on partial writes.

## Output Contract

Report tier, rationale, handoff and closure outcome or pending evidence.
