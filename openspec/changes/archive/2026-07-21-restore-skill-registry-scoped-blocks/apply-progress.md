# Apply Progress: Restore Skill Registry Scoped Blocks

## Status

Completed on the one authorized automatic retry. The first attempt's verifier defect and verified rollback are retained below; the retry used the exact EOF delimiter envelope and completed the registry-only repair.

## Completed Tasks

- [x] 1.1 Verified the expected Git root and RED-tested a wrong working directory without changing the registry.
- [x] 1.2 Captured direct registry and lock baselines, engine identity, excluded-surface manifest, and pre-existing `.codegraph/` state.
- [x] 1.3–4.2 Completed the fresh RED, narrow restoration, preservation, idempotence, sidecar finalization, health, rollback, and excluded-surface checks on the authorized retry.

## Incomplete Tasks

None.

## Authorized Retry Result

- Fresh baseline accepted: registry SHA-256 `0ddf7fb41f67656138d9ab63fa6ce43decda2e15955ef1bd2c8a0a7a879e497f`, mode `0644`, size `14529`; lock absent; engine SHA-256 unchanged; 96 excluded entries captured.
- RED passed: wrong-cwd root guard rejected `/tmp` without changing the registry; `bin/labdrian-overlay status-hooks` exited `2` and named all three missing scopes.
- GREEN passed: only the three designed propagation commands ran once each, in order; every command exited `0`; marker/row uniqueness and containment passed.
- Preservation passed: repaired bytes equal baseline plus exactly `LF + block(minimalism) + LF LF + block(safety) + LF LF + block(anti-generic) + LF`; all six delimiter LFs are insertion-owned.
- Idempotence passed: the second identical three-command pass produced byte/hash/mode-identical registry content with no duplicate marker or row.
- Finalization passed: the persistent lock sidecar was removed to restore its absent baseline; 96 excluded entries, including pre-existing `.codegraph/`, matched their fresh baseline.
- Health passed: `bin/labdrian-overlay status-hooks` exited `0` without a missing-scope warning.

## TDD Cycle Evidence

| Task | Test File / Harness | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| 1.1 | Ephemeral direct-filesystem root-guard harness | Integration | N/A — generated-state repair only | ✅ Wrong cwd `/tmp` rejected; registry SHA-256 unchanged | ✅ Expected root accepted before baseline capture | ➖ One deterministic root boundary | ➖ None needed |
| 1.2 | Ephemeral direct-filesystem baseline harness | Integration | N/A — generated-state repair only | ✅ Recorded absent lock baseline and missing scopes | ✅ Captured byte copies, SHA-256, modes, engine SHA-256, and 96 excluded entries | ✅ Present/absent sidecar branches represented by baseline metadata | ➖ None needed |
| 1.3 | `bin/labdrian-overlay status-hooks` and direct marker harness | Runtime | N/A — generated-state repair only | ✅ `status-hooks` exited 2 and named all three missing scopes | ❌ Not completed: later preservation invariant failed and rolled back | ➖ Deferred | ➖ None needed |
| 2.1 | Three exact propagator commands | Integration | N/A — generated-state repair only | ✅ Baseline proved all scopes absent | ❌ Commands each exited 0, but the enclosing transaction rolled back after a later invariant failure | ➖ Deferred | ➖ None needed |
| 2.2 | Ephemeral uniqueness/containment verifier | Integration | N/A — generated-state repair only | ✅ Missing-scope baseline recorded | ❌ Uniqueness and containment passed transiently, but no durable GREEN state remains after rollback | ➖ Deferred | ➖ None needed |
| 2.3 | Ephemeral owned-range removal verifier | Integration | N/A — generated-state repair only | ✅ Baseline bytes captured | ❌ Failed: `unrelated registry bytes changed`; rollback completed and was verified | ➖ Deferred | ➖ None needed |

### Retry TDD Cycle Evidence

| Task | Test File / Harness | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| 1.3 | Fresh direct-filesystem and `status-hooks` harness | Runtime | N/A — generated-state repair only | ✅ Exit `2` with all three missing scopes; wrong cwd rejected | ✅ Fresh baseline and absence guards passed before mutation | ✅ Root and missing-scope boundaries | ➖ None needed |
| 2.1–2.3 | Exact three-command transaction and EOF-envelope verifier | Integration | N/A — generated-state repair only | ✅ Fresh baseline had no required blocks | ✅ Three commands exit `0`; uniqueness, containment, exact envelope, mode, and excluded manifest passed | ✅ First pass plus envelope branches | ➖ None needed |
| 3.1–3.3 | Direct idempotence, lock, and `status-hooks` harness | Integration/Runtime | N/A — generated-state repair only | ✅ Repaired snapshot captured before second pass | ✅ Second pass byte/hash/mode identical; absent lock restored; health exit `0` | ✅ First/second pass and absent-sidecar branches | ➖ None needed |
| 4.1–4.2 | Prior rollback receipt plus fresh excluded-manifest comparison | Integration | N/A — generated-state repair only | ✅ First attempt exercised the rollback path | ✅ Retry left registry as the only regenerated surface; 96 excluded entries unchanged | ✅ Failure rollback and successful retry paths | ➖ None needed |

## Test Summary

- **Total persistent tests written**: 0 — this generated-state repair requires design-mandated direct filesystem and runtime evidence; no production source is changed.
- **Direct RED result**: `bin/labdrian-overlay status-hooks` exit `2`; warning named `minimalism-contract-scope`, `skill-discovery-safety-scope`, and `anti-generic-design-scope`.
- **First-attempt transaction result**: all three narrow propagator commands exited `0`; marker/row uniqueness and containment passed before the preservation check failed.
- **First-attempt rollback result**: registry bytes/hash/mode, absent lock state, and all 96 excluded manifest entries matched the baseline.
- **Retry transaction result**: all three narrow propagator commands exited `0`; the exact six-LF EOF envelope, idempotence, lock restoration, excluded manifest, and final `status-hooks` exit `0` passed.

## Work Unit Evidence

| Evidence | Exact result |
|---|---|
| Focused test command | Authorized retry direct-filesystem transaction harness; **passed**. Three exact first-pass commands and three exact second-pass commands exited `0`; uniqueness, containment, exact envelope, idempotence, mode, lock, and 96 excluded entries passed. |
| Runtime harness command/scenario | RED `bin/labdrian-overlay status-hooks`: exit `2` with all three missing scopes. Final `bin/labdrian-overlay status-hooks`: exit `0` with no missing-scope warning. |
| Rollback boundary | `.atl/skill-registry.md` and `.atl/skill-registry.md.lock` only. First-attempt rollback restored registry bytes/hash/mode and the originally absent lock; retry finalization again restored the absent lock while retaining the intended registry repair. |

## Baseline Identity

- Pre-retry registry: SHA-256 `0ddf7fb41f67656138d9ab63fa6ce43decda2e15955ef1bd2c8a0a7a879e497f`, mode `0644`, size `14529`.
- Lock sidecar: absent before the retry and absent after finalization.
- Engine: SHA-256 `6994472991ae54a2c929c978cc703472f97ae187794d10a2fa07de7fe1e7f26b`.
- Excluded surfaces: 96 direct filesystem entries unchanged; pre-existing untracked `.codegraph/` was preserved.

## Deviation and Blocker

The first attempt correctly rolled back when its marker-only preservation verifier failed. Fresh-context diagnosis proved that the propagator owns a deterministic six-LF EOF delimiter envelope; the authorized retry verified that exact envelope without normalization, hand editing, or scope broadening.

## Post-Review Safety Note

The executed absent-baseline lock cleanup occurred under isolated conditions. That cleanup is superseded and MUST NOT be reused: future transactions retain the persistent sidecar path/inode after flock release. This documentation correction did not alter runtime state.

## Delivery Boundary

- Mode: stacked PR slice (`stacked-to-main`), autonomous slice 1.
- Boundary: baseline and direct acceptance evidence for the indivisible registry repair transaction only.
- Review budget: registry-only generated-state repair plus SDD evidence; within the 800-line chained-slice budget.
