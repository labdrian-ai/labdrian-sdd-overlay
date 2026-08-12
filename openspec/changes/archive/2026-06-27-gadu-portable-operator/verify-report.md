```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:64988cb0d3daec5c775c460b352c2d9b1a5fda6a6392a4c4f6288f7dd7343bb9
verdict: fail
blockers: 0
critical_findings: 0
requirements: 12/13
scenarios: 30/31
test_command: cd engine && go test -count=1 ./... && cd ../tui && go test -count=1 ./...
test_exit_code: 0
test_output_hash: sha256:53d5ec7e8297dcf637aa6949c5dbccc3dd3b65748749364229621ef3e6eec81a
build_command: cd engine && go build ./... && go vet ./... && cd ../tui && go build ./... && go vet ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: gadu-portable-operator | **Mode**: Strict TDD
**Repository revision**: `c94afe564bae18b6ec0f8432f724875dd6748856` (branch `chore/archive-stranded-changes`)
**Nature**: Verify pass 5 — adjudicates whether verify pass 4's single CRITICAL is genuinely closed by apply unit 4.
**Artifact store**: hybrid — Engram `sdd/gadu-portable-operator/verify-report` + `openspec/changes/gadu-portable-operator/verify-report.md`.

### Executive result

**0 CRITICAL, 4 WARNING, 3 SUGGESTION.** Counts move 11/13 -> **12/13 requirements** and hold at **30/31 scenarios**.

The pass-4 CRITICAL is **closed**. It was verified from the repository, not accepted on assertion: the AC-5 restoration test now exists, its assertions were empirically proven to depend on the behaviour they claim to pin, and the characterization label was checked against commit history rather than trusted.

The requirement count rises by one because **evidence was created**, not because the standard was relaxed. Pass 4 lowered the count from 12 to 11 by finding that R-008's "restored by `cmd_apply`" obligation had zero runtime evidence. That obligation now has direct, non-vacuous runtime evidence. The same obligation-derived standard that cost a requirement in pass 4 returns it in pass 5.

### Build & Tests

Both module roots run separately, caching disabled, each from its own module directory — never from the repository root.

| Command | Exit | Result |
|---|---|---|
| `cd engine && go test -count=1 ./...` | 0 | 10 packages ok, 0 fail |
| `cd tui && go test -count=1 ./...` | 0 | 1 package ok |
| `go build ./... && go vet ./...` (both roots) | 0 | empty output (canonical empty sha256) |

The AC-5 test was run with `-v` to prove PASS rather than SKIP, since every overlay integration test in that package is guarded by `testing.Short()`:

```
=== RUN   TestSyncCheck_DetectsMissingAgentFile
--- PASS: TestSyncCheck_DetectsMissingAgentFile (0.47s)
ok  github.com/labdrian-ai/labdrian-sdd-overlay/engine/installer 0.468s
```

### 1. Is the pass-4 CRITICAL closed? YES — on three independent checks

Pass 4's CRITICAL was that `docs/e1-durability-probe.md` claimed an AC-5 regression test was "exercised in `engine/installer/route_test.go`" when no such test existed.

**(a) The test now exists.** `engine/installer/route_test.go:883-903` adds a restoration phase to `TestSyncCheck_DetectsMissingAgentFile`: after the pre-existing removal + `OVERLAY_NOT_DEPLOYED` assertion, it re-runs `apply --target all`, `os.Stat`s `~/.claude/agents/GADU.md`, and re-runs `sync-check --target claude` asserting the absence of `OVERLAY_NOT_DEPLOYED: ` and the presence of `IN_SYNC`.

**(b) The document now describes reality.** `docs/e1-durability-probe.md:75-98` strikes through the original false sentence (preserving it, per the file's existing correction convention) and adds a correction section naming the specific test, the exact command, and the PASS result. Every factual claim in that section was checked: the test exists, the command is the one that runs it, and it passes. One residual overstatement is recorded as WARNING-1 below.

**(c) It is not merely present — it is load-bearing.** See §2.

### 2. Is the new test non-vacuous? PROVEN BY NEUTRALIZATION

A test that would pass whether or not the behaviour works is worthless, so this was settled empirically rather than by reading. The repository's own test file was **not** modified. `engine/` and `bin/` were copied to a scratch directory outside the repository, and the scratch copy's `cmd_apply` deploy loop was mutated to simulate the exact bug class the AC-5 claim rules out — idempotence implemented as a persisted "already applied" ledger, which still performs the first deploy but refuses to restore a destination deleted afterwards.

| Run | Deploy loop | Result |
|---|---|---|
| Baseline (scratch, unmutated) | real | **PASS** |
| Neutralized (scratch, ledger bug) | restoration disabled | **FAIL** |

Under neutralization the test failed with exactly the assertions that encode the restoration claim:

```
route_test.go:891: agent file not restored at .../.claude/agents/GADU.md after reapply
route_test.go:899: agent file still reported OVERLAY_NOT_DEPLOYED after reapply
```

The first-deploy assertions still passed under the mutation, which confirms the experiment isolated the restoration half specifically rather than breaking the test wholesale. **The assertions genuinely depend on the restoration happening.**

The same experiment also exposed a real weakness, reported as WARNING-4: the third assertion, `strings.Contains(outRestored, "IN_SYNC")` at `route_test.go:901-902`, did **not** fire under neutralization. `sync-check --target claude` prints `IN_SYNC` for other manifest rows, so that assertion is satisfiable while `GADU.md` is absent. It is near-vacuous on its own. It is not blocking, because its two sibling assertions carry the proof, but it must not be cited as evidence.

### 3. Was the test honestly labelled? YES — the behaviour provably predates the test

The claim under audit is that this is a characterization test that passed on first run with no fabricated RED, because the behaviour already existed.

| Check | Evidence | Verdict |
|---|---|---|
| Behaviour predates the test | `git blame -L 730,736 bin/labdrian-overlay` -> all six lines authored by commit `43fdcb1d`, **2026-06-25** | CONFIRMED |
| Production code untouched by this unit | `git status --short bin/labdrian-overlay` is empty; only `engine/installer/route_test.go` and `docs/e1-durability-probe.md` are modified | CONFIRMED |
| No RED could have been manufactured | the test change is an **uncommitted** working-tree edit; there is no commit in which a temporarily-broken implementation or deliberately-failing assertion could have been staged | CONFIRMED |
| Deploy loop has no "already applied" ledger | `bin/labdrian-overlay:730-736` — copies whenever `dest` is absent **or** `diff --brief` differs; consults no persisted state | CONFIRMED |

The label matches reality. A RED phase was not quietly manufactured, and none is claimed anywhere in the apply record. Under Strict TDD this is the correct classification and the correct disclosure — the same standard units 1-3 applied to their own spec-only work.

### 4. Compliance summary — honest counts

**12/13 requirements. 30/31 scenarios. 0 FAILING.**

Totals re-derived from the retrieved specs this pass, not inherited: 4 + 3 + 6 = **13** requirements; 9 + 8 + 14 = **31** scenarios. Identical to passes 3 and 4, so nothing was lost or renumbered.

| Requirement | Scenarios | Status |
|---|---|---|
| R-001, R-002, R-003, R-004, R-006, R-007, R-009, R-010, R-011, R-012 | 24 | COMPLETE |
| R-005 | 3 | COMPLETE (MANUAL accepted, corroborated by format + placement tests) |
| **R-008** | 2 | **COMPLETE — restoration obligation now has non-vacuous runtime evidence** |
| **R-013** | 1 | **INCOMPLETE — E1's `gentle-ai upgrade` half never executed; the clobbers/does-not-clobber result its THEN demands does not exist** |

**Why R-008 moves to COMPLETE, stated explicitly so the change is auditable.** Pass 4 applied an obligation-derived standard and found four of R-008's five obligations evidenced, with "restored by `cmd_apply`" unevidenced. Re-checked against that same standard:

| Obligation | Status this pass |
|---|---|
| registered as overlay-managed | PROVEN (`overlay.manifest:78-80`, route-resolve tests) |
| drift detected by `cmd_sync_check` | PROVEN (`TestSyncCheck_DetectsMissingAgentFile`, removal half) |
| deployed to overlay-managed destinations | PROVEN (`TestApply_AgentsLandInNativeAgentDirs` + `TestRouteResolve_GADUSkillRow`) |
| **restored by `cmd_apply`** | **PROVEN — new restoration half, empirically non-vacuous (§2)** |
| re-deployable after any `gentle-ai sync` / `gentle-ai upgrade` | DISCHARGED by exhaustion over destination states: restoration is proven from an *arbitrary* destination state (absent), and the deploy loop is one generic branch over `all_tracked_files` with no per-route special-casing, so no property of the mutating command needs to be observed |

This is the closure pass 4 itself specified as Option B. Granting it is not leniency; refusing it after the prescribed evidence was produced would be the mirror-image error of inflation.

**R-013 is counted as NOT satisfied**, per honest accounting. The maintainer's decision to record it as a documented historical deviation rather than promote it is noted and not re-litigated; that decision governs the archive report's disposition, not this pass's arithmetic. Reporting 13/13 or 31/31 here would be exactly the inflation passes 2, 3 and 4 refused.

### 5. Task completion

**17/17 tasks checked, 0 unchecked** (`openspec/changes/gadu-portable-operator/tasks.md`). Task text matches code state, re-confirmed this pass: A6 manifest rows present at `overlay.manifest:78-80`; B1/B2 route functions and B3-B7 five-command wiring present in `bin/labdrian-overlay`; B0's `engine/installer/route_test.go` present and passing.

### 6. R-006 route domain — CONFIRMED as follow-up, prior judgement UPHELD

Re-derived independently, not inherited. `bin/labdrian-overlay` branches on `opencode-agent` at `:318`, `:359`, `:385`, `:405`. Manifest route-column tally: `agent` x2 (`:78`, `:79`), `opencode-agent` x1 (`:80`). The real domain is `{skill, agent, opencode-agent}`; R-006 enumerates two of three.

**Confirmed as follow-up, not a blocker.** R-006's normative force is that the router SHALL route `skill` and `agent` correctly, and it demonstrably does — all three of its scenarios pass, and `TestRouteResolve_GADUOpenCodeAgentRow` additionally covers the unenumerated third route. The enumeration is an incomplete *description*, not a `SHALL only accept` prohibition, so nothing normative is violated and no behaviour contradicts the text. That is precisely the line R-009 crossed and R-006 does not. One-word widening at promotion time.

### 7. Promotion readiness

`openspec/specs/{gadu-canonical-source,gadu-generator,overlay-agent-route}/spec.md` are all absent — no capability-name collision. Requirement IDs are file-local — no ID collision.

| Capability | Promotable verbatim | Note |
|---|---|---|
| `gadu-canonical-source` | YES | 4 requirements, 9 scenarios. R-005's MANUAL is a legitimate, corroborated declaration. |
| `gadu-generator` | YES | 3 requirements, 8 scenarios. Untouched since HEAD. |
| `overlay-agent-route` | YES, **except R-013** | R-008's previously-unbacked restoration clause is now backed. R-013 must not be promoted verbatim — see below. |

R-008's clause is no longer the promotion blocker pass 4 identified. **R-013 remains one**: its text asserts a process event (`gentle-ai upgrade` executed, result recorded) that did not happen and now never can, so promoting it verbatim would write a false process record into the permanent spec. The maintainer has already elected to record it as a documented historical deviation in the archive report instead of promoting it; that disposition resolves this item.

### Issues Found

**CRITICAL**: **None.** (0 — this is the gate the archive phase reads.)

**WARNING**

1. `docs/e1-durability-probe.md:96-98` restates AC-5 as "simulated sync -> reapply -> **both artifacts** present + managed" and claims it is exercised by `TestSyncCheck_DetectsMissingAgentFile`. The test exercises **one** artifact — the claude agent file — and asserts no managed-ness directly. The sandbox fixture manifest (`route_test.go:669-675`) contains no `gadu-operator/SKILL.md` row, so the skill artifact is never removed or restored in any test. The core claim (a real AC-5 restoration test exists and is named) is now true; the "both artifacts" scope is an overstatement. Narrower than pass 4's CRITICAL — that asserted a wholly nonexistent test — so it is graded WARNING, but it is the same failure family and should be tightened before archive rather than carried forward silently.
2. R-013 is NOT satisfied and is permanently unsatisfiable as worded: `gentle-ai upgrade` was never executed, the required clobbers/does-not-clobber verdict does not exist, and the BEFORE-guard-code window closed in June 2026. Accepted by the maintainer as a documented historical deviation; must not be promoted verbatim.
3. R-006 enumerates the route domain as `{skill, agent}` while the real domain includes `opencode-agent`. Follow-up; one-word widening at promotion.
4. `route_test.go:901-902`'s `IN_SYNC` assertion is near-vacuous — empirically demonstrated (§2) to remain satisfied while `GADU.md` is absent, because other manifest rows emit `IN_SYNC`. Harmless as written since two sibling assertions carry the proof, but it would not detect a regression on its own. Prefer a per-file assertion scoped to the agent path.

**SUGGESTION**

1. Add a `gadu-operator/SKILL.md` row to the integration fixture manifest (`route_test.go:669-675`). This would convert R-008's deployment scenario from a two-test composition into a direct observation and let the restoration half cover both GADU artifacts, closing WARNING-1 at its root.
2. Retitle R-008's heading: "Evidence-Based Durability Guard" no longer describes a guarantee that is design-based rather than E1-evidence-based.
3. Make the generator test assert an exhaustive emission set, so a future added artifact trips CI instead of surfacing months later — this is exactly how the R-009 drift went unnoticed for five weeks. (Carried from passes 3 and 4.)

### TDD Compliance

| Check | Result | Details |
|---|---|---|
| TDD Evidence reported | PASS | "TDD Cycle Evidence — Unit 4" table present in apply-progress |
| All tasks have tests | PASS | 17/17 checked; unit 4's work unit is itself a test |
| RED confirmed | N/A — correctly declared | Characterization test; no RED claimed and none fabricated. Verified: behaviour authored 2026-06-25 (`43fdcb1d`), production code untouched, test change uncommitted |
| GREEN confirmed | PASS | `TestSyncCheck_DetectsMissingAgentFile` PASS on execution this pass, verbosely confirmed not SKIP |
| Triangulation | ADEQUATE | Restoration half asserts three distinct post-conditions across two observation channels (filesystem `os.Stat`, CLI `sync-check` output) |
| Safety Net for modified files | PASS | `route_test.go` was modified, not new; full installer package (5.744s, all tests) green before and after |

**TDD Compliance**: 6/6 checks passed. Units 1-3 were spec/doc-only and correctly declared TDD-inapplicable.

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|---|---|---|---|
| Unit | route-resolve + engine packages | multiple | `go test` |
| Integration | overlay CLI in sandbox `t.TempDir()` HOME, `testing.Short()`-guarded | `engine/installer/route_test.go` | `go test` + `bash` |
| E2E | 0 | — | not installed |

The AC-5 test is an **integration** test: it shells into the real `bin/labdrian-overlay` against a sandboxed HOME and overlay git repo, exercising `cmd_apply` and `cmd_sync_check` end to end. Correct layer for the behaviour.

### Changed File Coverage

Coverage analysis skipped — no coverage threshold or tooling is configured for this repository. Not a failure.

### Assertion Quality

| File | Line | Assertion | Issue | Severity |
|---|---|---|---|---|
| `engine/installer/route_test.go` | 901-902 | `!strings.Contains(outRestored, "IN_SYNC")` | Satisfiable by unrelated manifest rows; proven not to fire under neutralization | WARNING |

No tautologies, no ghost loops, no assertions that bypass production code, no mock-heavy tests. The two load-bearing restoration assertions (`:890-892`, `:898-900`) were empirically proven to depend on real behaviour.

**Assertion quality**: 0 CRITICAL, 1 WARNING.

### Quality Metrics

**Build**: clean in both module roots. **Vet**: clean in both module roots (empty output, exit 0). **Linter**: no additional linter configured beyond `go vet`.

### Why `fail` despite 0 CRITICAL

The verdict field is `fail` because `gentle-ai sdd-verify-validate` refuses a passing verdict
against incomplete evidence, and R-013 leaves the counts at 12/13 and 30/31. This was tested
directly this pass rather than inherited from pass 4: submitting these exact bytes with
`verdict: pass_with_warnings` was denied with "passing verdict contradicts failing or incomplete
evidence".

**`fail` here means incomplete evidence accounting, not a defect and not a blocker.** There are
**0 CRITICAL findings and 0 blockers** — that is the gate the archive phase reads. No test fails,
no code is wrong, both suites and both vets are clean, and the single open requirement is an
accepted, documented historical deviation. Raising the counts to 13/13 to obtain a green verdict
would be precisely the fabrication passes 2, 3 and 4 refused.

### Verdict

**FAIL (incomplete evidence) — 0 blockers, 0 critical findings. Archive-eligible on the CRITICAL gate.**

Verify pass 4's CRITICAL is genuinely closed, established from the repository rather than from any assertion: the AC-5 restoration test exists, a neutralization experiment proved its assertions depend on the behaviour they pin, and commit history confirms the characterization label is honest rather than a manufactured RED. Both suites and both vets are clean. The remaining gap is R-013 — one requirement and one scenario, permanently unsatisfiable, already accepted by the maintainer as a documented historical deviation. Counts are 12/13 and 30/31; reporting 13/13 would be fabrication.
