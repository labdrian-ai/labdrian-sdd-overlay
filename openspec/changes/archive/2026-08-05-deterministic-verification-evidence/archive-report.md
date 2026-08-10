# Archive Report: deterministic-verification-evidence

**Change**: `deterministic-verification-evidence` · **Project**: labdrian-sdd-overlay · **Artifact store**: hybrid

**Shipped**: 2026-08-05 · **Archived**: 2026-08-10 (remediation — see below)

## Why this report is dated later than the change

This change merged to `main` on 2026-08-05 and then **was never archived**. Its folder sat in `openspec/changes/` for five days while the change was, by every other measure, complete: 82/82 tasks, issue #120 auto-closed, zero open PRs.

The archive folder is dated **2026-08-05**, the date the work actually landed, not 2026-08-10 when this report was written. Dating it by the bookkeeping date would imply the change completed on 08-10, which is false, and would sort it out of order with the work it shipped alongside.

The gap was found during a calibration audit and is recorded in `project/labdrian-sdd-overlay/actuals-gap-ledger`.

## What shipped

Replaces narrated verification judgment with deterministic machine output:

- `labdrian-overlay deterministic-checks <normalize|check>` — a Go runner under `tools/` executing the hardcoded v1 set (`gofmt`, `go vet`, `staticcheck`, `deadcode`), emitting one `tool | exit_code | summary` row per check. `check` never mutates, so it is legal after candidate freeze; its stdout pipes verbatim into `gentle-ai review capture-evidence --input -`.
- `labdrian-overlay review-preflight [--base-ref REF]` — the empty-candidate guard, failing closed to exit 3.
- `skills/sdd-verify/SKILL.md` carries the mechanical exit-code → `--outcome` table; any unmapped code falls to `procedural_tooling_failed`, never `passed`.
- CI gains `staticcheck` (blocking) and `deadcode` (non-blocking).
- `openspec/config.yaml` `test_command` walks `tools/*/go.mod`, so a new module under `tools/` is no longer invisible to SDD.

**Delivery**: 9 PRs — #121–#128 as slices into the tracker branch, #129 as the tracker roll-up into `main`, merged as `be2c3ca`. Landed diff 51 files, +5767/−113. CI green on every job of the tracker PR.

## Specs promoted

Three **new** capabilities. No canonical spec existed for any of them, so promotion was a straight copy — each promoted file is byte-identical to its delta, verified with `cmp`:

| Capability | Requirements | Bytes |
|---|---:|---:|
| `deterministic-check-runner` | 6 | 3964 |
| `deterministic-verification-policy` | 6 | 4159 |
| `verification-evidence-capture` | 7 | 7623 |
| **Total** | **19** | |

19 matches the requirement count independently reconstructed during the calibration audit.

## Review gate — read this carefully

`reviewGate.result` is **not** `allow`, and no receipt is bound to this change.

The 9-PR chain produced multiple review lineages, and **nothing records which lineage belongs to which PR**. Receipts carry a `paths_digest`, not a path list, so attribution cannot be recovered by inspection either. Binding an arbitrary approved lineage would fabricate provenance.

Instead, **this archive candidate carries its own review**, and that receipt attests to exactly one thing: that the promotion and move recorded here were reviewed. **It does not attest that the original 9-PR chain was reviewed under a bound receipt.** That provenance is permanently lost and is recorded as lost rather than papered over.

Verification evidence for the shipped work does exist and was green at the time: every job of the tracker PR passed, and the change's own runner reports `gofmt 0 / go vet 0 / staticcheck 0`.

## Closure

`sdd/deterministic-verification-evidence/actuals` **cannot be written**. All four hour fields required by `actuals-record.schema.json` depend on phase-level timestamps that were never captured, and closure-feedback fires after archive — it was never reachable for this change. What is recoverable (`requirement_count` 19, `changed_lines` ≈5880, tasks 82/82, durable checkpoint floor 1 from pipeline-state obs #2688) is recorded in `project/labdrian-sdd-overlay/actuals-gap-ledger` instead, where it cannot be mistaken for measurement.

Effective calibration sample size remains **n=2**.

## Root cause, and why it is not a closure-feedback bug

Two other changes closed without actuals because closure-feedback simply never fired after a normal archive. **This one is different**: the cycle was abandoned between merge and archive, so closure-feedback was unreachable by construction.

Hardening closure-feedback would not have caught this. The gap needs a check that reconciles *merged* against *archived*.

## Follow-ups carried forward

- `engine/skills/ondisk.go`'s functions had no non-test callers when this change shipped — closed later by PR #130.
- CI's `shellcheck … || true` can never gate; `bash -n` is the real gate in that job. Still open.
- Five WARNING/SUGGESTION follow-ups from the #128 review (obs #2764).
- Two upstream reports pending a decision: `sdd-apply` committing as it goes, and the `retry-final-verification` dead end of obs #2668.

**Related**: landing obs #2767, entry contract obs #2690, requirements obs #2685, pipeline-state obs #2688, gap ledger `project/labdrian-sdd-overlay/actuals-gap-ledger`.
