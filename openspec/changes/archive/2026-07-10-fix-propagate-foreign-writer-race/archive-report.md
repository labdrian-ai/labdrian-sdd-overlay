# Archive Report: fix-propagate-foreign-writer-race

**Change:** fix-propagate-foreign-writer-race
**Archived:** 2026-07-10
**Archived to:** `openspec/changes/archive/2026-07-10-fix-propagate-foreign-writer-race/`

## Bug closed

This change closes the READ-side half of a foreign-writer race in
`propagate` (`runPropagateVerified`, `engine/cmd/main.go`), completing the
gap left open by write-side fix #1889. Both `gentle-ai skill-registry
refresh` (a separate Homebrew binary that regenerates
`.atl/skill-registry.md` wholesale, ignoring this project's own
`<registry>.lock`) and this overlay's own `propagate` hooks run in PARALLEL
on every `UserPromptSubmit`. Fix #1889 made the write side survive a foreign
clobber by re-reading and retrying after a verified write. Two independent
`judgment-day` judges then confirmed the read side still had two open holes:

1. A torn/empty read (`runPropagateCore`'s own fail-loud `exit(1)` guard,
   whose comment already noted an empty registry is the signature of a
   concurrent torn read) was forwarded as an immediate, non-retryable hard
   failure.
2. An absent read without `--require-registry` (the common case — none of
   the 3 hooks pass that flag) was treated as "project does not use the
   overlay" and returned a false-success exit 0, even when the true cause was
   a foreign writer's mid-refresh unlink-then-recreate window.

The fix adds `classifyReadRace` inside the existing bounded
`maxPropagateWriteAttempts` loop: torn/empty and transient-absent reads are
now retried before concluding failure or no-op, while genuine hard failures
(bad args, unreadable contract, broken frontmatter) still forward
immediately with zero retries. `runPropagateCore`'s signature and observable
behavior are unchanged — only two inline message literals were replaced with
references to shared sentinel consts (`emptyRegistryErrMsg`,
`registryAbsentNoopMarker`), an output-preserving substitution pinned by
guard tests.

## Spec merge

`specs/skill-registry-propagate/spec.md` in this change was the FIRST spec
written against the `skill-registry-propagate` capability — no prior
`openspec/specs/skill-registry-propagate/spec.md` existed. Per this repo's
"main spec does not exist → copy forward" convention (confirmed from the
`2026-07-10-skill-external-provenance` archive), the delta content was
copied forward as the initial full spec at
`openspec/specs/skill-registry-propagate/spec.md`.

The delta was already written in this repo's target format
(`## Requirements` → `### Requirement: <name>` with an `ID: R-xxx` line →
`#### Scenario: <name>` with GIVEN/WHEN/THEN bullets), matching
`overlay-coherence`, `skill-package-manager`, and `skill-lifecycle`, so no
reformatting was needed beyond adding the standard `# <Title> Specification`
/ `## Purpose` header and renaming `## ADDED Requirements` to `## Requirements`.

| Target spec file | Action | Requirements carried |
|---|---|---|
| `openspec/specs/skill-registry-propagate/spec.md` | Created (new capability dir) | R-001–R-006 (12 scenarios) |

R-001 (write-side, fix #1889) and R-005 (propagate-vs-propagate lock) were
already-implemented behavior documented here for the first time as this
domain's formal requirements. R-002, R-003, R-004, R-006 are new behavior
introduced by this change.

## Verify verdict

**GO — 0 CRITICAL, 0 WARNING (blocking), 1 non-blocking process note**
(`verify-report.md`, dated 2026-07-10).

- Build: `go build ./...` and `go vet ./...` clean.
- Tests: `go test ./cmd/... -v -count=1` → 97/97 subtests pass, 0 failures.
- All 6 spec requirements (R-001–R-006) traced to at least one passing test.
- `runPropagateCore` independently confirmed byte-for-byte output-unchanged.
- P-1 (process note, not a defect): diff size (989 changed lines across
  `engine/cmd/main.go` +221/-6 and `engine/cmd/main_test.go` +759/-3) is more
  than double this project's ~400-line review-workload budget. Verify
  recommended an explicit size-exception acknowledgment at commit/PR time
  rather than retroactively splitting — this is a single, cohesive bugfix
  with no natural split point (classifier, evidence tracking, and tests are
  one indivisible unit). Acknowledged here; not a blocker for archive.
- `next_recommended`: `sdd-archive`.

## Judgment-day history (6 rounds)

The user set an explicit zero-tolerance review bar for this change, which is
why it went through 6 rounds of dual-blind adversarial review instead of the
usual one or two:

- **Round 1** — 6 findings (4 WARNING, 2 SUGGESTION), mostly message/wording
  precision issues in the initial implementation of the read-race classifier.
- **Round 2** — one REAL logic bug: `classifyReadRace` returned `raceNone`
  whenever `wrote==true`, making a successful-but-then-clobbered write
  invisible to the evidence tracking meant to prefer fail-loud over a false
  no-op. Fixed by adding `sawWrote` tracking, folded into both exhaustion
  tie-breaks. This is the one genuine correctness gap found across the whole
  review history; every other round's findings were documentation-sync or
  wording/precision fixes, not logic defects.
- **Round 3** — `design.md` had drifted from claiming `runPropagateCore` was
  "byte-for-byte unchanged" to actually describing two output-preserving
  const substitutions inside it; one exhaustion branch still used
  cause-specific wording inconsistent with Round 2's cause-agnostic framing.
- **Round 4** — `design.md`/`spec.md` documentation hadn't caught up with
  Round 2–3 code changes (missing identifiers, missing R-006 requirement);
  one real branch-coverage gap (a test case for the `raceEmptyRegistry`
  mixed-sequence mirror of the `raceAbsentRegistry` case).
- **Round 5** — two consecutive `sdd-verify` regeneration passes on the
  verify report itself were found inaccurate (wrong test counts, a
  non-existent test name, missing R-006, a mistraced `R-001` citation); this
  archive's verify-report.md is the result of a third, manually
  fact-checked pass.
- Rounds 3–5 found zero new logic/correctness bugs — only documentation and
  traceability drift. The classification/retry logic itself was
  independently re-derived as correct by 2 different judges across 3
  consecutive rounds after Round 2's fix landed.

## Completeness

All 41 tasks in `tasks.md` are marked `[x]` (Phases 1–4: 14 original
implementation tasks; Phase 5 Rounds 1–4: 27 judgment-day review-fix tasks).
Code for this change (`engine/cmd/main.go`, `engine/cmd/main_test.go`) was
already committed on branch `fix/skill-registry-scope-block-wipe` at commit
`e89123d` prior to this archive operation.

## Directory move

- `git mv` used for the entire change folder: `proposal.md`, `design.md`,
  `tasks.md`, `specs/` (and contents), `apply-progress.md`,
  `verify-report.md` — all were already tracked by git (committed alongside
  the code in `e89123d`), so full history is preserved.
- Source directory `openspec/changes/fix-propagate-foreign-writer-race/`
  removed (now empty) after the move.
