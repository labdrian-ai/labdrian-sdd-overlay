# Tasks: Project-Scoped Skill Install (issue #29, slice 2)

change: `skill-project-scope`
created: 2026-06-26
delivery_strategy: ask-on-risk
chain_strategy: feature-branch-chain

---

## Dependency Order

```
T-01 → T-02 → T-03 → T-04 → T-05 → T-06
                ↘
                T-07 (parallel with T-03..T-06)
```

T-01 through T-06 are sequential; T-07 is parallel-safe from T-02 onward.
All tasks follow strict TDD: tests ship in the same work unit as the code they cover.

---

## T-01 — Extend `Install` struct: add `AllowedProjects`

**Type**: schema change (data-only)
**Files**: `engine/skills/types.go`
**Spec refs**: R-041
**Dependencies**: none

**What to do**:
- Add `AllowedProjects []string` to the `Install` struct.
- Update the inline comment on `DefaultScope` to reflect that `"project"` is now a valid value (the guard moves to `parse.go`).

**Done when**:
- `go build ./engine/skills/...` passes.
- All pre-existing tests still pass (`go test ./engine/skills/...`).

---

## T-02 — Parser: accept `project` scope, parse `allowedProjects`, enforce cross-field constraint

**Type**: test-first feature (parse logic)
**Files**: `engine/skills/parse.go`, `engine/skills/parse_test.go`
**Spec refs**: R-040, R-041, R-042, R-043, R-044, R-045, SC-10, SC-11, SC-12, SC-13, SC-14
**Dependencies**: T-01

**What to do (test-first)**:

1. **Write tests first** in `parse_test.go`:
   - SC-10: entry with `defaultScope: project` + `allowedProjects: [labdrian-sdd-overlay, other-repo]` → parse succeeds, `AllowedProjects == ["labdrian-sdd-overlay","other-repo"]`.
   - SC-11: `defaultScope: workspace` → error containing the line number and the invalid value.
   - SC-12: `defaultScope: global` + non-empty `allowedProjects` → error referencing entry id and the reason.
   - SC-13: `defaultScope: project` with no `allowedProjects` key → no error, `AllowedProjects == nil`.
   - SC-14: run the full existing golden suite and confirm zero new failures (non-regression).

2. **Edit `parse.go`**:
   - In `parseInstall` switch: add `case "allowedProjects"` that calls `parseScalarSequence` (same pattern as `targets`) and assigns to `inst.AllowedProjects`.
   - The `default` branch in `parseInstall` already returns an error for unknown keys — no change needed there.
   - In `validateEntry` (~L571): replace `!= "global"` guard with a closed enum check:
     - `"global"` → OK.
     - `"project"` → OK.
     - anything else → error with line number and invalid value.
   - Add cross-field rule below the scope guard: `if scope=="global" && len(AllowedProjects)>0` → error with entry id and reason.

**Done when**:
- SC-10..SC-14 all pass.
- Zero pre-existing tests broken.

---

## T-03 — `install.go` (pure planner): `CopyOp`, `PlanInstall`

**Type**: test-first feature (pure logic, no I/O)
**Files**: `engine/skills/install.go` (new), `engine/skills/install_test.go` (new)
**Spec refs**: R-048, ADR-3
**Dependencies**: T-01, T-02

**What to do (test-first)**:

1. **Write `install_test.go` planner cases first**:
   - Project-scoped entry admitted: `defaultScope=project`, `allowedProjects=[target-repo]`, `projectID="target-repo"` → one `CopyOp` with correct `Src` and `Dst`.
   - Project-scoped entry excluded (allowedProjects mismatch): `allowedProjects=[other-repo]` → empty plan.
   - Global-scoped entry always excluded from plan.
   - `project`-scoped entry with nil `AllowedProjects` → excluded.
   - Multiple entries: correct filter, declaration order preserved.
   - Empty registry → empty plan (no error).

2. **Write `install.go`** (pure section only):
   ```go
   // CopyOp is a single file-tree copy directive: copy Src/ tree to Dst/.
   type CopyOp struct {
       SkillID string
       Src     string
       Dst     string
   }

   // PlanInstall filters reg for project-scoped skills allowed for projectID,
   // building one CopyOp per admitted entry. Pure: no filesystem access.
   func PlanInstall(reg Registry, projectID, sourceRoot, targetRoot string) ([]CopyOp, error)
   ```
   - Filter: `DefaultScope == "project"` AND `projectID in AllowedProjects` (case-sensitive).
   - `Src = filepath.Join(sourceRoot, e.ID)`.
   - `Dst = filepath.Join(targetRoot, ".claude", "skills", e.ID)`.
   - Preserve declaration order.
   - Empty result is not an error.

**Done when**:
- All planner table tests pass.
- No FS calls anywhere in `PlanInstall`.

---

## T-04 — `install.go` (executor + CLI entry): `ExecuteInstall`, `RenderInstallCore`

**Type**: test-first feature (I/O layer + CLI wiring)
**Files**: `engine/skills/install.go` (extend), `engine/skills/install_test.go` (extend)
**Spec refs**: R-049, R-050, R-051, R-052, R-053, R-054, R-055, R-046 (via --project-id derivation), SC-15, SC-16, SC-17, SC-18, SC-19, SC-20, SC-21, SC-22
**Dependencies**: T-03

**What to do (test-first)**:

1. **Write `install_test.go` executor/CLI cases** (all use `t.TempDir()`):
   - SC-15: valid install copies `SKILL.md` into `<cwd>/.claude/skills/<id>/`.
   - SC-16: idempotent — second run overwrites cleanly.
   - SC-17: skill not in `allowedProjects` is not copied; admitted skill is.
   - SC-18: global-scoped skill excluded; project-scoped admitted.
   - SC-19: missing source dir → non-zero exit, stderr contains skill id and expected path; all missing dirs reported before exit.
   - SC-20: no project identity resolvable (no git, no `--project-id`) → non-zero exit, stderr contains reason, no files written.
   - SC-21: successful run writes nothing outside `<cwd>/.claude/skills/`.
   - SC-22: empty install set → exit 0, stdout contains project id and "no project-scoped skills" notice.

2. **Write executor and CLI entry in `install.go`**:

   ```go
   // ExecuteInstall executes a copy plan produced by PlanInstall.
   func ExecuteInstall(plan []CopyOp, stdout, stderr io.Writer) error
   ```
   - For each `CopyOp`: verify `Src` is a directory (fail-loud if not; collect all failures before returning).
   - Copy strategy: `os.RemoveAll(Dst)` then recursive `copyTree` (WalkDir + MkdirAll + io.Copy, preserve mode bits).
   - Print `"installed: <id>\n"` to `stdout` per copied skill.
   - Any write failure → return error with destination path and OS error (partial write left in place).

   ```go
   // RenderInstallCore is the testable CLI entry for `engine skills install`.
   func RenderInstallCore(args []string, readFile readFileFn, stdout, stderr io.Writer, exit func(int))
   ```
   - Parses: `--registry`, `--source-root`, `--project-id` flags.
   - Derives `projectID`: use `--project-id` if supplied; otherwise `filepath.Base(os.Getwd())`.
   - `targetRoot = os.Getwd()`.
   - Reads + parses registry, calls `PlanInstall`, then `ExecuteInstall`.
   - Empty plan: print notice to stdout and exit 0 (R-052).
   - Project identity unresolvable: if `os.Getwd()` fails, exit non-zero with error to stderr.

**Done when**:
- SC-15..SC-22 all pass.
- `go vet ./engine/skills/...` clean.

---

## T-05 — `skills.go`: dispatch `install` verb; update unknown-verb error

**Type**: test-first wiring (1 switch case, 1 error string)
**Files**: `engine/skills/skills.go`, `engine/skills/skills_test.go`
**Spec refs**: R-056, R-057, SC-23, SC-24
**Dependencies**: T-04

**What to do (test-first)**:

1. **Add test cases to `skills_test.go`**:
   - SC-23: `SkillsCore("install", ...)` routes to `RenderInstallCore`, not the default-error branch.
   - SC-24: `SkillsCore("frobnicate", ...)` → exit 1, stderr contains `"install"` in the supported-verb list.

2. **Edit `skills.go`**:
   - Add `case "install": RenderInstallCore(args, readFile, stdout, stderr, exit)` before the default.
   - Update the empty-verb and unknown-verb error strings to include `"install"` (e.g., `"list, status, validate, install"`).

**Done when**:
- SC-23 and SC-24 pass.
- Zero pre-existing tests broken.

---

## T-06 — `bin/labdrian-overlay`: inject `--source-root` default in `cmd_skills`

**Type**: bash wiring (additive, ~6 lines)
**Files**: `bin/labdrian-overlay`
**Spec refs**: R-058, ADR-1
**Dependencies**: T-05

**What to do**:
- In `cmd_skills`, alongside the existing `has_registry`/`has_manifest` detection block, add:
  ```bash
  local has_source_root=0
  for arg in "$@"; do
    [[ "$arg" == "--source-root" ]] && has_source_root=1
  done
  [[ "$has_source_root" -eq 0 ]] && defaults+=("--source-root" "$OVERLAY_DIR/skills")
  ```
- The loop can be merged into the existing `for arg in "$@"` loop to avoid a second pass.
- The `exec` line is unchanged; defaults are appended after `"$@"` as before.

**Done when**:
- `cmd_skills` unchanged in structure; only the new flag-detect and default-inject lines added.
- `cmd_apply`, `cmd_capture`, and `bin/overlay` byte-identical to pre-change (R-059, R-060).
- Running `labdrian skills install` from a target repo no longer requires the caller to pass `--source-root`.

---

## T-07 — `skills.registry.yaml`: add at least one project-scoped entry

**Type**: data (registry fixture)
**Files**: `skills.registry.yaml`
**Spec refs**: R-061
**Dependencies**: T-02 (parser must accept `project` scope before the file is valid)
**Parallel with**: T-03 through T-06 (no Go code dependency; only blocked on T-02)

**What to do**:
- Add one entry with:
  - `install.defaultScope: project`
  - `install.allowedProjects:` containing at least `labdrian-sdd-overlay`
  - `install.targets:` non-empty (R-045, R-007 still required)
  - A real `skills/<id>/` directory must exist in the overlay.
- Choose a skill that makes sense as project-local (e.g., a skill whose scope is repo-specific).

**Done when**:
- `labdrian skills validate` passes (registry + manifest aligned).
- `go test ./engine/skills/...` passes (existing integration smoke test parses the updated registry).

---

## Review Workload Forecast

| Item | Estimated lines |
|---|---|
| T-01 types.go | ~5 |
| T-02 parse.go + parse_test.go | ~120 |
| T-03 install.go (pure) + install_test.go (planner) | ~120 |
| T-04 install.go (executor+CLI) + install_test.go (executor) | ~200 |
| T-05 skills.go + skills_test.go | ~25 |
| T-06 bin/labdrian-overlay | ~8 |
| T-07 skills.registry.yaml | ~15 |
| **Total** | **~493** |

Estimated changed lines: ~493
400-line budget risk: High
Chained PRs recommended: Yes
Decision needed before apply: Yes
