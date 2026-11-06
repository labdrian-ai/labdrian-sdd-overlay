# Archive Report: longterm-mem-knowledge-ingestion

- **Change**: `longterm-mem-knowledge-ingestion`
- **Archived**: 2026-11-06
- **Archive status**: archived by Phase 9 item C22 (`odd/tasks/phase9-c22-knowledge-ingestion.md`, T6), moved as-is.
- **Verification status**: no verify report; the Phase 3 checks are recorded in `tasks.md` and in `odd/tasks/phase9-c22-knowledge-ingestion.md` (T6 Log entry L30).
- **Implementation status**: the override, reading port, filesystem adapter, single-source extraction and directory scan are implemented. The archived task list has 44 checked tasks and no unchecked tasks; its checklist records unsupported scenarios honestly.

## Cycle Timestamps

| Phase | Timestamp | Notes |
|-------|-----------|-------|
| t1 (landing commit) | n/a | landing_commit is absent: the work of this change reached main through its own earlier pull requests, and this archive moves only its record, so it records no landing commit. |

## Canonical Spec Composition

T5 copied both complete specifications verbatim in commit `a00310b2`:

- `specs/knowledge-ingestion/spec.md` to `openspec/specs/knowledge-ingestion/spec.md`.
- `specs/longterm-mem-ingest-extraction/spec.md` to `openspec/specs/longterm-mem-ingest-extraction/spec.md`.

Both canonical files still match their archived source bytes. Their remaining Engram terminology belongs to the follow-up below; this archive does not rewrite historical specifications.

## Archive Move

`openspec/changes/longterm-mem-knowledge-ingestion/` → `openspec/changes/archive/2026-11-06-longterm-mem-knowledge-ingestion/`, by `git mv` in commit `67bfbd34`; no file of the folder was edited in that commit. It contains exactly six `R100` renames, with no additions or deletions.

The folder preserves `proposal.md`, `exploration.md`, `design.md`, `tasks.md`, and both specification files. Phase 3 completion was recorded separately in `2e1f82f2` before the move; this report is additive.

The recursive snapshot readback `diff -r /tmp/c22-t6.nh2R5w/pre-move openspec/changes/archive/2026-11-06-longterm-mem-knowledge-ingestion` exited 0. Its verbatim stdout and stderr were empty:

```text
```

The independent structural verifier also observed a clean working tree and `Risk: none` on the move. The native exemption is stored in `odd/review-results/phase9-c22-knowledge-ingestion/67bfbd34dbeb-3.exempt.json`; its sweep and the classification of the six historical references are stored alongside it.

## Acceptance Evidence

Go vet and full suites pass in `longterm-mem/` and `engine/` with `GOTOOLCHAIN=go1.26.1`, the CI-selected module toolchain. The initial Go 1.27.3 run failed on toolchain-sensitive JSON golden wording; no golden was changed. Architecture, network allowlist, read-only Engram and registry checks pass.

The agent-owned intake procedure was exercised with the committed binary, a synthetic local text source, isolated memorycard and Engram fixtures, and a scratch vault containing real support scripts. A prepared marker proved scratch reads before the first CLI save; the installed memorycard stored a 20,000-byte probe unsplit. The live store was checked through SQLite read-only access, with no marker and an unchanged memories-table digest. The scratch Engram fixture also remained unchanged.

| Checklist scenario | Observed result |
|---|---|
| R-003 intake and vault query | Pass: save, explicit promotion and MCP query with `sources: ["vault"]` returned the page with its provenance block. |
| R-004 unchanged source | Pass: `unchanged`, zero save/promote calls, zero new versions, one topic key and unchanged page bytes. |
| R-004 changed source | Pass: same memory id, key and page path, exactly one new version, and refreshed page content and digest. |
| OQ-4 implicit sync promotion | Not exercisable for memorycard, accepted under D7: a saved, unpromoted record remained unpromoted; `sync` reported zero promotions and zero patches. |

L30 records the exact command forms, outputs, source key and version-history checks. Temporary fixtures and logs are not shipped artifacts. The current page-refresh result is the final-state evidence for that behavior.

## Follow-up: C22 Memorycard Compatibility

Owner-accepted scope from D7 and D13, recorded in the Phase 9 ledger:

- Add memorycard consumption to `sync`; OQ-4 remains unsupported.
- Migrate once, or add explicit handling for, the ten pre-memorycard vault pages identified in L26. L26/D13 carry the case where re-ingestion saves an imported memory and promotion refuses it as bridged; T6 did not re-ingest those live sources.
- Finish the shared-contract and canonical-spec memorycard wording and the old-origin chunk supersession gap. Carry the remaining non-blocking T7 documentation/test advisories from L21 and L27; the history-count question is resolved by T6's CLI observations.

Other observation outside T6's edit surfaces: `docs/architecture/standalone-product-start-here.md:20` and `docs/architecture/standalone-product-roadmap.md:59` still refer to the old active directory. The sweep found no live code consumer. Those prose references were preserved rather than expanding the archive change.
