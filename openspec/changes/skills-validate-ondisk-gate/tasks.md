# Tasks: skills-validate-ondisk-gate

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | Slice 1: 120-260. Slice 2: 400-735 (post Decision-5 amendment) |
| 400-line budget risk | Low (slice 1) / High (slice 2) |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 (`ondisk-gate-consumer-contract`) → PR 2 (`ondisk-gate-wiring`) |
| Delivery strategy | auto-chain |
| Chain strategy | feature-branch-chain |
| Project review budget override | 800 lines/slice per entry.json — slice 2's high end (~735) leaves ~8% headroom |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: High

If slice 2 implementation crosses 800 lines, split at the pre-agreed seam without asking: `ondisk-source-root-and-seam` (R-002, R-003, R-008, plus `ci.yml`/`spec.md`/`skills_test.go` arg edits) → `ondisk-divergence-reporting` (R-005, R-006, R-007, R-011, plus the `skills_test.go` fixture file).

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Consumer enumeration, R-004 pin, R-009 remediation fix, R-010 docs — zero behavior change | PR 1 (base: tracker branch) | `cd engine && go test ./skills/...` | N/A — no runtime behavior changes this slice | Revert independently any time before slice 2 merges |
| 2 | `--source-root` flag, scan seam, wiring, full-scan emission, real-tree proof | PR 2 (base: PR 1 branch) | `cd engine && go test ./skills/... && cd engine && go test ./cmd/...` | `go run ./cmd/ skills validate --registry skills.registry.yaml --manifest overlay.manifest --source-root skills` from repo root, exit 0 | Revert restores today's exit contract; revert `ci.yml`/spec-scenario/test-vector edits atomically with it |

## Phase 1: Slice 1 — ondisk-gate-consumer-contract (R-001, R-004, R-009, R-010)

- [x] 1.1 R-004: add bidirectional pin test to `engine/skills/ondisk_test.go` (fixture rows `_shared/x.md`, `_shared/pin/SKILL.md`, `engine/y.go`, `real-skill/SKILL.md`): Subtest A asserts `DeployableManifestPaths` includes both `_shared` rows and excludes `engine/y.go`; Subtest B asserts `LoadManifestView` yields `real-skill` but not `_shared`. Confirm it fails if either exclusion rule is unified.
- [x] 1.2 R-009: rewrite the two `Detail` strings in `engine/skills/ondisk.go` (`DivUnregisteredOnDisk`/`DivMissingOnDisk`) to name the manifest-row edit, never `sync-manifest`; add table-driven assertions in `ondisk_test.go`.
- [x] 1.3 R-009: correct the invalid `sync-manifest` remediation claim in `openspec/changes/skill-manifest-gen/proposal.md:15,67` — scope it to `*/SKILL.md` rows only.
- [x] 1.4 R-010 (R-001 disposition, this is Decision 4 — the design table itself is the persisted enumeration, already saved): update five doc sites to describe the on-disk cross-check: `README.md:93,298` (add `--source-root` to the usage signature), `bin/labdrian-overlay:149`, `engine/cmd/main.go:41-43,173` (add `--source-root <path>` to the validate usage line).
- [x] 1.5 Run `cd engine && go test ./skills/...`; confirm no assertion edits to existing tests (R-008 baseline).

## Phase 2: Slice 2 — ondisk-gate-wiring (R-002, R-003, R-005, R-006, R-007, R-008, R-011)

- [ ] 2.1 RED — add stubbed-`scanSkills` core tests in `engine/skills/skills_test.go`: absent `--source-root` → exit 1 naming the flag; unregistered file → diagnostic + exit 1, row added → exit 0; orphan row → exit 1; ≥3 mixed divergences in one run; cwd-independence via `os.Chdir` + `t.Cleanup` across 3 dirs with an absolute `--source-root` (precedent `engine/runtime/opencode_test.go:383-386`; never `t.Chdir`, Go 1.24-only).
- [ ] 2.2 GREEN — in `engine/skills/skills.go`: parse `--source-root` in `RenderValidateCore`'s flag loop (absent → stderr `error: skills validate requires --source-root <skills-dir>` + exit 1); add `scanSkills func(string) ([]string, error)` parameter; wire `readFile(manifestPath)` into `DeployableManifestPaths` and `scanSkills(root)` into `DiffOnDisk`; print on-disk divergences after existing ones, same run (R-007); update `SkillsCore`'s `validate` dispatch to pass `ScanSkillFiles`.
- [ ] 2.3 Update `verb_validate` in `engine/skills/skills_test.go:73-113`: add `--source-root` pointing at a dedicated subdirectory (write `dir/skills/sdd-spec/SKILL.md`, pass `--source-root dir/skills`) — do NOT reuse the temp dir holding `registry.yaml`/`overlay.manifest`, else `ScanSkillFiles` also picks those up as `UNREGISTERED_ON_DISK`. No assertion-statement edits.
- [ ] 2.4 Update `.github/workflows/ci.yml:158` — add `--source-root ../skills` to the `run:` line.
- [ ] 2.5 Decision 5 — locate the scenario by text (not path:line) at `openspec/changes/deterministic-verification-evidence/specs/verification-evidence-capture/spec.md`; if that folder has moved to `openspec/specs/verification-evidence-capture/spec.md`, edit there instead, never under `archive/`. Edit the GIVEN ("a new file under `skills/`" → also states it is registered with a manifest row) and the WHEN (add `--source-root ../skills` to the invocation).
- [ ] 2.6 R-011 — verify `TestRepositorySkillsAreFullyRegistered` (`engine/skills/ondisk_test.go:178-204`) passes; add/confirm a command-level run of `skills validate` against the real repo tree exits 0 with no manifest edits.
- [ ] 2.7 Run focused (`cd engine && go test ./skills/...`, `cd engine && go test ./cmd/...`) then broad (`cd engine && go test ./...`, `cd tui && go test ./...`, `tools/*/go.mod` loop) test commands; confirm green CI with the unmodified `ci.yml` step.
