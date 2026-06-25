# Tasks: skill-package-manager (First Slice — Read-Only Semantic Layer)

Generated: 2026-06-25
Spec: openspec/changes/skill-package-manager/specs/spec.md
Design: openspec/changes/skill-package-manager/design.md
Delivery strategy: ask-on-risk · Chain strategy: feature-branch-chain

---

## Execution notes

- STRICT TDD: every Go file listed below has its `_test.go` authored and passing **before**
  the implementation file is written. Tests and implementation ship in the same commit
  (work-unit-commits contract).
- ADR-1 is a hard constraint: no `gopkg.in/yaml.v3` or any new Go module dependency.
- ADR-5 is a hard constraint: `skills.registry.yaml` lives at repo root; no manifest row
  for it except the explicit `custom` tracking row added in T-20.
- `bin/overlay` is NEVER touched (R-038, R-041).
- All new Go code in `engine/skills/` and `engine/cmd/main.go` only (R-041).

---

## Dependency graph

```
T-01 ──► T-02 ──► T-03 ──► T-04 ──► T-05
                                      │
           ┌───────────────────────────┘
           │
           ▼
         T-06 ──► T-07 ──► T-08 ──► T-09
                                      │
                   ┌──────────────────┘
                   │
                   ▼
                 T-10 ──► T-11 ──► T-12 ──► T-13 ──► T-14 ──► T-15
                                                                  │
                                             ┌────────────────────┘
                                             │
                                             ▼
                                           T-16 ──► T-17 ──► T-18
                                                               │
                                                    ┌──────────┘
                                                    │
                                                    ▼
                                                  T-19 ──► T-20
```

Parallel opportunity: T-06..T-09 can begin immediately after T-01 (types only needed);
T-02..T-05 and T-06..T-09 are independent chains that converge at T-10. In practice,
complete the parser chain first to avoid blocked validate tests.

---

## PR-1 — Zero-dep YAML subset parser

Target branch: `feat/skill-package-manager`
All tasks sequential.

### T-01 · Types skeleton
**Files**: `engine/skills/types.go` (new)
**Spec**: R-001..R-012 (type contract), A-1
**Work**: Define Go structs: `Registry{Version string; Skills []Entry}`,
`Entry{ID, Path string; Source Source; Install Install; Lifecycle Lifecycle}`,
`Source{Type string; Upstream *Upstream}`, `Upstream{Owner string}`,
`Install{DefaultScope string; Targets []string}`, `Lifecycle{UpdateStrategy string}`.
No logic — pure data declarations. No test file needed for bare struct types.
**Commit**: `feat(skills): add engine/skills types skeleton`

### T-02 · Parser test suite (TDD — write first)
**Files**: `engine/skills/parse_test.go` (new), `engine/skills/testdata/` (new dir)
**Spec**: SC-01, SC-02, SC-03, SC-04, SC-05 + R-001..R-012
**Work**: Table-driven `TestParseRegistry` covering:
- `valid_core_and_custom` (SC-01): 2 entries, declaration order preserved.
- `missing_id` (SC-02): error non-nil, entries nil/empty.
- `unknown_source_type` (SC-03): `source.type: external` → error.
- `duplicate_id` (SC-04): error message includes the conflicting id.
- `upstream_on_custom_entry` (SC-05): custom entry with upstream block → error.
- `missing_version`: top-level version absent → error.
- `wrong_version`: `version: "2"` → error.
- `empty_targets`: empty `install.targets` → error (R-007).
- `invalid_target`: `install.targets: [vscode]` → error (R-008).
- `invalid_scope`: `install.defaultScope: project` → error (R-006).
- `invalid_lifecycle`: `updateStrategy: rolling` → error (R-009).
- `malformed_yaml_tab`: file uses literal tab indentation → error (out-of-subset).
- `malformed_yaml_flow`: file uses `{key: val}` flow mapping → error.
- `empty_file`: `io.Reader` yielding `""` → error (missing version).
- Golden file test for deterministic entry order (SC-01 output shape).
Add YAML fixture files under `engine/skills/testdata/`.
**Commit**: `test(skills): add ParseRegistry table tests and YAML fixtures`

### T-03 · YAML subset parser implementation
**Files**: `engine/skills/parse.go` (new)
**Spec**: R-001..R-012, ADR-1
**Work**: Implement `ParseRegistry(r io.Reader) (Registry, error)`.
Hand-rolled strict-subset parser:
- Accepted: full-line comments (`#`), `key: value`, nested maps by 2-space indentation,
  block sequences (`- `), plain and single/double-quoted scalars.
- Rejected with line-numbered error: tab indentation, flow `{}`/`[]`, anchors (`&`/`*`),
  tags (`!`), block scalars (`|`/`>`), multi-doc (`---` after the first).
Post-parse validation: version == "1", unique ids, source.type in {core,custom},
upstream.owner non-empty if upstream present, upstream absent if custom,
defaultScope == "global", targets non-empty and each in {claude,opencode,codex},
updateStrategy in {vendor-merge,overlay-only}.
`// minimal: forced — ADR-1 (zero-dep invariant)`
All T-02 tests must pass before merge.
**Commit**: `feat(skills): implement hand-rolled YAML subset parser`

### T-04 · Load adapter tests (TDD — write first)
**Files**: `engine/skills/load_test.go` (new)
**Spec**: R-022 (fail-loud on unreadable)
**Work**: Tests for `Load(path string)` using `t.TempDir()`:
- `valid_file`: writes a minimal valid YAML, expects nil error and correct entry count.
- `missing_file`: path does not exist → error non-nil.
- `invalid_yaml`: malformed YAML in temp file → error non-nil.
**Commit**: `test(skills): add Load adapter tests`

### T-05 · Load adapter implementation
**Files**: `engine/skills/load.go` (new)
**Spec**: R-022, R-023 (default path context)
**Work**: Implement `Load(path string) (Registry, error)` — opens file, defers close,
calls `ParseRegistry`. No logic beyond the file boundary.
All T-04 tests must pass before merge.
**Commit**: `feat(skills): add Load file adapter`

---

## PR-2 — ManifestView + Validate

Target branch: PR-1 branch (feature-branch-chain: each PR targets the previous PR branch)
All tasks sequential.

### T-06 · ManifestView tests (TDD — write first)
**Files**: `engine/skills/manifest_test.go` (new), additional testdata fixtures
**Spec**: R-013..R-016, ADR-3 (INFRA_PREFIXES)
**Work**: Tests for `LoadManifestView(path string)` using `t.TempDir()`:
- `skill_dirs_extracted`: manifest with `sdd-spec/SKILL.md managed`,
  `gadu-orchestrate/SKILL.md custom` → view has entries for both with correct tags.
- `infra_prefixes_excluded`: rows `engine/cmd/main.go managed`,
  `_shared/minimalism-contract.md custom` → NOT in view.
- `non_skill_md_rows_ignored`: `genesis-design-system/references/forms.md custom` →
  NOT a separate entry; parent dir entry from `genesis-design-system/SKILL.md` is kept.
- `managed_maps_to_core`: `managed` tag in manifest → `source.type == "core"`.
- `custom_maps_to_custom`: `custom` tag → `source.type == "custom"`.
- `missing_file`: path absent → error non-nil.
**Commit**: `test(skills): add LoadManifestView tests`

### T-07 · ManifestView implementation
**Files**: `engine/skills/manifest.go` (new)
**Spec**: R-013..R-016, ADR-3
**Work**: Define `ManifestEntry{Dir, Tag string}` and `ManifestView map[string]ManifestEntry`.
Implement `LoadManifestView(path string)(ManifestView, error)`:
- Read file line by line; parse `<path> <tag>` rows; skip blank and `#` lines.
- Identify skill dirs: rows where path matches `*/SKILL.md`; extract first path component as dir.
- Exclude dirs matching INFRA_PREFIXES = `{"engine/", "_shared/"}`.
- Collapse multiple rows for the same dir to one entry (first SKILL.md row wins for tag).
All T-06 tests must pass before merge.
**Commit**: `feat(skills): implement ManifestView manifest parser`

### T-08 · Validate tests (TDD — write first)
**Files**: `engine/skills/validate_test.go` (new), additional testdata fixtures
**Spec**: SC-09, SC-10, SC-11, SC-12 + R-028..R-035
**Work**: Table-driven tests for `Diff(reg Registry, mv ManifestView)[]Divergence`:
- `aligned_registry_and_manifest` (SC-09): 2 entries, both covered → `[]Divergence{}`.
- `registry_entry_not_in_manifest` (SC-10): registry has `new-skill`, manifest does not →
  divergence with class `MISSING_IN_MANIFEST`, path contains `"new-skill"`.
- `manifest_skill_not_in_registry` (SC-11): manifest has `orphan-skill/SKILL.md`, registry
  has no entry → divergence with class `MISSING_IN_REGISTRY`, path `"orphan-skill"`.
- `non_skill_rows_ignored` (SC-12): engine and _shared non-SKILL.md rows present → no
  extra divergences (INFRA_PREFIXES filter works end-to-end).
- `tag_mismatch`: registry `source.type: core` but manifest tag `custom` → `TAG_MISMATCH`
  divergence (ADR-3).
Also test `Validate(reg, manifestPath)` integration: aligned → nil error; diverged → error
with divergence list in message.
**Commit**: `test(skills): add Diff and Validate tests`

### T-09 · Validate implementation
**Files**: `engine/skills/validate.go` (new)
**Spec**: R-028..R-035, ADR-3
**Work**: Define `DivergenceClass` enum string (MISSING_IN_MANIFEST, MISSING_IN_REGISTRY,
TAG_MISMATCH, MIXED_TAG) and `Divergence{Class DivergenceClass; Path, Detail string}`.
Implement `Diff(reg Registry, mv ManifestView)[]Divergence`:
- For each registry entry: check `mv[entry.Path]` exists; if not → MISSING_IN_MANIFEST.
- Check tag alignment (managed↔core, custom↔custom); mismatch → TAG_MISMATCH.
- For each manifest dir: check registry has entry with `path == dir`; if not → MISSING_IN_REGISTRY.
Implement `Validate(reg Registry, manifestPath string) ([]Divergence, error)`:
- Loads ManifestView, calls Diff, returns divergences + non-nil error if any found.
All T-08 tests must pass before merge.
**Commit**: `feat(skills): implement Diff and Validate cross-check`

---

## PR-3 — CLI commands + engine wiring + bash passthrough

Target branch: PR-2 branch
All tasks sequential.

### T-10 · RenderList tests (TDD — write first)
**Files**: `engine/skills/list_test.go` (new), golden files under `testdata/`
**Spec**: SC-06, SC-07 + R-019..R-023
**Work**:
- `deterministic_output` (SC-06): inject registry with 3 entries (alphabetically unsorted ids),
  call `RenderListCore` twice, assert outputs byte-identical, line count == 3, each line
  contains id + source.type + updateStrategy + targets. Use golden file.
- `missing_registry_file` (SC-07): inject file-reader returning os.ErrNotExist → exit code 1,
  stderr non-empty, stdout empty.
- `invalid_registry`: inject file-reader returning bad YAML → exit 1.
**Commit**: `test(skills): add RenderList tests and golden files`

### T-11 · RenderList implementation
**Files**: `engine/skills/list.go` (new)
**Spec**: R-019..R-023
**Work**: Implement `RenderListCore(args []string, readFile readFileFn, stdout, stderr io.Writer, exit func(int))`:
- Parse `--registry <path>` flag; default `skills.registry.yaml`.
- Load via `ParseRegistry(readFile(...))`.
- Output sorted by id (stable sort); one line: `id\ttype\tstrategy\ttargets`.
- Missing/parse error → stderr diagnostic + exit 1.
All T-10 tests must pass before merge.
**Commit**: `feat(skills): implement RenderList`

### T-12 · RenderStatus tests (TDD — write first)
**Files**: `engine/skills/status_test.go` (new)
**Spec**: SC-08 + R-024..R-027
**Work**:
- `count_report` (SC-08): inject registry with 3 core + 15 custom entries → stdout contains
  "18", "3", "15", "OK"; exit 0.
- `empty_registry`: 0 entries → stdout contains "0" total, "OK"; exit 0.
- `missing_file`: readFile returns error → stderr non-empty, exit 1.
- Verify status does NOT open manifest (R-026): inject nil manifest reader; no failure.
**Commit**: `test(skills): add RenderStatus tests`

### T-13 · RenderStatus implementation
**Files**: `engine/skills/status.go` (new)
**Spec**: R-024..R-027
**Work**: Implement `RenderStatusCore(args []string, readFile readFileFn, stdout, stderr io.Writer, exit func(int))`:
- Parse `--registry` flag; default `skills.registry.yaml`.
- Count total, core, custom entries from Registry.
- Print report lines to stdout; final line "Status: OK"; exit 0.
- Read only from registry file (no manifest access).
All T-12 tests must pass before merge.
**Commit**: `feat(skills): implement RenderStatus`

### T-14 · SkillsCore dispatcher tests (TDD — write first)
**Files**: `engine/skills/skills_test.go` (new)
**Spec**: ADR-2, R-039 (read-only invariant enforced by verb set)
**Work**: Tests for `SkillsCore(verb string, args []string, readFile readFileFn, stdout, stderr io.Writer, exit func(int))`:
- `verb_list`: verb "list" with valid registry → exit 0, output has entries.
- `verb_status`: verb "status" with valid registry → exit 0, counts in stdout.
- `verb_validate`: verb "validate" with aligned registry+manifest → exit 0, "OK" in stdout.
- `unknown_verb`: verb "nuke" → exit 1, stderr contains verb name.
- `empty_verb`: empty string → exit 1.
**Commit**: `test(skills): add SkillsCore dispatcher tests`

### T-15 · SkillsCore dispatcher implementation
**Files**: `engine/skills/skills.go` (new)
**Spec**: ADR-2, R-019..R-035
**Work**: Implement `SkillsCore(verb string, args []string, readFile readFileFn, stdout, stderr io.Writer, exit func(int))`:
- Switch on verb: "list" → `RenderListCore`, "status" → `RenderStatusCore`,
  "validate" → `RenderValidateCore` (wraps Validate with flag parsing for --registry/--manifest).
- Empty or unknown verb → fail-loud (stderr + exit 1) matching prespec pattern.
No global state; all I/O injected.
All T-14 tests must pass before merge.
**Commit**: `feat(skills): implement SkillsCore verb dispatcher`

### T-16 · engine/cmd/main.go wiring
**Files**: `engine/cmd/main.go` (modified)
**Spec**: R-019..R-035, ADR-4
**Work**:
- Add `case "skills": runSkills(os.Args[2:])` to main switch.
- Implement `runSkills(args []string)` (mirrors `runPrespec`; calls `runSkillsCore`).
- Implement `runSkillsCore(verb string, args []string, stdout, stderr io.Writer, exit func(int))`
  — calls `skills.SkillsCore`.
- Add `import "github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"`.
- Update `usage()` to add skills verbs line.
- Update package doc comment at top of file.
**Commit**: `feat(engine): wire skills subcommand in cmd/main.go`

### T-17 · engine/cmd/main_test.go tests
**Files**: `engine/cmd/main_test.go` (modified)
**Spec**: R-040..R-042, SC-13
**Work**:
- `TestRunSkillsCore_list`: smoke test — valid registry via temp file → exit 0.
- `TestRunSkillsCore_unknown_verb`: "bogus" → exit 1.
- `TestApplyIgnoresRegistry` (SC-13): integration test tagged `t.Skip` when
  `testing.Short()` — verifies apply-path code does not open `skills.registry.yaml`.
All tests must pass with `go test ./engine/cmd/ -short`.
**Commit**: `test(engine): add skills dispatch and SC-13 regression tests`

### T-18 · bin/labdrian-overlay cmd_skills
**Files**: `bin/labdrian-overlay` (modified)
**Spec**: R-036, R-037, R-038
**Work**:
- Add `cmd_skills()` after `cmd_prespec`; mirror exact pattern (binary check, `exec "$ENGINE_BINARY" skills "$@"`).
- Add dispatch case `skills) cmd_skills "$@" ;;` after `prespec)` case.
- Add usage line: `  skills <verb>                    Forward to the engine's skills registry core`
  with indented verbs: `list`, `status`, `validate`.
- Do NOT touch `bin/overlay`.
**Commit**: `feat(overlay): add cmd_skills passthrough to bin/labdrian-overlay`

---

## PR-4 — Initial registry population + manifest tracking rows

Target branch: PR-3 branch
All tasks sequential.

### T-19 · Populate skills.registry.yaml
**Files**: `skills.registry.yaml` (new, repo root)
**Spec**: R-013..R-018, A-3, A-5, ADR-5
**Work**: Hand-author initial registry from overlay.manifest.
Core skills (managed rows with SKILL.md): `sdd-spec`, `sdd-tasks`, `sdd-verify`.
Custom skills (custom rows with SKILL.md): `gadu-orchestrate`, `requirements-from-transcripts`,
`prespec-malandra`, `kadia-content-guard`, `kadia-ui-fix`, `kadia-visual-qa`,
`genesis-delivery-workflow`, `genesis-design-system`, `chat-thread-analyzer`,
`project-inception`, `inception-pipeline`, `project-manifest`, `project-architect`,
`roadmap-maker`, `sdd-time-estimation`.
Each entry: `id` from dir name, `source.type` from manifest tag,
`upstream.owner: gentleman-programming` for core only,
`lifecycle.updateStrategy: vendor-merge` for core / `overlay-only` for custom,
`install.defaultScope: global`, `install.targets: [claude, opencode, codex]`.
Run `engine skills validate` after authoring; MUST exit 0 before commit.
**Commit**: `feat(skills): add initial skills.registry.yaml (20 skills)`

### T-20 · overlay.manifest tracking rows
**Files**: `overlay.manifest` (modified)
**Spec**: R-043, R-044
**Work**:
- Add one `managed` row per new Go source file under `engine/skills/`:
  `engine/skills/types.go managed`, `engine/skills/parse.go managed`,
  `engine/skills/load.go managed`, `engine/skills/manifest.go managed`,
  `engine/skills/validate.go managed`, `engine/skills/list.go managed`,
  `engine/skills/status.go managed`, `engine/skills/skills.go managed`.
  (Test files are not tracked in overlay.manifest; only production source files.)
- Add `skills.registry.yaml custom` row.
Run `engine skills validate` after edit; MUST exit 0.
**Commit**: `chore(manifest): add engine/skills source rows and skills.registry.yaml tracking`

---

## Summary

| Task | PR Slice | Seq/Parallel | Spec refs | Files |
|------|----------|--------------|-----------|-------|
| T-01 | PR-1 | Sequential | R-001..R-012 | `engine/skills/types.go` |
| T-02 | PR-1 | After T-01 | SC-01..SC-05, R-001..R-012 | `engine/skills/parse_test.go`, testdata/ |
| T-03 | PR-1 | After T-02 | R-001..R-012, ADR-1 | `engine/skills/parse.go` |
| T-04 | PR-1 | After T-03 | R-022 | `engine/skills/load_test.go` |
| T-05 | PR-1 | After T-04 | R-022..R-023 | `engine/skills/load.go` |
| T-06 | PR-2 | After T-05* | R-013..R-016 | `engine/skills/manifest_test.go` |
| T-07 | PR-2 | After T-06 | R-013..R-016, ADR-3 | `engine/skills/manifest.go` |
| T-08 | PR-2 | After T-07 | SC-09..SC-12, R-028..R-035 | `engine/skills/validate_test.go` |
| T-09 | PR-2 | After T-08 | R-028..R-035, ADR-3 | `engine/skills/validate.go` |
| T-10 | PR-3 | After T-09 | SC-06..SC-07, R-019..R-023 | `engine/skills/list_test.go`, testdata/ |
| T-11 | PR-3 | After T-10 | R-019..R-023 | `engine/skills/list.go` |
| T-12 | PR-3 | After T-11 | SC-08, R-024..R-027 | `engine/skills/status_test.go` |
| T-13 | PR-3 | After T-12 | R-024..R-027 | `engine/skills/status.go` |
| T-14 | PR-3 | After T-13 | ADR-2, R-039 | `engine/skills/skills_test.go` |
| T-15 | PR-3 | After T-14 | ADR-2, R-019..R-035 | `engine/skills/skills.go` |
| T-16 | PR-3 | After T-15 | R-019..R-035, ADR-4 | `engine/cmd/main.go` |
| T-17 | PR-3 | After T-16 | R-040..R-042, SC-13 | `engine/cmd/main_test.go` |
| T-18 | PR-3 | After T-17 | R-036..R-037 | `bin/labdrian-overlay` |
| T-19 | PR-4 | After T-18 | R-013..R-018 | `skills.registry.yaml` |
| T-20 | PR-4 | After T-19 | R-043..R-044 | `overlay.manifest` |

*T-06 only requires T-01 (types); but PR-2 starts after PR-1 merges for practical cohesion.

---

## Review Workload Forecast

Estimated changed lines:
- PR-1 (T-01..T-05): ~530 lines (types 50 + parser tests 180 + parser 220 + load tests 50 + load 30)
- PR-2 (T-06..T-09): ~440 lines (manifest tests 100 + manifest 90 + validate tests 140 + validate 110)
- PR-3 (T-10..T-18): ~565 lines (list tests 100 + list 60 + status tests 70 + status 50 + skills_test 90 + skills 70 + main.go 60 + main_test 60 + labdrian-overlay 25)
- PR-4 (T-19..T-20): ~315 lines (registry.yaml 300 + manifest 15)

Total estimated changed lines: ~1850
400-line budget risk: High
Chained PRs recommended: Yes
Decision needed before apply: Yes
