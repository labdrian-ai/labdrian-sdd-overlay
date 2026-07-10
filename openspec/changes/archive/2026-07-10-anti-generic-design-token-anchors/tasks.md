# Tasks: anti-generic-design-token-anchors

**No automated tests**: content-only change; the checklist is v1 manual by explicit design
(proposal Out of Scope). Verify-before-done discipline replaces unit tests: each capture entry
is independently spot-checked, and the Phase 4 validation run is the RED→GREEN analog — it must
actually run and its outcome must be recorded, not asserted.

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~200-320 (palette-typography.md rewrite, +2 critique-template.md, +6 SKILL.md, example-output.md re-citations, new apply-progress.md section) |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Low

## Phase 1: Capture Pass (R-002, R-006, R-007)

- [x] 1.1 Fix the 6 verticals per `design.md` (dev-tools, fintech, luxury automotive,
      aerospace/frontier-tech, consumer electronics/entertainment, editorial retail/e-commerce);
      confirm each site is the CURRENT market leader, not a stale rebrand (apply may drop to 5).
- [x] 1.2 Visit dev-tools leader; record canvas/accent/radius/depth/type via
      computed-style/view-source/rendered, format `Site (sector): <value> — <method>, verified <date>`.
- [x] 1.3 Repeat 1.2 for fintech leader.
- [x] 1.4 Repeat 1.2 for luxury automotive leader.
- [x] 1.5 Repeat 1.2 for aerospace/frontier-tech leader.
- [x] 1.6 Repeat 1.2 for consumer electronics/entertainment leader.
- [x] 1.7 Repeat 1.2 for editorial retail/e-commerce leader.
- [x] 1.8 Spot-check each of the 6 recorded entries by independently re-visiting the site; no
      entry is done until re-checked (verify-before-done discipline).

## Phase 2: Restructure `palette-typography.md` (R-002)

- [x] 2.1 Replace the aspect table (current lines 6-13) with a 5-row dimension table — canvas
      polarity, accent scarcity, corner-radius signature, depth mechanism, type philosophy —
      each row citing 2-3 contrasting anchors from Phase 1.
- [x] 2.2 Relocate Sequencing/Layout/Voice rows (current lines 11-13) verbatim into a new fenced
      `## Compositional signals (qualitative — not per-anchor captured)` subsection.
- [x] 2.3 Add a derived `oldest-verified: <date>` per dimension row from its anchors' dates.
- [x] 2.4 Rewrite the Attribution note (current lines 15-25): retire the "human recollection /
      no capture artifact" caveat; state dated first-party capture + observational/factual CSS
      commentary as sole trademark mitigation.

## Phase 3: Checklist Gate (R-008)

- [x] 3.1 Append `[ ] token set is NOT >80% traceable to a single cited anchor → PASS` as the
      7th line of `critique-template.md`'s fenced checklist (current lines 7-14); re-pad `→ PASS`.
- [x] 3.2 Append the identical line to `SKILL.md`'s inline checklist (current lines 61-68);
      re-pad alignment.
- [x] 3.3 Update `SKILL.md`'s "Steer toward" (lines 42-55) and References (lines 73-78) to cite
      the sector-leader dimension table instead of "4 real award-gallery sites."

## Phase 4: Validation — the RED→GREEN analog (R-009)

- [x] 4.1 Construct Case A: purpose-built output faithfully reproducing ONE sector anchor's full
      kit (canvas+radius+depth+font) — the decisive mimicry test.
- [x] 4.2 Run the 7-line checklist against Case A; expect PASS lines 1-6, FAIL line 7. Record
      the actual per-line result (this is the RED run — must fail line 7 for the gate to be valid).
- [x] 4.3 Update `example-output.md`'s "After" section (lines 15-27) citations to the new
      sector anchors; this becomes Case B.
- [x] 4.4 Run the 7-line checklist against Case B; expect PASS all 7. Record the result.
- [x] 4.5 Construct Case C (fresh generation attempt) and run the checklist against it; record
      per-line PASS/FAIL.
- [x] 4.6 Record all 3 outcomes in `openspec/changes/anti-generic-design-token-anchors/apply-progress.md`.
      Validation PASSES only if Case A fails line 7 while passing 1-6, proving the gate is
      non-redundant; otherwise mark validation FAILED and rework the gate or case before closing.

## Phase 5: Close-out

- [x] 5.1 Verify all proposal Success Criteria checkboxes against the final files.
- [x] 5.2 Confirm no engine/Go/TUI/registry/manifest file was touched — change stays within
      `skills/anti-generic-design/**` plus `apply-progress.md`.
