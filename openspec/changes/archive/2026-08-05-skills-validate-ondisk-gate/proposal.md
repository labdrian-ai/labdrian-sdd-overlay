# Proposal: skills-validate-ondisk-gate

## Intent

`openspec/changes/deterministic-verification-evidence/specs/verification-evidence-capture/spec.md:138-146` requires every new `skills/` file to carry a deploying `overlay.manifest` row and names `skills validate` as the enforcing check — but that command never compares registry/manifest against the filesystem. The cross-check already exists, fully unit-tested, in `engine/skills/ondisk.go` (`DeployableManifestPaths` :48, `ScanSkillFiles` :89, `DiffOnDisk` :137, classes `UNREGISTERED_ON_DISK`/`MISSING_ON_DISK`) with zero non-test callers (settled: obs #2763, #2767). `openspec/specs/anti-generic-design/spec.md:120` records the failure already happening: a skill registered nowhere, undeployed to all three runtimes, while `apply`, `sync-check`, and `skills validate` all reported green. Wire the check into `skills validate` so the rule is machine-enforced.

## Scope

### In Scope — two chained slices (frozen in entry contract obs #2774; do not re-derive)

| # | Slice | Requirements | Lines | Depends |
|---|---|---|---|---|
| 1 | `ondisk-gate-consumer-contract`: exit-code consumer enumeration, bidirectional infra-rule pin test, corrected `sync-manifest` remediation claim, documented exit contract. Emits NO new divergence class; `skills validate` behavior unchanged after this slice. | R-001, R-004, R-009, R-010 | 120-260 | — |
| 2 | `ondisk-gate-wiring`: resolve skills-directory input and walk seam, wire `DiffOnDisk` into `RenderValidateCore`, emit `UNREGISTERED_ON_DISK`/`MISSING_ON_DISK` in full-scan form, prove the overlay's real tree still exits 0. | R-002, R-003, R-005–R-008, R-011 | 400-715 | Slice 1 |

Slice 1 precedes slice 2 because R-001 requires the consumer enumeration persisted before any code path emits the new class.

### Out of Scope

Extending `sync-manifest`; unifying the two parsers; changing `route_resolve`; wiring the check into apply/capture/sync-check/hooks (forbidden by `openspec/changes/skill-package-manager/specs/spec.md:225-226`, R-034); changing the four existing divergence classes (`MISSING_IN_MANIFEST`, `MISSING_IN_REGISTRY`, `TAG_MISMATCH`, `MIXED_TAG`); adding manifest rows.

## Capabilities

### New Capabilities
- `skills-ondisk-validation`: `skills validate` cross-checks the `skills/` tree against deployable `overlay.manifest` rows in both directions, full-scan, exit 1 on any divergence.

### Modified Capabilities
- `skill-package-manager`: validate exit contract gains the two on-disk classes; the four existing classes, diagnostic format, and exit codes are pinned unchanged (R-008); validate remains standalone.

## Approach

Reuse the tested `ondisk.go` functions as-is; only the two `Detail` format strings change (R-009). Hard constraints that must survive into design and apply:

1. **Do NOT unify the two infra-exclusion rule sets.** `engine/skills/manifest.go:13` excludes `engine/` AND `_shared/`; `engine/skills/ondisk.go:66-72` excludes only first-segment `engine`. `overlay.manifest` carries 8 real deployable `_shared/*` rows; unifying onto manifest.go's list silently drops all 8 with every existing test still green. R-004 pins this bidirectionally.
2. **Exit-code blast radius is 9 consumers, not 3**: CI `.github/workflows/ci.yml:156-158`; `bin/labdrian-overlay:1544-1564` and help text `:149`; TUI read-only "Skills" entry `tui/run.go:80-86` whose worst-severity aggregator (`:368-377`, `:387-432`) renders exit 1 as hard failure; `engine/cmd/main.go:41-43`; `README.md:93,298`; prior acceptance gates `openspec/changes/skill-project-scope/tasks.md:226` and `openspec/changes/deterministic-verification-evidence/tasks.md:314`; `openspec/changes/skill-manifest-gen/proposal.md:15,67`.
3. **The documented remediation is INVALID — correct it, never cite it.** `skill-manifest-gen/proposal.md:15,67` claims `sync-manifest` fixes a drifted manifest, but `engine/skills/sync.go:19-44` `isSkillRow` only regenerates `*/SKILL.md` rows and cannot clear an `UNREGISTERED_ON_DISK` for a reference or `_shared/*` file (R-009).
4. **`skills validate` stays standalone** — no wiring into any other command.
5. **cwd trap**: `engine/skills/` is a real 28-file Go package. A cwd-derived `./skills` default from CI's `working-directory: engine` would flag every `.go` file. The two invocation paths already differ: `bin/labdrian-overlay:1559` injects `--source-root` (silently ignored by `engine/skills/skills.go:60-73`); CI passes none.
6. **Real-tree baseline (measured 2026-08-05)**: 80 manifest rows → 57 deployable; 57 non-dot files under `skills/`; zero divergences both directions. Lands with NO manifest edits; `TestRepositorySkillsAreFullyRegistered` (`engine/skills/ondisk_test.go:178-204`) already covers much of R-011.

### Forwarded design decisions — named here, resolved by sdd-design on evidence (auto mode, no human checkpoint)

- **R-002**: which input carries the skills-directory path, and what happens when it is absent. If fail-loud-on-absent, `.github/workflows/ci.yml:157` becomes a mandatory in-slice edit.
- **R-003**: which seam supplies the directory walk to the validate core. `RenderValidateCore` injects `readFile`/`stdout`/`stderr`/`exit` only; `Validate` reads the manifest outside that seam (`engine/skills/validate.go:93-94`, `engine/skills/manifest.go:38-45`); `ScanSkillFiles` calls `os.Stat`/`filepath.WalkDir` directly.

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `engine/skills/skills.go`, `ondisk.go`, tests | Modified | Flag parse, seam, wiring, diagnostics, pin tests |
| `engine/cmd/main.go`, `bin/labdrian-overlay`, `README.md`, `.github/workflows/ci.yml`, `tui/run.go` | Modified | Exit-contract docs, help text, CI flag, TUI disposition |
| `openspec/changes/skill-manifest-gen/proposal.md` | Modified | Correct invalid remediation claim |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| R-002 auto-resolution fires the CI cwd trap | Med | Named design output; CI edit lands in the same slice |
| Infra-rule unification during wiring | Low | R-004 pin test lands in slice 1, before wiring |
| Slice 2 exceeds 800-line budget | Med | Pre-agreed split seam: `ondisk-source-root-and-seam` / `ondisk-divergence-reporting` |
| TUI renders new exit 1 unacceptably on read-only entry | Med | R-001 disposition decided in slice 1 |

## Rollback Plan

Pure git reverts — no data, state, or manifest migration. One PR per slice on a feature-branch chain:

- **Revert slice 2** (gate misfires, e.g. false-positive flood in CI): revert the slice-2 PR on the tracker branch. This restores today's exit contract; the CI `--source-root` edit reverts atomically with it. Slice 1's R-010 doc lines would then overstate the contract, so the revert PR must also re-edit the four doc sites (or revert both PRs).
- **Revert slice 1**: safe independently at any time before slice 2 merges — it changes no runtime behavior.
- The overlay's own tree passes at zero divergences today, so no consumer needs manifest remediation to survive a rollback in either direction.

## Dependencies

- `engine/skills/ondisk.go` + `ondisk_test.go` as on `main` after `deterministic-verification-evidence` (obs #2767). No upstream `gentle-ai` change required or permitted.

## Success Criteria

- [ ] R-001: enumeration of 9 consumers (path:line + per-consumer disposition) persisted before any emitting code path
- [ ] R-004: bidirectional pin test fails on either unification direction
- [ ] R-005: unregistered file → `UNREGISTERED_ON_DISK` diagnostic + exit 1; adding the row → exit 0
- [ ] R-006/R-007: orphan deploying row → `MISSING_ON_DISK` + exit 1; all divergences reported in one run
- [ ] R-008: existing validate tests pass with no assertion edits
- [ ] R-009/R-010: diagnostics name a working manifest-row remediation; four documentation sites updated
- [ ] R-011: real tree exits 0 with zero manifest edits and green CI
