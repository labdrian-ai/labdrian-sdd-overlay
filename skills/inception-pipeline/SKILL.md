---
name: inception-pipeline
description: "Select the inception tier for a project, feature, chore, or fix; delegate producers, enter the role chain, and run closure after delivery."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "3.0.0"
---

## Activation Contract

Use first for a project, feature, chore or fix. Direct project-inception is for the requested manifest/architect/roadmap trilogy only.

## Hard Rules

Delegate producers; never execute their work or chain hops. Artifacts are English. Use parent-resolved memorycard capabilities and exact locators, without a storage-preference question. Legacy `engram` placement is historical only; never write or export Engram. Load `../_shared/pre-sdd-contracts.md`.

## Decision Gates

Record the first firing rule and rationale in `sdd/{change}/pipeline-state`:
1. No `project/{project}/manifest/context`: Tier 1.
2. Manifest and architect exist; self-contained single surface, no new architecture, schema, integration or production risk: Tier 3.
3. Manifest and architect exist; feature or cross-cutting work needs sequencing: Tier 2.
4. Ambiguous: default up (3→2, 2→1); read `references/tier-selection.md`, ask at most one question only if genuinely ambiguous.

Verify foundation before Tier 2/3; missing manifest forces Tier 1, regardless of phrasing.

## Execution Steps

Requirements-from-transcripts derives `change-name` once; inherit it verbatim. Tier 1: delegate requirements when a transcript/brief exists, then project-inception `inception_mode: full` with optional `requirements_brief_ref`. Tier 2: delegate requirements, then project-inception `inception_mode: reuse` (read-only foundation; missing either falls back to `full`; roadmap-maker runs `incremental-insert`). Tier 3: delegate lightweight requirements then roadmap-maker `incremental-insert`, `tier: 3`, preserving actuals/history. Then enter; close after delivery.

Delegations require `model`, deduplicated `(phase, fingerprint)` and resolved skill paths. Builder prompts require strict TDD and the runner from project instructions or build files; ask once if absent.

## Chain entry

Load `skills/role-chain/SKILL.md`; it owns subsequent hops. Inception writes role-chain records only; it opens no workflow. Only the parent appends with `labdrian roles append --project {project} --goal {change} --chain {change} --stdin`; goal and chain equal `change-name`. First record is `shaper → estimator`, or `prototyper → shaper` with an `exploration` payload if a premise is open.

   ```json
   {"version":1,"project_id":"acme-shop","goal_id":"add-wishlist","chain_id":"add-wishlist","seq":1,"from_role":"shaper","to_role":"estimator","prev_sha256":"0000000000000000000000000000000000000000000000000000000000000000","payload_kind":"feature-doc","payload_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","evidence":[],"context":{"summary":"Feature document odd/tasks/add-wishlist.md shaped: 4 tasks, 1 slice.","decisions":[],"open_questions":[]},"status":"completed"}
   ```

The parent names `odd/tasks/<feature>.md`; shaper writes its Specs and Tasks, estimator receives the same document's sha256. Replace illustrative ids and digest; `prev_sha256` has 64 zeros. Builder records progress in its Log and delivery adds its final Log entry in place; `progress` and `close-report` are handoff kinds, not separate files. Bind closure to that document and result/verifier files.

## Closure-Feedback (plan → validate → execute; single writer)

Run `labdrian roles resume --project {project} --goal {change} --chain {change}`. `"terminal": true` is a handoff, not proof of delivery or merge; otherwise wait. Require the delivery outcome and an explicitly recorded `landing_commit` before t1; disclose absence or mismatch and omit t1. A tree hash MUST be used to **verify** a commit, never to **discover** one. When the anchor is ambiguous, omit t1; it is never resolved by position.

Before measuring or writing closure, read `references/closure-feedback.md` and follow its plan, validation and execution. Only closure-feedback writes `sdd/{change}/actuals`; stop on partial writes.

## Output Contract

Report tier, rationale, handoff and closure outcome or pending evidence.
