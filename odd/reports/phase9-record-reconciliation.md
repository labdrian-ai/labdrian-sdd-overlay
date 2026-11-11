# Phase 9 record reconciliation

Date: 2026-11-09. Status: local, uncommitted documentation reconciliation on `docs/phase9-c12-closeout`, based on `952d4425f21d000b3c3f7902c7566028395d2896`. No remote fetch or publication is claimed.

Owner request: "actualicemos el registros que quedwn solo las que tenemos pendiente revisemos que quede todo en orden para continuar con lo que realmente tenemos pendiente /model". This records the request; no model configuration was changed.

## Current open work

| Entry | Remaining outcome | Sequence and limits |
|---|---|---|
| C13 | Rewrite the custom inception family. | Next work; shape the bounded change against the approved role design and current contracts. Documentation is not the rewrite. |
| C14 | Rewrite the remaining custom planning skills. | Follows C13; do not assume prior reference-page ingestion implements these changes. |
| C16 | Lint and approval for every new or rewritten skill. | Applies to C13/C14 and remains a closing gate, not another role-documentation batch. |
| C22-FU | Existing memorycard compatibility follow-up. | Pending shaping, separate from completed C22. Preserve D7/D13 scope; no new dependency on C13 is inferred. |

C22-FU explicitly includes memorycard consumption by sync, one-time migration or handling of ten pre-memorycard vault pages, shared/canonical wording, old-origin chunk supersession, and the carried T7 advisories. Explicit changed-source promotion was already observed working and is not reopened. Source: `openspec/changes/archive/2026-11-06-longterm-mem-knowledge-ingestion/archive-report.md:43-58`.

## Delivered work removed from the open queue

| Entry | Delivery evidence verified in local Git | Outcome retained |
|---|---|---|
| C25 | PR #716, `c9e9eaf50008b11fe10c8a0c617d34363e87427a` | Experiment NOT MET; prescribed fallback completed. Detailed evidence: `odd/tasks/phase9-c25-review-hunt.md:139-156`. Historical review-mode settings are not current configuration evidence. |
| C26 | PR #724, `eb5ffea641ae4b1e8e33b1ca5802c0aacce02d01` | Experiment NOT MET; owner chose C27 iteration. `odd/tasks/phase9-c26-review-contract.md:167-177`. |
| C27 | PR #731, `835dcd3a99d346920a69292d0f96c256ec4c11dd` | Selected-arm implementation and adoption delivered; S6 MET in the recorded experiment. Conditional T5/S5 was not selected, not implemented, and is not an assignment to resume. `odd/tasks/phase9-c27-review-recall.md:62,89-95` and T7/T8 records retain limitations and delivery history. |
| C28 | PR #736, `e8e55b219a8f121de4afda29d0dcfc30352d6dfd` | Four tasks complete; precision baseline, not an invented pass threshold. `odd/tasks/phase9-c28-review-precision.md:207`. Nonblocking suggestions remain in its log. |
| C22 | PR #751, `952d4425f21d000b3c3f7902c7566028395d2896` | Extraction, acceptance accounting and archive delivered; unsupported sync is retained separately in C22-FU. |
| C12 | Local closeout; `odd/reports/phase9-c12-documentation-evidence.json` | All 43 registered skills and eight roles documented, promoted and query-verified. No publication or native workflow closure inferred. |

Each listed merge is an ancestor of the checkout's HEAD, checked with `git merge-base --is-ancestor`. This is local delivery evidence, not a fresh remote CI inspection. Negative experiments, historical failures, accepted limitations and conditional tasks remain readable in their original detailed logs.

## Record boundaries

- The Tasks register and the canonical roadmap's Phase 9 row now agree on four open entries. C22-FU labels already accepted scope instead of hiding it in a paragraph or creating a new requirement.
- The master roadmap remains in progress. Phase 10/11 and the product's other residual acceptance rows are not closed by this reconciliation.
- Earlier progress entries remain historical snapshots, including superseded pending-delivery statements. They are not silently rewritten or used to repeat delivered work.
- The stopped Hermes implementation is outside this queue. Its branches, evidence and changes are preserved. No native workflow or role-chain state transition is claimed by these document edits.
- C12 closeout and this reconciliation remain uncommitted and unpublished. No code, skill contract, approval record, live configuration, deployment, or model setting was changed.

## Verification scope

This is passive documentation work. Verification checks the exact open-ID set, the local merge ancestry, preservation of the historical progress section, scope of modified files, evidence paths, C12 page hashes, and `git diff --check`. No functional suite or review benchmark is rerun or claimed.

Observed checks passed: exactly C13/C14/C16/C22-FU open; all listed merges are local HEAD ancestors; the complete previous progress section remains unchanged before the appended entries; only the Phase 9 roadmap row changed; all 51 C12 page hashes still match. `git diff --check` passed.

The existing Memorycard mirror topic `odd/consolidation-phase9/tasks` was updated through the native CLI, retaining its identity rather than creating a competing ledger. All 35 returned parts were read back in order against the complete current ledger, allowing only documented presentation and whitespace-only split boundaries. The CLI trimmed the final newline; this is not a claim of whole-file byte equality. Embedding was explicitly deferred. Exact receipt, part IDs, hashes, and validation results: `odd/reports/phase9-record-reconciliation-evidence.json`.
