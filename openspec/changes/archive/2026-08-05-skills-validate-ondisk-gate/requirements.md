# Requirements Brief — Enforce skills/ Registration in `skills validate`

**change-name:** `skills-validate-ondisk-gate`
**project:** `labdrian-sdd-overlay`
**tier:** 2 · **interaction_mode:** `auto` · **artifact_store_mode:** `hybrid` · **strict_tdd:** `true`
**contract bundle:** `2.0.0`
**Engram topic key:** `project/labdrian-sdd-overlay/requirements/skills-validate-ondisk-gate`

## Executive Summary

`openspec/changes/deterministic-verification-evidence/specs/verification-evidence-capture/spec.md:138-146`
states that a change adding a new file under `skills/` MUST add a matching row to
`overlay.manifest`, and names `skills validate --registry ../skills.registry.yaml --manifest
../overlay.manifest` as the check that enforces it. That check does not enforce it today.
`RenderValidateCore` (`engine/skills/skills.go:57-98`) only cross-checks `skills.registry.yaml`
against `overlay.manifest`; neither artifact is compared against the filesystem, so a file added
under `skills/` with no manifest row is invisible to every gate and is silently never deployed to
any runtime target.

The enforcement logic already exists and is already tested. `engine/skills/ondisk.go` implements
`DeployableManifestPaths` (`:48`), `ScanSkillFiles` (`:89`), and `DiffOnDisk` (`:137`), with the two
divergence classes `UNREGISTERED_ON_DISK` (`:22`) and `MISSING_ON_DISK` (`:25`). All three functions
are covered by `engine/skills/ondisk_test.go`, and all three have zero callers outside that file and
its tests. The work unit is the wiring, not the algorithm: connect proven logic to the command the
spec already names, without changing what `skills validate` already reports.

The wiring is not mechanical. It crosses four verified traps: there is no injected directory-walk
seam in the validate core; the command has no way to locate the `skills/` directory; the repository
contains two parser rule sets whose infra-exclusion rules deliberately diverge; and adding two
divergence classes changes what a non-zero exit from `skills validate` means for every consumer of
that exit code. Each trap is captured below as a named requirement rather than left to
implementation judgment.

## Source Inputs

| Source | Description | Confidence |
|---|---|---|
| `openspec/changes/deterministic-verification-evidence/specs/verification-evidence-capture/spec.md:138-153` | The requirement "New skills/ Files Are Registered in overlay.manifest" and its two scenarios naming `skills validate` as the enforcing check | High — read directly |
| `engine/skills/ondisk.go:14-174` | The complete, unwired on-disk cross-check: two divergence classes, three functions | High — read in full |
| `engine/skills/ondisk_test.go:1-215` | Coverage for all three functions; the only callers in the repository | High — read |
| `engine/skills/skills.go:14-98` | `SkillsCore` dispatch and `RenderValidateCore` — the flag parser and injection surface | High — read in full |
| `engine/skills/manifest.go:11-107` | `infraPrefixes`, `LoadManifestView`, `isInfraDir` — the second, divergent parser | High — read in full |
| `engine/skills/validate.go:8-107` | The four existing divergence classes and `Validate` | High — read in full |
| `bin/labdrian-overlay:1540-1564`, `:149`, `:1744` | `cmd_skills` flag injection, help text, dispatch | High — read |
| `.github/workflows/ci.yml:156-158` | The CI step that runs `skills validate` with `../`-relative paths from `engine/` | High — read |
| `tui/run.go:80-86`, `:368-377`, `:387-432` | The TUI "Skills" entry, `invocationSeverity`, and worst-severity exit aggregation | High — read |
| `overlay.manifest` (80 rows) and the real `skills/` tree | Baseline measured 2026-08-05: 57 deployable rows, 57 on-disk files, zero divergence in either direction | High — measured |
| `engine/skills/sync.go:19-44` | `isSkillRow` — `sync-manifest` regenerates only `*/SKILL.md` rows and excludes infra dirs | High — read |

## Stakeholder Anchors

| Anchor | Speaker/Source | Interpretation | Status |
|---|---|---|---|
| "WHERE the change adds a new file under `skills/`, the change MUST add a matching row to `overlay.manifest`." | `verification-evidence-capture/spec.md:140` | The registration rule already exists as a spec obligation | Business rule |
| "WHEN `skills validate --registry ../skills.registry.yaml --manifest ../overlay.manifest` runs — THEN it exits zero" | `verification-evidence-capture/spec.md:145-146` | The named enforcing check, invoked with `../`-relative paths | Requirement |
| "GIVEN no new file under `skills/` — THEN it exits zero unchanged" | `verification-evidence-capture/spec.md:150-152` | Existing behaviour must not regress | Requirement |
| "If a new `skills/` file is ever added, add its `overlay.manifest` row and run `skills validate` — exit 0." | `openspec/changes/deterministic-verification-evidence/tasks.md:314` | The obligation currently depends on a human remembering it | Requirement |
| "That route is impossible ... following it literally left the skill registered nowhere, undeployed to all three runtimes, while `apply`, `sync-check` and `skills validate` all reported green." | `openspec/specs/anti-generic-design/spec.md:120` | The exact failure this gate must make impossible: green gates over an undeployed skill | Requirement |
| "`engine skills validate` SHALL be a STANDALONE command this slice; it MUST NOT be invoked or wired inside `cmd_apply`, `cmd_capture`, or any other existing command." | `openspec/changes/skill-package-manager/specs/spec.md:225-226` (R-034) | A binding constraint on where the new gate may live | Business rule |
| "`skills sync-manifest` makes a drifted manifest pass `skills validate`." | `openspec/changes/skill-manifest-gen/proposal.md:67` | A documented remediation claim that this change partially invalidates | Open question → resolved as R-009 |

## Contradictions and Ambiguities Pre-Analysis

| Issue ID | Type | Conflicting / Ambiguous Anchors | Why It Matters | Resolution Options | Decision Needed |
|---|---|---|---|---|---|
| I-01 | Missing rule | `RenderValidateCore` injects `readFile` (`engine/skills/skills.go:57`) but has no directory-walk seam; `ScanSkillFiles` (`engine/skills/ondisk.go:89-131`) calls `os.Stat` and `filepath.WalkDir` directly | Without a seam the validate core becomes untestable without a real filesystem, breaking the package's established injection pattern | A: pass pre-scanned paths into the core. B: inject a scan function alongside `readFile`. C: another named seam | Deferred to design by explicit producer instruction — captured as R-003, which requires the decision to be named and justified rather than made here |
| I-02 | Missing rule | `skills validate` accepts only `--registry` and `--manifest` (`engine/skills/skills.go:60-73`); the spec scenario invokes it with `../`-relative paths (`spec.md:145`) | The skills directory cannot be assumed to be `./skills`. From the CI cwd (`engine/`, `ci.yml:157`) `./skills` resolves to `engine/skills/` — the Go package — which would report every `.go` file as `UNREGISTERED_ON_DISK` | A: consume the `--source-root` already injected by `cmd_skills` (`bin/labdrian-overlay:1559`). B: derive from `dirname(--manifest)`. C: add a new dedicated flag | Deferred to design by explicit producer instruction — captured as R-002, which fixes the property (explicit, not cwd-derived) without fixing the mechanism |
| I-03 | Contradiction | `manifest.go:13` excludes `engine/` **and** `_shared/` as infra; `ondisk.go:70` excludes **only** `engine/` | The divergence is correct and load-bearing: `overlay.manifest` carries 8 real deployable `_shared/*` rows. A "tidy-up" unification onto `manifest.go`'s rule would silently drop all 8 from the on-disk check; a unification onto `ondisk.go`'s rule would inject `_shared` as a phantom skill directory into the registry cross-check | A: pin the divergence with a test and document why. B: unify only after proving what each consumer needs | Resolved: option A, captured as R-004. No unification is in scope |
| I-04 | Missing rule | `skill-manifest-gen/proposal.md:15,67` states `sync-manifest` makes a drifted manifest pass `skills validate`; `engine/skills/sync.go:19-44` shows `isSkillRow` only regenerates `*/SKILL.md` rows and excludes infra dirs | After this change, `sync-manifest` cannot resolve an `UNREGISTERED_ON_DISK` for any non-`SKILL.md` file (e.g. `requirements-from-transcripts/references/output-template.md`) or any `_shared/*` file. A diagnostic pointing at `sync-manifest` would send the operator down a path that cannot work | A: diagnostics name the manifest-row edit as the remediation. B: extend `sync-manifest` (out of scope, separate change) | Resolved: option A, captured as R-009. Option B recorded in Out of Scope |

No blocking stop was raised: I-01 and I-02 were explicitly directed by the producer contract to become named design outputs rather than answers decided here; I-03 and I-04 were resolvable from verified code evidence.

<!-- Promotion note: when promoting this brief to a delta spec, reformat each `### R-NNN — <name>` heading to `### Requirement: {Name}` with `ID: R-NNN` as the first body line — never embed the ID in the heading (archive matches by exact heading). -->

**Scope enum rubric used below** — `new-capability`: behaviour with no CLI-observable equivalent today. `feature`: extends or constrains an existing observable surface (flags, diagnostics, documented contract). `fix`: closes a verified gap or latent defect in existing code or test coverage.

## Requirements

### R-001 — Exit-Code Consumer Enumeration Precedes The Behavior Change

**Scope:** feature
**Type:** technical debt
**Size:** small
**Order:** 1
**Keywords:** exit code, consumer enumeration, blast radius, skills validate, non-zero exit, CI gate, TUI severity, consumer-impact rule
**Source anchor:** Producer constraint: "This change adds two new diagnostic classes to `skills validate`, which changes what a non-zero exit MEANS." Reinforced by `openspec/specs/anti-generic-design/spec.md:120`, where every gate reported green over an undeployed skill.
**Intent:** This repository's recurring failure mode is computing a value and never checking who consumes it. Adding `UNREGISTERED_ON_DISK` and `MISSING_ON_DISK` redefines the meaning of `exit 1` from "registry and manifest disagree" to "registry, manifest, or the filesystem disagree". Every consumer that branches on that exit code inherits the new meaning whether or not anyone looked.
**Requirement:** WHERE the change adds a divergence class to `skills validate`, the change SHALL persist a named enumeration of every consumer of that command's exit code together with the assessed impact on each consumer, before any code path emits the new class.
**Acceptance scenarios:**
- GIVEN the change proposes a new divergence class, WHEN design is produced, THEN design contains a consumer table with one row per consumer, each row carrying `path:line` and an explicit impact statement.
- GIVEN the enumeration exists, WHEN a reviewer checks it against a fresh repository-wide search for `skills validate` and `overlay.manifest` invocations, THEN no consumer is missing from the table.
- GIVEN a consumer whose behaviour changes, WHEN the table is read, THEN it states whether that consumer requires a code, config, or documentation change in this same change unit.

**Verified consumer seed (starting set, not the closed list — R-001 requires re-deriving and closing it):**

| # | Consumer | Location | Assessed impact |
|---|---|---|---|
| 1 | CI step "Validate skills registry" | `.github/workflows/ci.yml:156-158` | Runs `go run ./cmd/ skills validate --registry ../skills.registry.yaml --manifest ../overlay.manifest` with `working-directory: engine` and **no** `--source-root`. Turns red on any new class. If the skills directory were resolved from cwd, this step would scan `engine/skills/` (Go sources) and fail on every `.go` file. |
| 2 | `cmd_skills` wrapper | `bin/labdrian-overlay:1544-1564` | Injects `--registry`, `--manifest`, and `--source-root "$OVERLAY_DIR/skills"` (`:1559`), then `exec`s the engine binary so the exit code propagates verbatim. `--source-root` is already supplied here but silently ignored by `RenderValidateCore` (`engine/skills/skills.go:60-73`). |
| 3 | TUI "Skills" dashboard entry | `tui/run.go:80-86` (action + `Also`), `:387-432` (aggregation), `:368-377` (`invocationSeverity`) | `skills validate` is folded into a read-only menu entry; severity aggregates by worst result, so `exit 1` renders the Skills entry as a hard failure rather than a warning. |
| 4 | `overlay --help` text | `bin/labdrian-overlay:149` | States "cross-check registry vs overlay.manifest" — becomes incomplete. |
| 5 | README command reference | `README.md:298` ("cross-check registry vs manifest; exit 1 on any divergence"), `README.md:93` (read-only registry actions) | Documented exit contract becomes incomplete. |
| 6 | Engine command doc comment | `engine/cmd/main.go:41-43` | States "validate: cross-check registry vs overlay.manifest; exit 1 on divergence" — becomes incomplete. |
| 7 | `sync-manifest` remediation claim | `openspec/changes/skill-manifest-gen/proposal.md:15,67`; behaviour at `engine/skills/sync.go:19-44` | `sync-manifest` regenerates only `*/SKILL.md` rows and excludes infra dirs, so it cannot clear an `UNREGISTERED_ON_DISK` for a reference file or a `_shared/*` file. The documented remediation stops being sufficient. |
| 8 | Prior acceptance gates asserting exit 0 | `openspec/changes/skill-project-scope/tasks.md:226`; `openspec/changes/deterministic-verification-evidence/tasks.md:314` | These now assert a strictly stronger property than when they were written. |
| 9 | Standalone-command constraint | `openspec/changes/skill-package-manager/specs/spec.md:225-226` (R-034) | Binds the new gate to live inside `skills validate` only; it must not be wired into `cmd_apply`, `cmd_capture`, or any other command. |

**Evidence required:** Design-time consumer table with `path:line` per row; reviewer re-derivation of the list; explicit disposition (change / no change) per consumer.
**Ambiguities:** None. The seed table is a floor, not a ceiling — R-001 is satisfied only by a re-derived, closed list.

---

### R-002 — The Skills Directory Is An Explicit Input, Never Derived From The Working Directory

**Scope:** feature
**Type:** feature
**Size:** small
**Order:** 2
**Keywords:** skills directory, source-root, relative path, working directory, cwd, --manifest, path resolution, engine/skills collision
**Source anchor:** `verification-evidence-capture/spec.md:145` invokes the check as `skills validate --registry ../skills.registry.yaml --manifest ../overlay.manifest`; `.github/workflows/ci.yml:157` sets `working-directory: engine`.
**Intent:** The command has no current way to locate `skills/`: `RenderValidateCore` parses only `--registry` and `--manifest` (`engine/skills/skills.go:60-73`). Because the spec scenario and CI both invoke it with `../`-relative paths from a cwd that is not the overlay root, a cwd-derived `./skills` would resolve to `engine/skills/` — a real directory holding 28 Go files — and report each as unregistered. The failure would look like a working gate producing correct-looking noise.
**Requirement:** WHERE `skills validate` performs the on-disk cross-check, the command SHALL resolve the skills directory from an explicit caller-supplied path rather than from the process working directory.
**Acceptance scenarios:**
- GIVEN cwd is `engine/` and the exact CI argument vector from `ci.yml:158`, WHEN `skills validate` runs, THEN the cross-check reads the overlay `skills/` tree and never enumerates any file under `engine/skills/`.
- GIVEN the wrapper invocation `bin/labdrian-overlay skills validate` from an arbitrary cwd, WHEN the command runs, THEN it resolves the same skills directory as the CI invocation.
- GIVEN the skills directory cannot be resolved, WHEN the command runs, THEN it fails loud with a diagnostic naming the missing input and never silently skips the on-disk cross-check.
**Evidence required:** Table-driven core test asserting resolution from an explicit input across at least three cwd values; the third scenario asserts fail-loud, never silent-skip. Named design output: which input carries the path (reuse of the already-injected `--source-root`, derivation from `--manifest`, or a new flag) and the absent-input behaviour.
**Ambiguities:** Mechanism is a required design output; the property (explicit, not cwd-derived) is fixed here.

---

### R-003 — The Directory-Walk Seam Is A Named, Justified Design Output

**Scope:** fix
**Type:** technical debt
**Size:** small
**Order:** 3
**Keywords:** injected seam, testability, ScanSkillFiles, RenderValidateCore, readFile injection, filesystem walk, pure core, dependency injection
**Source anchor:** `engine/skills/skills.go:57` injects `readFile`, `stdout`, `stderr`, and `exit`; `engine/skills/ondisk.go:89-131` calls `os.Stat` and `filepath.WalkDir` directly.
**Intent:** The package's established pattern is a testable core with injected I/O, but there is no injected directory-walk seam, and the manifest is already read outside the injected seam (`Validate` → `LoadManifestView` → `os.Open`, `engine/skills/validate.go:93-94`, `engine/skills/manifest.go:38-45`). Adding a real-filesystem walk without a decision makes the validate core untestable in isolation and quietly deepens the existing inconsistency. The decision itself — not a specific answer — is the required output.
**Requirement:** WHERE `RenderValidateCore` gains the on-disk cross-check, the design SHALL name the seam through which the directory walk enters the core and record why that seam was chosen over the alternatives it rejects.
**Acceptance scenarios:**
- GIVEN design is produced, WHEN it is read, THEN it names exactly one seam and lists the rejected alternatives with the reason each was rejected.
- GIVEN the chosen seam, WHEN the new validate-core tests run, THEN they exercise both on-disk divergence classes without walking the repository's real `skills/` tree.
- GIVEN the chosen seam, WHEN the existing registry/manifest tests in `engine/skills/skills_test.go` and `engine/skills/validate_test.go` run, THEN they pass without modification to their assertions.
**Evidence required:** Named seam in design with rejected alternatives; core tests that produce both classes from injected inputs only.
**Ambiguities:** The answer is deliberately not chosen here. Selecting it in this brief would decide a design question with no consumer evidence.

---

### R-004 — The Two Infra-Exclusion Rule Sets Stay Independent And Are Pinned By Test

**Scope:** fix
**Type:** technical debt
**Size:** small
**Order:** 4
**Keywords:** infraPrefixes, _shared, engine, DeployableManifestPaths, LoadManifestView, infra exclusion, unification trap, silent drop
**Source anchor:** `engine/skills/manifest.go:13` — `infraPrefixes = []string{"engine/", "_shared/"}`; `engine/skills/ondisk.go:66-72` — excludes only a first segment equal to `engine`.
**Intent:** The divergence is verified and load-bearing. `LoadManifestView` builds a *skill-directory* view for the registry cross-check, where `_shared` is not a skill and would otherwise appear as a phantom `MISSING_IN_REGISTRY` entry. `DeployableManifestPaths` builds a *deployable-file* view, and `_shared/*` files are real deployable rows: `overlay.manifest` carries 8 of them (`_shared/pre-sdd-contracts.md`, `minimalism-contract.md`, `oo-quality-contract.md`, `skill-discovery-safety.md`, `anti-generic-design.md`, `entry-contract.schema.json`, `actuals-record.schema.json`, `review-projection-contract.md`). A refactor that unifies the two rule sets onto `manifest.go`'s list would silently remove all 8 from the on-disk check while every test still passed. Today nothing in the test suite makes that regression visible.
**Requirement:** The overlay SHALL pin, with an executable test, that `DeployableManifestPaths` treats `_shared/` rows as deployable while `LoadManifestView` excludes them as infra.
**Acceptance scenarios:**
- GIVEN a manifest containing a `_shared/*` row, WHEN `DeployableManifestPaths` parses it, THEN the row is present in the returned set.
- GIVEN the same manifest, WHEN `LoadManifestView` parses it, THEN no `_shared` entry appears in the view.
- GIVEN a change that adds `_shared/` to `ondisk.go`'s exclusion or removes it from `manifest.go`'s `infraPrefixes`, WHEN the suite runs, THEN at least one test fails with a message naming the consumer whose requirement was broken.
**Evidence required:** A test asserting both directions in the same file, with a comment stating why the rule sets differ and what each consumer needs. Unification is explicitly out of scope for this change.
**Ambiguities:** None. Divergence verified in both files by direct read.

---

### R-005 — A File Under skills/ With No Deploying Manifest Row Fails Validation

**Scope:** new-capability
**Type:** feature
**Size:** small
**Order:** 5
**Keywords:** UNREGISTERED_ON_DISK, unregistered skill file, missing manifest row, never deployed, DiffOnDisk, skills validate exit 1
**Source anchor:** `verification-evidence-capture/spec.md:140` — "the change MUST add a matching row to `overlay.manifest`"; enforcement class already defined at `engine/skills/ondisk.go:22`.
**Intent:** This is the failure the spec requirement exists to prevent and the one `openspec/specs/anti-generic-design/spec.md:120` records as having actually happened: a skill file present in the repository, absent from the manifest, therefore never copied to any runtime target, with `apply`, `sync-check`, and `skills validate` all reporting green.
**Requirement:** IF a regular file exists under the overlay `skills/` tree and no `overlay.manifest` row deploys it, THEN `skills validate` SHALL write an `UNREGISTERED_ON_DISK` diagnostic naming that path to stderr and exit 1.
**Acceptance scenarios:**
- GIVEN a new file `some-skill/references/new.md` under `skills/` and no manifest row for it, WHEN `skills validate` runs, THEN stderr contains an `UNREGISTERED_ON_DISK` line naming that path and the exit code is 1.
- GIVEN that file is then given a matching `overlay.manifest` row, WHEN `skills validate` runs again, THEN the exit code is 0 and no on-disk diagnostic is written.
- GIVEN a dot-prefixed file or directory under `skills/`, WHEN `skills validate` runs, THEN it is not reported (matches `ScanSkillFiles`, `engine/skills/ondisk.go:105-111`).
- GIVEN a manifest row whose third column is `agent` or `opencode-agent`, WHEN the cross-check runs, THEN that row is excluded from the deployable set (matches `route_resolve` in `bin/labdrian-overlay`; see `engine/skills/ondisk.go:31-34,62-64`).
**Evidence required:** Test-first Go tests at the CLI-core level asserting the diagnostic class, the named path, and exit 1; plus the negative case restoring exit 0.
**Ambiguities:** None.

---

### R-006 — A Deploying Manifest Row With No File Fails Validation

**Scope:** new-capability
**Type:** feature
**Size:** small
**Order:** 6
**Keywords:** MISSING_ON_DISK, orphan manifest row, missing source file, deploy cannot satisfy, DiffOnDisk, skills validate exit 1
**Source anchor:** `engine/skills/ondisk.go:23-25` — "reports a deploying manifest row whose source file is absent from `skills/`. The deploy step cannot satisfy such a row."
**Intent:** The converse failure is equally silent: a row survives a file deletion or rename, and the deploy step has nothing to copy. Without this direction the gate would only be half a gate, and the DoD explicitly names both.
**Requirement:** IF an `overlay.manifest` row deploys a path under `skills/` and no file exists at that path, THEN `skills validate` SHALL write a `MISSING_ON_DISK` diagnostic naming that row path to stderr and exit 1.
**Acceptance scenarios:**
- GIVEN a manifest row `some-skill/gone.md custom` and no such file under `skills/`, WHEN `skills validate` runs, THEN stderr contains a `MISSING_ON_DISK` line naming that path and the exit code is 1.
- GIVEN a root-level row with no `/` (for example `skills.registry.yaml custom`), WHEN the cross-check runs, THEN it is excluded from the deployable set and never reported as missing (`engine/skills/ondisk.go:66-69`).
- GIVEN an `engine/*` row, WHEN the cross-check runs, THEN it is excluded from the deployable set and never reported as missing (`engine/skills/ondisk.go:70-72`).
**Evidence required:** Test-first Go tests asserting the class, the named path, and exit 1; plus explicit tests for the root-level and `engine/*` exclusions.
**Ambiguities:** None.

---

### R-007 — On-Disk Divergences Are Reported In Full, Not One At A Time

**Scope:** new-capability
**Type:** feature
**Size:** small
**Order:** 7
**Keywords:** full scan, all divergences, no early return, batch diagnostics, operator feedback loop
**Source anchor:** `engine/skills/ondisk.go:133-136` — "It performs a full scan and never stops at the first divergence"; matching behaviour in `Diff` (`engine/skills/validate.go:42`).
**Intent:** The existing registry/manifest check already reports every divergence in one run. If the wiring short-circuits on the first on-disk finding, an operator who adds five unregistered files needs five runs to learn about five problems, and the new gate would behave inconsistently with the check it sits beside.
**Requirement:** WHEN `skills validate` finds more than one on-disk divergence, the command SHALL report every divergence in the same run.
**Acceptance scenarios:**
- GIVEN two unregistered files and one manifest row with no file, WHEN `skills validate` runs, THEN stderr contains three diagnostic lines and the exit code is 1.
- GIVEN on-disk divergences and registry/manifest divergences in the same run, WHEN `skills validate` runs, THEN both sets are reported and the exit code is 1.
**Evidence required:** Test asserting the exact count and set of diagnostic lines for a mixed-divergence fixture.
**Ambiguities:** None.

---

### R-008 — Existing Registry/Manifest Behavior Is Unchanged

**Scope:** feature
**Type:** technical debt
**Size:** small
**Order:** 8
**Keywords:** regression guard, MISSING_IN_MANIFEST, MISSING_IN_REGISTRY, TAG_MISMATCH, MIXED_TAG, R-028, R-033, unchanged exit 0
**Source anchor:** `verification-evidence-capture/spec.md:150-152` — "GIVEN no new file under `skills/` ... THEN it exits zero unchanged"; existing classes at `engine/skills/validate.go:11-20`; contract at `openspec/changes/skill-package-manager/specs/spec.md:204-230`.
**Intent:** The DoD states the existing behaviour must be untouched. Four divergence classes, their diagnostic text, the aligned-success message on stdout (`engine/skills/skills.go:97`), and the `--registry`/`--manifest` defaults are an established contract with documented consumers. Adding a gate must not renegotiate them.
**Requirement:** WHILE the on-disk cross-check is active, `skills validate` SHALL keep the `MISSING_IN_MANIFEST`, `MISSING_IN_REGISTRY`, `TAG_MISMATCH`, and `MIXED_TAG` outcomes, their diagnostic format, and their exit codes exactly as they are today.
**Acceptance scenarios:**
- GIVEN a registry entry with no manifest skill directory, WHEN `skills validate` runs, THEN the `MISSING_IN_MANIFEST` diagnostic and exit 1 are byte-identical to the pre-change behaviour.
- GIVEN a fully aligned registry, manifest, and `skills/` tree, WHEN `skills validate` runs, THEN stdout still reads `registry and manifest aligned (N skills)` and the exit code is 0.
- GIVEN the existing tests in `engine/skills/validate_test.go` and `engine/skills/skills_test.go`, WHEN the suite runs after the change, THEN they pass without assertion edits.
**Evidence required:** Unmodified existing test assertions passing; a success-path test covering the aligned case end to end.
**Ambiguities:** None.

---

### R-009 — On-Disk Diagnostics Name A Remediation That Actually Works

**Scope:** feature
**Type:** feature
**Size:** small
**Order:** 9
**Keywords:** diagnostic remediation, sync-manifest limitation, isSkillRow, SKILL.md only, _shared, actionable error, operator guidance
**Source anchor:** `openspec/changes/skill-manifest-gen/proposal.md:15,67` claims `sync-manifest` makes a drifted manifest pass `skills validate`; `engine/skills/sync.go:19-44` shows `isSkillRow` accepts only paths ending in `/SKILL.md` and excludes infra dirs.
**Intent:** After this change the most common failure will be an unregistered non-`SKILL.md` file — a `references/*.md`, a schema, or a `_shared/*` file. `sync-manifest` structurally cannot create rows for any of those. A diagnostic that points at it, or that says nothing, sends the operator to a command that exits successfully while fixing nothing.
**Requirement:** WHEN `skills validate` exits non-zero because of an on-disk divergence, the command SHALL name the manifest-row edit that resolves it rather than a command that cannot resolve it.
**Acceptance scenarios:**
- GIVEN an unregistered `some-skill/references/x.md`, WHEN `skills validate` runs, THEN the diagnostic tells the operator to add a matching `overlay.manifest` row and does not attribute the fix to `sync-manifest`.
- GIVEN a `MISSING_ON_DISK` row, WHEN `skills validate` runs, THEN the diagnostic tells the operator to restore the file or remove the row.
- GIVEN the `sync-manifest` remediation claim in `openspec/changes/skill-manifest-gen/proposal.md:67`, WHEN the change ships, THEN that claim is corrected or explicitly scoped to `*/SKILL.md` rows.
**Evidence required:** Diagnostic-text assertions in tests; the corrected or scoped remediation claim.
**Ambiguities:** None.

---

### R-010 — The Documented Exit Contract Covers The New Classes

**Scope:** feature
**Type:** feature
**Size:** small
**Order:** 10
**Keywords:** README, help text, exit 1 on any divergence, documented contract, doc drift, engine/cmd/main.go comment
**Source anchor:** `README.md:298` ("cross-check registry vs manifest; exit 1 on any divergence"), `README.md:93`, `bin/labdrian-overlay:149`, `engine/cmd/main.go:41-43`.
**Intent:** Four separate places describe `skills validate` as a registry-versus-manifest check. Once the command also compares the filesystem, all four understate what a non-zero exit means. This is the documentation half of R-001's consumer impact and is independently observable.
**Requirement:** WHERE `skills validate` can exit non-zero because of an on-disk divergence, the overlay documentation SHALL state that the command also cross-checks the `skills/` tree against `overlay.manifest`.
**Acceptance scenarios:**
- GIVEN the change is applied, WHEN `README.md:298` and `README.md:93` are read, THEN both describe the on-disk cross-check as part of the command's contract.
- GIVEN the change is applied, WHEN `overlay --help` is run, THEN the `validate` line reflects the on-disk cross-check.
- GIVEN the change is applied, WHEN `engine/cmd/main.go:41-43` is read, THEN the doc comment reflects the on-disk cross-check.
**Evidence required:** Diff of the four documentation sites; the `--help` output line.
**Ambiguities:** None.

---

### R-011 — The Overlay's Own Tree Passes The New Gate

**Scope:** new-capability
**Type:** feature
**Size:** small
**Order:** 11
**Keywords:** real tree, real manifest, 80 rows, 57 deployable, dogfooding, not fixtures, CI acceptance, broken gate
**Source anchor:** Producer constraint: "The new check must be verified against the REAL 80-row `overlay.manifest` and the real `skills/` tree, not only fixtures." Baseline measured 2026-08-05 by replaying the `DeployableManifestPaths` rules over the real files.
**Intent:** A gate that fails against its own repository ships broken and gets disabled on first contact. Fixtures prove the algorithm; only the real tree proves the wiring, the path resolution from R-002, and the infra rules from R-004 simultaneously. The repository has an established precedent for tests that read the real files (`engine/skills/oo_quality_contract_artifact_test.go:31-36`; `tools/entry-contract-validator/main_test.go:326`).
**Requirement:** WHEN `skills validate` runs against this repository's real `overlay.manifest` and real `skills/` tree, the command SHALL exit 0.
**Acceptance scenarios:**
- GIVEN the repository at the change's HEAD, WHEN the CI step at `.github/workflows/ci.yml:156-158` runs unmodified, THEN it exits 0.
- GIVEN the real `overlay.manifest` and the real `skills/` tree, WHEN a Go test replays the on-disk cross-check over them, THEN it reports zero `UNREGISTERED_ON_DISK` and zero `MISSING_ON_DISK` divergences.
- GIVEN the measured baseline (80 manifest rows total; 3 root-level rows and 19 `engine/*` rows and 1 `opencode-agent` row excluded; 57 deployable rows; 57 on-disk files; 8 of the deployable rows under `_shared/`), WHEN the test runs, THEN the deployable-row count and the on-disk file count agree.
**Evidence required:** A real-tree Go test in `engine/skills/` following the existing repo-root test precedent, plus a green CI run of the unmodified `ci.yml` step.
**Ambiguities:** None. Baseline independently measured: zero divergence in both directions today.

## Traceability Matrix

| Source Anchor | Requirement | Keywords | Scenario | Evidence | Status |
|---|---|---|---|---|---|
| `spec.md:140` "MUST add a matching row to `overlay.manifest`" | R-005 | UNREGISTERED_ON_DISK, missing manifest row | New file with no row → exit 1 | Core test + diagnostic assertion | Covered |
| `ondisk.go:23-25` deploying row with absent source | R-006 | MISSING_ON_DISK, orphan row | Row with no file → exit 1 | Core test + diagnostic assertion | Covered |
| `spec.md:145` `../`-relative invocation | R-002 | source-root, cwd, path resolution | CI cwd `engine/` resolves overlay `skills/` | Multi-cwd core test | Covered |
| `spec.md:150-152` "exits zero unchanged" | R-008 | regression guard, exit 0 | Aligned tree → exit 0, message unchanged | Unmodified existing tests | Covered |
| `ci.yml:156-158` CI invocation without `--source-root` | R-001, R-002, R-011 | CI gate, exit code, real tree | Unmodified CI step exits 0 | Green CI run | Covered |
| `tui/run.go:80-86`, `:368-377` severity aggregation | R-001 | TUI severity, hard failure | Consumer table row 3 | Design consumer table | Covered |
| `manifest.go:13` vs `ondisk.go:70` infra divergence | R-004 | `_shared`, infraPrefixes, unification trap | Both parsers pinned in one test | Bidirectional pin test | Covered |
| `skills.go:57` injected `readFile`, no walk seam | R-003 | injected seam, testability | Design names one seam + rejects alternatives | Design output + core tests | Covered |
| `sync.go:19-44` `isSkillRow` SKILL.md-only | R-009 | sync-manifest limitation, remediation | Diagnostic names the row edit | Diagnostic-text test | Covered |
| `README.md:298`, `bin/labdrian-overlay:149`, `engine/cmd/main.go:41-43` | R-010 | documented contract, doc drift | All four sites updated | Documentation diff | Covered |
| `ondisk.go:133-136` full scan | R-007 | full scan, all divergences | Three divergences in one run | Count assertion test | Covered |
| `skill-package-manager/specs/spec.md:225-226` R-034 standalone | R-001 | standalone command, no apply/capture wiring | Consumer table row 9 | Design consumer table | Covered |
| `anti-generic-design/spec.md:120` green gates over undeployed skill | R-005, R-011 | undeployed skill, green gate | Unregistered file → exit 1 on real tree | Core test + real-tree test | Covered |

## Atomic Requirement Order

| Order | Requirement | Type | Size | Depends On | Why This Order |
|---:|---|---|---|---|---|
| 1 | R-001 Exit-code consumer enumeration | technical debt | small | — | Blocking gate. The meaning of `exit 1` must be understood across all consumers before any code emits a new class. |
| 2 | R-002 Explicit skills-directory input | feature | small | R-001 | Determines the CLI surface. Consumer 1 and 2 in R-001's table both hinge on this decision. |
| 3 | R-003 Named directory-walk seam | technical debt | small | R-002 | Depends on knowing what the core receives before deciding how it receives it. |
| 4 | R-004 Infra-exclusion divergence pinned | technical debt | small | — | Independent, but must land before any code touches either parser, so the regression cannot be introduced unnoticed. |
| 5 | R-005 UNREGISTERED_ON_DISK gate | feature | small | R-002, R-003, R-004 | The primary user-visible behaviour named by the spec. |
| 6 | R-006 MISSING_ON_DISK gate | feature | small | R-005 | Symmetric direction; reuses the wiring from R-005. |
| 7 | R-007 Full-scan reporting | feature | small | R-005, R-006 | Only meaningful once both classes can fire. |
| 8 | R-008 Existing behaviour unchanged | technical debt | small | R-005, R-006 | Regression guard verified once the new paths exist. |
| 9 | R-009 Actionable remediation text | feature | small | R-005, R-006 | Diagnostic wording depends on the final diagnostics. |
| 10 | R-010 Documented exit contract | feature | small | R-001, R-005, R-006 | Documentation reflects the settled contract. |
| 11 | R-011 Real-tree acceptance | feature | small | R-002…R-010 | Final acceptance; proves the whole wiring against the real repository. |

## Manifest Inputs

### Mission / Outcome

A file added under `skills/` without an `overlay.manifest` row must fail a gate that already runs,
rather than silently reaching no runtime target while every check reports green. The obligation
already exists as a spec rule; this change makes the rule self-enforcing so it no longer depends on
a human remembering it.

### Product Rules

- The registration obligation is enforced by `skills validate` — the command the spec already names — and by no other command. `openspec/changes/skill-package-manager/specs/spec.md:225-226` (R-034) forbids wiring it into `cmd_apply`, `cmd_capture`, or any other existing command.
- Adding a divergence class to `skills validate` changes what a non-zero exit means; every consumer of that exit code is enumerated with its impact before the class is emitted.
- The two infra-exclusion rule sets (`engine/skills/manifest.go:13` and `engine/skills/ondisk.go:70`) serve different consumers and are not unified without proving what each consumer requires.
- The skills directory is always an explicit input; it is never inferred from the process working directory.
- The gate must pass against this repository's own `overlay.manifest` and `skills/` tree, not only fixtures.
- Existing registry/manifest divergence behaviour, diagnostics, and exit codes are unchanged.
- Diagnostics name a remediation that can actually resolve the reported divergence.

### Non-Goals

- Extending `sync-manifest` to generate non-`SKILL.md` rows.
- Unifying, refactoring, or deduplicating the two manifest parsers.
- Changing `route_resolve` in `bin/labdrian-overlay` or the deployment routing it defines.
- Adding the on-disk check to `apply`, `capture`, `sync-check`, or any git hook.
- Changing the four existing divergence classes or their diagnostic format.
- Adding new rows to `overlay.manifest` as part of this change (the tree already passes; see R-011).

### Success Criteria

- A new file under `skills/` with no manifest row fails `skills validate` with exit 1 and a named path, with no human step involved.
- A manifest row whose file was deleted or renamed fails `skills validate` with exit 1 and a named path.
- The unmodified CI step at `.github/workflows/ci.yml:156-158` exits 0 against the repository's real tree.
- Every consumer of the `skills validate` exit code has a recorded impact assessment and an explicit change/no-change disposition.
- No existing `skills validate` behaviour or documented contract regresses.

### Risk Of Doing Nothing

`openspec/specs/anti-generic-design/spec.md:120` records this failure already occurring: a skill
registered nowhere, undeployed to all three runtimes, while `apply`, `sync-check`, and `skills
validate` all reported green. The enforcement logic exists, is tested, and runs nowhere, so the
repository currently pays the maintenance cost of the code without receiving any of its protection.

## Architecture Inputs

| Requirement | Affected Domains | Likely Surfaces | Data/Permission Impact | Risk | Sizing Notes |
|---|---|---|---|---|---|
| R-001 | Delivery contract, CI, TUI, docs | Design artifact; consumer table | None (analysis output) | Medium — an unenumerated consumer is exactly this repo's recurring miss | Small; blocking, must precede code |
| R-002 | CLI surface, path resolution | `engine/skills/skills.go:57-98`; possibly `bin/labdrian-overlay:1544-1564` | None | High — a cwd-derived default silently scans `engine/skills/` (28 Go files) | Small once the input is chosen; `--source-root` is already injected at `bin/labdrian-overlay:1559` but ignored |
| R-003 | Go package structure, testability | `engine/skills/skills.go`, `engine/skills/ondisk.go:89` | None | Medium — a wrong seam locks in an untestable core | Small; design decision dominates the cost |
| R-004 | Manifest parsing | `engine/skills/manifest.go:13`, `engine/skills/ondisk.go:66-72`, a new pin test | None | High if violated — silently drops 8 `_shared/*` rows from the check | Small; test-only, no production change |
| R-005 | Validation core, CLI exit codes | `engine/skills/skills.go`, `engine/skills/ondisk.go:137-174` | None | Medium — a false positive breaks CI for everyone | Small; `DiffOnDisk` already implements the logic |
| R-006 | Validation core, CLI exit codes | Same as R-005 | None | Low — symmetric direction, same wiring | Small |
| R-007 | Diagnostic output | `engine/skills/skills.go:89-96` output loop | None | Low | Small |
| R-008 | Existing validation contract | `engine/skills/validate.go`, existing tests | None | Medium — a subtle output change breaks consumers 3, 4, 5, 6 | Small; guarded by unmodified existing tests |
| R-009 | Diagnostic text, docs | `engine/skills/ondisk.go:147-170`; `openspec/changes/skill-manifest-gen/proposal.md:15,67` | None | Low | Small |
| R-010 | Documentation | `README.md:93,298`; `bin/labdrian-overlay:149`; `engine/cmd/main.go:41-43` | None | Low | Small; four sites |
| R-011 | CI, real-tree acceptance | `.github/workflows/ci.yml:156-158`; a new real-tree test in `engine/skills/` | None | High — a failing gate against its own repo would be disabled on first contact | Small; baseline already verified clean (57 = 57) |

**Unknowns that block safe sizing:** none blocking. The two open design decisions (R-002 mechanism,
R-003 seam) are bounded to a single Go file each and do not change the estimate materially.

## Open Questions

| Question | Why it blocks | Suggested owner |
|---|---|---|
| Which input carries the skills-directory path — reuse of the already-injected `--source-root`, derivation from `dirname(--manifest)`, or a new flag? | Determines whether `.github/workflows/ci.yml:158` must change; the wrapper already injects `--source-root` but CI does not | sdd-design (R-002) |
| What happens when the skills directory input is absent or unresolvable — fail loud, or skip the on-disk check? | Silent skip would reproduce the exact "green gate over an undeployed skill" failure this change exists to prevent | sdd-design (R-002) |
| Which seam supplies the directory walk to `RenderValidateCore` — pre-scanned paths, an injected scan function, or another seam? | Determines whether the new core paths are testable without a real filesystem | sdd-design (R-003) |

## Out of Scope

| Item | Reason |
|---|---|
| Extending `sync-manifest` to emit non-`SKILL.md` rows | `engine/skills/sync.go:19-44` is `SKILL.md`-only by design; changing it is a separate change with its own consumers. R-009 corrects the documentation claim instead. |
| Unifying `infraPrefixes` (`manifest.go:13`) with the `engine`-only rule (`ondisk.go:70`) | The divergence is load-bearing; R-004 pins it. Unification requires proving what each consumer needs, which is not this change's job. |
| Wiring the on-disk check into `apply`, `capture`, `sync-check`, or git hooks | Forbidden by `openspec/changes/skill-package-manager/specs/spec.md:225-226` (R-034). |
| Adding or reorganising rows in `overlay.manifest` | The real tree already passes (57 deployable rows, 57 on-disk files, zero divergence, measured 2026-08-05). |
| Changing `route_resolve` or deployment routing in `bin/labdrian-overlay` | `DeployableManifestPaths` mirrors it (`engine/skills/ondisk.go:29-34`); changing the source of truth is out of scope. |
| Reporting dot-prefixed files under `skills/` | `ScanSkillFiles` skips them by design (`engine/skills/ondisk.go:105-111`); nothing the overlay deploys is dot-prefixed. |

## SDD Change Candidates

| Change ID | Requirement IDs | Goal | Keywords | Depends On | Risk | Suggested first phase |
|---|---|---|---|---|---|---|
| `skills-validate-ondisk-gate` | R-001…R-011 | Wire the existing on-disk cross-check into `skills validate` so unregistered `skills/` files and orphan manifest rows fail the gate, with the exit-code blast radius enumerated first | UNREGISTERED_ON_DISK, MISSING_ON_DISK, DeployableManifestPaths, ScanSkillFiles, DiffOnDisk, source-root, infraPrefixes, `_shared`, exit code consumers, real tree | None (self-contained) | Medium — small diff, wide exit-code blast radius | `sdd-propose` |

Single change unit. The eleven requirements are tightly coupled through one command's exit-code
contract; splitting them would ship a half-wired gate or an unenumerated consumer set.

## Project-Inception Handoff

| Roadmap Order | Requirement ID | Manifest Input | Architecture Input | Suggested SDD Change | Required Keywords In SDD | Minimum PASS Evidence |
|---:|---|---|---|---|---|---|
| 1 | R-001 | Rule: exit-code meaning changes require enumerated consumers | Delivery contract, CI, TUI, docs | `skills-validate-ondisk-gate` | exit code, consumer enumeration, blast radius | Design consumer table with `path:line` and per-consumer disposition, re-derived by a reviewer |
| 2 | R-002 | Rule: skills directory is always an explicit input | CLI surface, path resolution | `skills-validate-ondisk-gate` | source-root, cwd, path resolution, engine/skills collision | Core test proving resolution from ≥3 cwd values; fail-loud on absent input |
| 3 | R-003 | Rule: injected seams keep the core testable | Go package structure, testability | `skills-validate-ondisk-gate` | injected seam, ScanSkillFiles, RenderValidateCore | Design names one seam with rejected alternatives; core tests run without walking the real tree |
| 4 | R-004 | Rule: divergent infra rules are not unified without consumer proof | Manifest parsing | `skills-validate-ondisk-gate` | `_shared`, infraPrefixes, unification trap | Bidirectional pin test failing on either unification direction |
| 5 | R-005 | Mission: unregistered file must fail a gate that already runs | Validation core, exit codes | `skills-validate-ondisk-gate` | UNREGISTERED_ON_DISK, missing manifest row | Test: unregistered file → named diagnostic + exit 1; row added → exit 0 |
| 6 | R-006 | Mission: orphan row must fail the same gate | Validation core, exit codes | `skills-validate-ondisk-gate` | MISSING_ON_DISK, orphan row | Test: row with no file → named diagnostic + exit 1 |
| 7 | R-007 | Rule: diagnostics report the full scan | Diagnostic output | `skills-validate-ondisk-gate` | full scan, all divergences | Test asserting three divergences in one run |
| 8 | R-008 | Rule: existing behaviour must not drift | Existing validation contract | `skills-validate-ondisk-gate` | regression guard, MISSING_IN_MANIFEST, exit 0 | Existing tests pass with no assertion edits; aligned-path success test |
| 9 | R-009 | Rule: diagnostics name a remediation that works | Diagnostic text, docs | `skills-validate-ondisk-gate` | sync-manifest limitation, remediation | Diagnostic-text test; corrected remediation claim |
| 10 | R-010 | Rule: the documented contract matches the real contract | Documentation | `skills-validate-ondisk-gate` | README, help text, exit 1 on any divergence | Diff of four documentation sites + `--help` output |
| 11 | R-011 | Success criterion: the overlay's own tree passes | CI, real-tree acceptance | `skills-validate-ondisk-gate` | real tree, 80 rows, 57 deployable, dogfooding | Real-tree Go test at zero divergences + green unmodified CI step |

## Verification Gate

- [ ] Every stakeholder anchor is mapped in the traceability matrix or listed in Out of Scope.
- [ ] Contradictions and ambiguities were analyzed before requirements were finalized (I-01…I-04).
- [ ] I-01 and I-02 are carried as named design outputs (R-003, R-002), not silently decided; I-03 and I-04 are resolved from verified code evidence.
- [ ] Every requirement has at least one acceptance scenario.
- [ ] Every requirement is atomic and small; none is marked "must split".
- [ ] Every requirement carries stakeholder-language and technical keywords.
- [ ] Atomic requirements are ordered for project-inception, dependencies first.
- [ ] Manifest Inputs capture mission, product rules, non-goals, success criteria, and risk of doing nothing.
- [ ] Architecture Inputs capture affected surfaces, risk, and sizing notes for every requirement.
- [ ] SDD Roadmap Inputs preserve R-001…R-011 verbatim instead of collapsing them into themes.
- [ ] Every scenario names an evidence type and a target surface.
- [ ] R-001's consumer enumeration is re-derived and closed before any code emits a new divergence class.
- [ ] R-011's real-tree acceptance passes before PASS is claimed; fixtures alone are insufficient.
- [ ] Strict TDD: every behavioural requirement (R-005…R-009, R-011) has a failing test written before its implementation.
- [ ] No requirement's PASS is claimed from an unchanged-file argument alone.
