# Apply Progress: anti-generic-design-runtime-wiring (PR-1/4)

**Branch:** `anti-generic-design-runtime-wiring/pr-1-markers-asset`
**Delivery strategy:** feature-branch-chain (PR 1 -> PR 2 -> PR 3 -> PR 4)
**Chain position:** PR-1 of 4 — Unit 1: "Marker pair + embedded asset + frontmatter test (Phase 1)"
**Status:** Phase 1 COMPLETE (5/5 tasks). Phases 2-6 NOT started — reserved for PR-2, PR-3, PR-4.
**Mode:** Strict TDD (RED before GREEN, real failing tests first)

## Scope boundary (explicit)

This batch implements ONLY Phase 1 of `tasks.md`. It does NOT touch:
- Phase 2: `embeddedContract()` case in `engine/cmd/main.go`, `usage()` line (-> PR-2, Unit 2)
- Phase 3: `checkRegistry` + `HasSupportedClaudeLifecycleState` recognition (-> PR-2, Unit 2)
- Phase 4: `settings.Merger` hook pair wiring (-> PR-3, Unit 3)
- Phase 5: Isolation/drift guard + standalone `skills/_shared/anti-generic-design.md` copy (-> PR-3, Unit 3)
- Phase 6: Rebuild, `install-hooks`, live registry/status verification (-> PR-4, Unit 4)

The new marker constants and embedded asset are intentionally UNUSED by any `main.go` code path
until PR-2 wires `embeddedContract("anti-generic-design")`. `go build ./...` succeeds because Go
does not require package-level `const`/`var` to be referenced.

## Tasks completed (Phase 1 — R-101, R-103, R-104)

### [x] 1.1 RED — marker constants test
- Added `TestAntiGenericDesignMarkersAreDistinct` in `engine/propagator/propagator_test.go`
- Asserted non-empty, and pairwise-distinct from `BeginMarker`/`EndMarker` and
  `DiscoverySafetyBeginMarker`/`DiscoverySafetyEndMarker`
- Confirmed RED: `go test ./propagator/...` failed to compile —
  `undefined: propagator.AntiGenericDesignBeginMarker` / `EndMarker`

### [x] 1.2 GREEN — marker constants
- Added `AntiGenericDesignBeginMarker` / `AntiGenericDesignEndMarker` constants to
  `engine/propagator/propagator.go`, slug `anti-generic-design-scope`, mirroring the
  `DiscoverySafetyBeginMarker`/`EndMarker` pattern
- Confirmed GREEN: `TestAntiGenericDesignMarkersAreDistinct` passes; full `./propagator/...`
  suite (16 tests) still green — no regression

### [x] 1.3 — embedded canonical asset authored
- Created `engine/assets/anti-generic-design.md`: frontmatter
  `applies_to_phases: [sdd-tasks, sdd-apply]`, `excluded_phases: [sdd-propose, sdd-spec,
  sdd-design, sdd-verify, sdd-archive]`, `injection_point: "## Skills to load before work"`,
  plus an advisory-scope note matching the `skill-discovery-safety.md` shape
- Body distilled verbatim from `skills/anti-generic-design/SKILL.md` (4 forbidden patterns,
  steer-toward list, v1 manual self-critique checklist); the frontend-design cross-reference
  note is preserved

### [x] 1.4 RED — frontmatter parse test
- Created `engine/assets/assets_test.go` with `TestAntiGenericDesignFrontmatterParses`,
  asserting `propagator.ParseFrontmatter(assets.AntiGenericDesign)` yields
  `AppliesTo == ["sdd-tasks", "sdd-apply"]` with no error
- Confirmed RED: `go test ./assets/...` failed to compile — `undefined: assets.AntiGenericDesign`

### [x] 1.5 GREEN — embed wiring
- Added `//go:embed anti-generic-design.md` / `var AntiGenericDesign string` to
  `engine/assets/assets.go`, plus a doc-comment addition describing the new contract
- Confirmed GREEN: `TestAntiGenericDesignFrontmatterParses` passes

## TDD Cycle Evidence

| Task | RED | GREEN | REFACTOR |
|------|-----|-------|----------|
| 1.1/1.2 (markers) | `TestAntiGenericDesignMarkersAreDistinct` failed to compile (undefined constants) | Constants added; test + full `propagator` suite green | `gofmt -w` applied to touched files; no further refactor needed |
| 1.4/1.5 (embed) | `TestAntiGenericDesignFrontmatterParses` failed to compile (undefined `assets.AntiGenericDesign`) | `go:embed` var added; test green | None needed |
| 1.3 (asset content, no test) | N/A — content authoring task, verified indirectly by 1.4's frontmatter parse | N/A | N/A |

## Verification

- `cd engine && go build ./...` — OK
- `cd engine && go vet ./...` — OK
- `cd engine && go test ./...` — ALL PASS (assets, cmd, gadu, gate, installer, prespec,
  propagator, runtime, settings, skills)
- `gofmt -l` on touched files (`propagator.go`, `propagator_test.go`, `assets.go`,
  `anti-generic-design.md`, `assets_test.go`) — CLEAN (pre-existing gofmt issues in unrelated
  files like `cmd/main_test.go`, `gate/gate.go` are out of scope for this PR and were not
  touched)
- `git diff --stat engine/` confirms only the 3 intended files modified + 2 new files, no
  Phase 2-6 files touched

## Files changed

| File | Action | What Was Done |
|------|--------|----------------|
| `engine/propagator/propagator.go` | Modified | Added `AntiGenericDesignBeginMarker`/`EndMarker` const pair (R-101) |
| `engine/propagator/propagator_test.go` | Modified | Added `TestAntiGenericDesignMarkersAreDistinct` |
| `engine/assets/anti-generic-design.md` | Created | Canonical embedded contract text (R-103, R-104) |
| `engine/assets/assets.go` | Modified | Added `//go:embed anti-generic-design.md` / `var AntiGenericDesign string`; updated package doc |
| `engine/assets/assets_test.go` | Created | Added `TestAntiGenericDesignFrontmatterParses` |
| `openspec/changes/anti-generic-design-runtime-wiring/tasks.md` | Modified | Marked Phase 1 tasks 1.1-1.5 `[x]`; Phases 2-6 left unmarked |

## Remaining tasks (PR-2, PR-3, PR-4 — NOT in this batch)

- [ ] Phase 2 (2.1-2.6): `embeddedContract()` case + `usage()` update — Unit 2, PR-2
- [ ] Phase 3 (3.1-3.4): `checkRegistry` + lifecycle-state recognition — Unit 2, PR-2
- [ ] Phase 4 (4.1-4.4): Merger hook wiring — Unit 3, PR-3
- [ ] Phase 5 (5.1-5.5): Isolation, drift guard, standalone copy, R-106 verification — Unit 3, PR-3
- [ ] Phase 6 (6.1-6.3): Rebuild, install-hooks, live verification — Unit 4, PR-4

## Deviations from design

None — implementation matches `design.md`'s File Changes table entries for
`engine/propagator/propagator.go` and `engine/assets/anti-generic-design.md` /
`engine/assets/assets.go`.

## Status

5/5 Phase 1 tasks complete. This is PR-1 of 4 in the feature-branch-chain. Next batch (PR-2)
targets this branch and implements Phase 2 + Phase 3 (Unit 2). Ready for sdd-verify on the
Phase 1 slice.
