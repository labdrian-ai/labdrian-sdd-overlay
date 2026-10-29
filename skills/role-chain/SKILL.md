---
name: role-chain
description: "Parent duties for running a goal through the labdrian role agents. Trigger: the orchestrator runs the role chain."
disable-model-invocation: true
license: MIT
metadata:
  author: labdrian
  version: "1.0"
---

Chain: `[prototyper] → shaper → estimator → builder → [sweeper] → [polisher] → reviewer → delivery`.

## Appends

- Pipe records to `labdrian roles append --project P --goal G --chain C --stdin`; `payload_kind`: `exploration`, `feature-doc`, `progress`, `review-report`, `verify-report`, `close-report`.
- `shaper → estimator`, `estimator → builder`: same payload digest.
- `partial` or `blocked` envelope: record `interrupted` with `resume_reason`.

## Triggers

- Prototyper, only for a question of shape: does installed tool X behave as premise P says; what is number N; can X be done within the appetite. No premise: back to the asker. The prompt sets the lookup/spike box.
- Sweeper, on the builder's commit (`base` = parent): `labdrian review sweep --feature F --base B --head H` runs T1-T3 (`labdrian --help`); if it fires, launch the sweeper on its candidates, store its report with `labdrian review record --role sweeper`; if the `Risk:` trailer names an item, store each polisher judge's findings with `labdrian review record --role polisher`.
- Reviewer: add a readability row when docs, public API, messages or help change, a second blind launch (another `model`) when the Risk line names an item.

## Worktrees

- Spikes: `git worktree add --detach ~/labdrian-sdd-overlay-worktrees/spike-<goal>`; reviews: `review bind` makes it. Never under the repository or the scratchpad; remove after the verdict.
- The prompt names one path an agent works in; the delivery writes only the paths it names. Pass Engram `project`.

## Binding

- `labdrian review bind --base B --head H` prints `base`, `head` (full SHAs), `diff_sha256`, `paths`, `worktree`. Send `reviewer` the diff (up to 150 KB, else one launch per commit; never truncated), those, the S# criteria and the row sentences copied verbatim from `roles/reviewer/rows.json` (`default` always).
- `labdrian review record --feature F --base B --head H` (stdin; `--rows R`) stores `odd/review-results/<feature>/<head12>-<n>.json`, prints the record to fill; evidence: `review-diff`, `review-result`, `verifier-report`. Exit 2 (`candidate` or `paths` differ, `inspection.status` is not `completed`, or any other rule of the reviewer's contract breaks): record `interrupted`, `resume_reason: review not run: <reason>`; retry once with another `model`. Delivery recomputes `diff_sha256` and trusts the record only on equality.
- A severe finding blocks only if introduced, activated or worsened and reproduced (`unknown`: on `base` first) by `verifier` or you within 3 calls: `reviewer → builder` (the only backward edge), the file as ledger. WARNING: fix before delivery; SUGGESTION: carry. Else `reviewer → delivery`.

## Persistence

You persist the prototyper's and sweeper's artifacts; others keep saving.
