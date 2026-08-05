# Design: skills-validate-ondisk-gate

> Revision 2 — amended after phase-contract validation. Decisions 1-2 unchanged; Decision 3 premise corrected; Decision 5 added; consumer table closed at 11 rows; pin fixture, `t.Chdir`, R-008 wording, and doc-site list corrected.

## Technical Approach

Wire the existing, tested `DeployableManifestPaths`/`ScanSkillFiles`/`DiffOnDisk` (`engine/skills/ondisk.go`) into `RenderValidateCore` (`engine/skills/skills.go:57`) behind a new required `--source-root` flag and an injected scan seam. Slice 1 lands the consumer contract, R-004 pin test, corrected remediation text, and docs with zero behavior change; slice 2 lands the flag, seam, wiring, invocation-vector updates, and real-tree proof.

## Architecture Decisions

### Decision 1 (R-002): `--source-root` is a required explicit flag; absence fails loud

**Choice**: parse `--source-root <dir>` in the validate flag loop. Absent → stderr `error: skills validate requires --source-root <skills-dir>` + exit 1. No default. A supplied-but-invalid dir also fails loud via `ScanSkillFiles`'s stat error.

| Alternative | Rejected because |
|---|---|
| cwd default `./skills` | CI runs from `engine/`; `engine/skills/` holds 28 Go files → false `UNREGISTERED_ON_DISK` flood (cwd trap). Direct R-002 violation. |
| Derive from `dir(--manifest)/skills` | Implicit coupling of independent inputs; wrong tree for temp-file manifests; violates "skills directory is always an explicit input". |
| Absent → skip on-disk check (exit 0, with or without warning) | Silent healthy — consumers read exit codes, not warnings. Violates manifest rule 9; reintroduces the anti-generic-design failure. |

**Rationale**: `--source-root` is already a parsed flag with the identical skills-root meaning for `install` (`engine/skills/install.go:172`) and `add` (`engine/skills/lifecycle.go:176`), and `bin/labdrian-overlay:1559` already injects it for every skills verb — reuse is semantically consistent, not merely wire-compatible. The asymmetry vs `--registry`/`--manifest` defaults is deliberate: those predate this gate and every automated caller passes them explicitly. Interim safety: today's flag loop ignores unknown flags, so documenting or injecting `--source-root` before slice 2 is behavior-neutral.

**Consequences (all slice 2, atomic with fail-loud)**: `.github/workflows/ci.yml:158` (the `run:` line; step spans `:156-158`) gains `--source-root ../skills`; the spec scenario vector at `verification-evidence-capture/spec.md:145` gains the same (Decision 5); `engine/skills/skills_test.go:73-113` gains the flag plus an on-disk-complete fixture.

### Decision 2 (R-003): inject a scan function into RenderValidateCore

**Choice**: new parameter `scanSkills func(dir string) ([]string, error)`; the `SkillsCore` dispatch passes `ScanSkillFiles`; tests pass stubs. Manifest rows for `DeployableManifestPaths` come from the already-injected `readFile(manifestPath)` — no second new seam (the manifest is read twice: once by `Validate` via `os.Open`, once via `readFile`; accepted to keep `Validate` untouched).

| Alternative | Rejected because |
|---|---|
| Caller pre-scans, core takes `[]string` | Splits `--source-root` parsing out of the core (flag parsing lives inside `RenderValidateCore`); caller must know when a scan is needed. |
| Inject `fs.FS` | Requires rewriting tested `ScanSkillFiles`; proposal mandates reuse as-is; larger diff; loses the stat error distinction. |
| Package-level var swap | Global mutable state; contradicts documented "all I/O is injected"; racy under parallel tests. |

**Rationale**: matches the existing per-verb injection precedent (`RenderInstallCore` receives `os.Getwd` at `skills.go:23`, `AddCore` receives `os.Stat` at `skills.go:25`). Core tests never walk a real tree.

### Decision 3: the check lives in RenderValidateCore, NOT in `Validate`

`Validate` (`engine/skills/validate.go:93`) has exactly one non-test caller — `engine/skills/skills.go:89` — plus 10 test call sites (`validate_test.go` ×5, `lifecycle_test.go` ×4, `sync_test.go` ×1). Embedding the on-disk check there would widen the package API's contract for every current and future consumer, invalidate those 10 call sites' arrangement, and structurally hand the gate to any command that later calls `Validate` — violating the standalone constraint (skill-package-manager R-034). Only the `validate` verb gains it.

### Decision 4 (R-001): this design is the persisted consumer enumeration

Persisted (Engram + openspec) before any emitting code; slice-1 tasks reference this table rather than duplicating it.

### Decision 5: amend the motivating spec scenario's invocation vector (slice 2)

`openspec/changes/deterministic-verification-evidence/specs/verification-evidence-capture/spec.md:142-146` asserts that the bare vector (`--registry ../skills.registry.yaml --manifest ../overlay.manifest`, no `--source-root`) exits zero — under Decision 1 it becomes a usage error.

**Choice**: edit the WHEN line at `spec.md:145` to add `--source-root ../skills`, in slice 2, atomically with the `ci.yml:158` edit. The scenario's intent — "new skill file gets a manifest row, validate passes" — is preserved exactly; the invocation vector is incidental detail written before the gate had an on-disk input. As written, the scenario passes only vacuously today (the command never compares against the filesystem — the gap this change closes), so amending the vector is what makes the scenario genuinely enforceable.

| Alternative | Rejected because |
|---|---|
| Keep the bare vector exiting 0 via implicit derivation | Reintroduces Decision 1's rejected `dir(--manifest)/skills` coupling and the R-002 violation. |
| Declare the scenario superseded | False — nothing supersedes it; this change implements it. |

## Exit-Code Consumer Enumeration (R-001) — closed at 11 rows

| # | Consumer | Disposition |
|---|---|---|
| 1 | `.github/workflows/ci.yml:156-158` (cwd `engine`, no flag; `:158` is the `run:` edit target) | Edit in slice 2: add `--source-root ../skills` |
| 2 | `bin/labdrian-overlay:1544-1564` (injects flag at :1559; `exec` propagates exit) | No change; existing injection becomes active |
| 3 | `bin/labdrian-overlay:149` help text | Doc edit, slice 1 (R-010) |
| 4 | TUI `tui/run.go:80-86` + worst-severity aggregator `:368-377,:387-432` | No change: invokes via wrapper (gets flag); exit 1 on a real divergence rendering as hard failure is correct fail-loud; R-011 keeps it green today |
| 5 | `engine/cmd/main.go:41-43` doc comment AND `:173` validate usage line | Doc edits, slice 1 (R-010); usage line gains required `--source-root <path>` (format precedent: install/add lines `:174-175`) |
| 6 | `README.md:93,298` | Doc edits, slice 1 (R-010); `:298` usage signature gains `--source-root` |
| 7 | `openspec/changes/skill-project-scope/tasks.md:226` | Historical gate, already executed — leave unchanged |
| 8 | `openspec/changes/deterministic-verification-evidence/tasks.md:314` | Historical gate — leave unchanged |
| 9 | `openspec/changes/skill-manifest-gen/proposal.md:15,67` false `sync-manifest` claim | Correct in slice 1 (R-009): `isSkillRow` (`engine/skills/sync.go:19-44`) regenerates only `*/SKILL.md` rows — it cannot clear `UNREGISTERED_ON_DISK` for reference or `_shared/*` files |
| 10 | `openspec/changes/deterministic-verification-evidence/specs/verification-evidence-capture/spec.md:142-146` scenario vector | Edit in slice 2 per Decision 5 |
| 11 | `engine/skills/skills_test.go:73-113` `verb_validate` (no flag; fixture row `sdd-spec/SKILL.md` has no backing file) | Edit in slice 2: add `--source-root` and create the fixture file (else `MISSING_ON_DISK` fires); assertion statements (exit 0, "aligned") stay unedited |

R-010 therefore covers five documentation sites (rows 3, 5×2, 6×2), and every documented usage signature gains the now-required `--source-root <path>`, so the flag is discoverable before it can fail anyone.

## Sequence (validate verb after slice 2)

```
caller (CI | wrapper | TUI→wrapper)
  │ skills validate --registry R --manifest M --source-root S
  ▼
SkillsCore ─► RenderValidateCore(args, readFile, ScanSkillFiles, out, err, exit)
  │ parse flags; S absent → usage error, exit(1)
  │ readFile(R) → ParseRegistry
  │ Validate(reg, M)  — unchanged (4 classes, format/exit pinned, R-008)
  │ readFile(M) → DeployableManifestPaths   (infra rules untouched, R-004)
  │ scanSkills(S) → disk paths              (stat/walk error → fail loud)
  │ DiffOnDisk(disk, rows) → on-disk divs
  │ print existing divs first (byte-identical format), then on-disk divs as [%s] %s: %s
  │ any divs → exit(1); aligned → existing stdout line unchanged + one new on-disk line, exit 0
```

R-007: both sets print in one run; never first-error-stop.

## File Changes

| File | Slice | Action |
|---|---|---|
| `engine/skills/ondisk.go` | 1 | Modify the two `Detail` strings to name the manifest-row edit that resolves each class (R-009); zero non-test callers → no behavior change |
| `engine/skills/ondisk_test.go` + new pin test | 1 | Bidirectional R-004 pin (fixture below) |
| `README.md:93,298`, `bin/labdrian-overlay:149`, `engine/cmd/main.go:41-43,173` | 1 | R-010 exit-contract docs — five sites, incl. usage signatures gaining `--source-root` |
| `openspec/changes/skill-manifest-gen/proposal.md` | 1 | Correct remediation claim (R-009) |
| `engine/skills/skills.go` | 2 | Flag parse, new `scanSkills` param, wiring, fail-loud |
| `engine/skills/skills_test.go` | 2 | `verb_validate`: add `--source-root`, create fixture file for `sdd-spec/SKILL.md` row; new stubbed-seam core tests |
| `.github/workflows/ci.yml:158` | 2 | Add `--source-root ../skills` |
| `openspec/changes/deterministic-verification-evidence/specs/verification-evidence-capture/spec.md:145` | 2 | Add `--source-root ../skills` to the scenario vector (Decision 5) |

## R-004 Pin Test Fixture (corrected)

Manifest fixture rows: `_shared/x.md managed`, `_shared/pin/SKILL.md managed`, `engine/y.go managed`, `real-skill/SKILL.md managed`.

- Subtest A (pins ondisk direction): `DeployableManifestPaths` INCLUDES both `_shared` rows and EXCLUDES `engine/y.go`. Adding `_shared` to ondisk's exclusions fails A.
- Subtest B (pins manifest direction): `LoadManifestView` yields `real-skill` but NOT `_shared`. The explicit `_shared/pin/SKILL.md` row is what makes this direction meaningful — non-`SKILL.md` rows are skipped at `manifest.go:64-66` before `infraPrefixes` ever applies, so without it, removing `_shared/` from `infraPrefixes` (`manifest.go:13`) would fail nothing. With it, that removal fails B.

## Testing Strategy

Strict TDD — tests precede implementation. Focused: `cd engine && go test ./skills/...`, `cd engine && go test ./cmd/...`.

| Layer | What |
|---|---|
| Unit (slice 1) | Table-driven `Detail`-string remediation assertions; bidirectional pin test above |
| Core (slice 2) | Stub `scanSkills`: absent flag → exit 1 naming the flag; unregistered file → diagnostic + exit 1, row added → exit 0; orphan row → exit 1; ≥3 mixed divergences in one run; cwd-independence via `os.Chdir` + `t.Cleanup` across 3 dirs with absolute `--source-root` (repo precedent `engine/runtime/opencode_test.go:383-386`; NOT `t.Chdir` — Go 1.24 API, `engine/go.mod:3` pins `go 1.21` and CI installs via `go-version-file`) |
| Regression (R-008) | The four existing divergence classes' diagnostics, format, and exit codes stay byte-identical; existing assertion statements stay unedited. `verb_validate` (`skills_test.go:73-113`) changes invocation args and fixture files only — "zero assertion edits" applies to assert statements, not to the arrange/act sections |
| Real tree (slice 2) | Existing `TestRepositorySkillsAreFullyRegistered` (`engine/skills/ondisk_test.go:178-204`) plus a command-level run against the repo root exits 0 (R-011); green CI with no manifest edits |

## Threat Matrix

All rows N/A: `skills validate` executes nothing — no subprocess, no git invocation, no PR automation, no executable-file classification; it compares path strings and reports. The one real adversarial boundary — cwd-dependent path resolution — is closed by Decision 1 (explicit flag, fail-loud) and the cwd-independence test.

## Migration / Rollout / Risks

No data or manifest migration (real-tree baseline 2026-08-05: 80 rows → 57 deployable; 57 files; zero divergences). Feature-branch chain per entry contract; a slice-2 revert restores today's exit contract and reverts the CI, spec-scenario, and test-vector edits atomically.

- **Named risk — inter-slice doc drift**: R-010 docs (slice 1) describe the required flag before slice 2 makes it real. Window confined to the tracker branch (main sees the chain only at tracker merge), and interim behavior is neutral because today's flag loop ignores unknown flags.
- **Recorded, no action**: `openspec/changes/skill-manifest-gen/specs/spec.md:100` calls `SyncManifest` "the write-side inverse of `skills validate`" — imprecise post-change (R-098 concerns `Diff`, not `DiffOnDisk`). Harmless; do not chase.

## Open Questions

None — R-002 and R-003 are resolved above; the documented-vector conflict is resolved by Decision 5 (auto mode, no human checkpoint).
