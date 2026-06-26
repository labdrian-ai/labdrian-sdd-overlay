# Apply Progress: skill-project-scope

branch: skill-project-scope/pr-2-executor
last_updated: 2026-06-26
pr_slice: PR-2 (T-04..T-07) — COMPLETE (post-verify remediation applied)

---

## PR-1 Tasks (pure logic; no I/O, no CLI)

- [x] T-01 — Extend `Install` struct: add `AllowedProjects []string`; update `DefaultScope` comment.
  - File: `engine/skills/types.go`
  - Commit: d07317d

- [x] T-02 — Parser: accept `project` scope, parse `allowedProjects`, enforce cross-field constraint.
  - Files: `engine/skills/parse.go`, `engine/skills/parse_test.go`, `engine/skills/testdata/invalid_scope.yaml`
  - Commit: dfdee46
  - Note: updated `invalid_scope.yaml` fixture from `project` (now valid) to `workspace` (still invalid).
  - Fix (CRITICAL-1/R-040): scope-enum guard moved from `validateEntry` into `parseInstall` where `t.lineNum` is in scope; error now reads `line N: install.defaultScope "X" is not valid`. SC-11 tightened to assert `"line 8"` in error. Commit: 2337734
  - Fix (SUGGESTION-1): corrected ADR-4 in design.md — `project` with absent/empty `allowedProjects` is valid per R-041/SC-13 (not an error). Commit: 7664403

- [x] T-03 — Pure planner: `CopyOp`, `PlanInstall` (no FS access).
  - Files: `engine/skills/install.go` (new), `engine/skills/install_test.go` (new)
  - Commit: f06def0

---

## PR-2 Tasks

- [x] T-04 — `install.go` executor + CLI entry (`ExecuteInstall`, `RenderInstallCore`)
  - Files: `engine/skills/install.go` (extended), `engine/skills/install_test.go` (extended)
  - Commit: 42eaecb
  - Notes:
    - `ExecuteInstall(plan []CopyOp, stdout, stderr io.Writer) error` — full-scan fail-loud on missing sources, then clean-overwrite (RemoveAll + copyTree) per CopyOp.
    - `RenderInstallCore` signature includes injected `cwdFn func() (string, error)` for testability of SC-20.
    - `copyTree` uses `filepath.WalkDir + os.MkdirAll + io.Copy`, preserves mode bits, skips symlinks.
    - SC-15, SC-16, SC-17, SC-18, SC-19, SC-20, SC-21, SC-22 all pass.

- [x] T-05 — `skills.go`: dispatch `install` verb; update unknown-verb error
  - Files: `engine/skills/skills.go`, `engine/skills/skills_test.go`
  - Commit: 6b5a5f5
  - Notes: `case "install": RenderInstallCore(args, readFile, os.Getwd, stdout, stderr, exit)`. Verb-list updated to `list, status, validate, install`. SC-23, SC-24 pass.

- [x] T-06 — `bin/labdrian-overlay`: inject `--source-root` default in `cmd_skills`
  - File: `bin/labdrian-overlay`
  - Commit: b210332
  - Notes: `has_source_root` detection merged into existing for-arg loop. Default `--source-root "$OVERLAY_DIR/skills"` appended after `"$@"` (verb-first preserved). `bash -n` clean. `cmd_apply`/`cmd_capture` byte-identical.

- [x] T-07 — testdata fixture: project-scoped registry entry
  - Files: `engine/skills/testdata/valid_project_scoped.yaml` (new), `engine/skills/parse_test.go` (T07 test added)
  - Commit: c75fe98
  - Notes: `prespec-malandra` with `defaultScope: project`, `allowedProjects: [labdrian-sdd-overlay]`. Real `skills.registry.yaml` stays all-global this slice.

---

## Post-verify Remediation (sdd-verify NO-GO)

- [x] CRITICAL-1 (R-055 path traversal — security): `PlanInstall` now rejects any entry whose
  `filepath.Clean(Dst)` does not start with the clean skills root, or whose `filepath.Clean(Src)`
  does not start with the clean source root. Returns a non-nil error naming the offending id; no
  CopyOp is produced. Guard lives in the pure planner (table-testable without filesystem access).
  R-054 write-failure test also added. Commit: 924425d

- [x] WARNING-3: Added `install` verb description to `usage()` skills block in `bin/labdrian-overlay`.
  Commit: 04b9b99

- WARNING-1 (doc-only, no code change): R-046/R-047 (git-remote identity derivation) are DEFERRED
  per ADR-2. For this slice, project identity = `filepath.Base(os.Getwd())` with `--project-id`
  override. The git-remote upgrade path is isolated to the `cwdFn`/`projectID` boundary in
  `RenderInstallCore` and requires no planner signature changes.

---

## Verification

```
cd engine && go test ./... -count=1 && go vet ./...  → all green (post-remediation)
bash -n bin/labdrian-overlay                          → clean
go.mod                                                → unchanged (zero new deps)
```

## Deviations from spec/design

- `RenderInstallCore` signature adds `cwdFn func() (string, error)` (not in design doc) for SC-20
  testability. `SkillsCore` passes `os.Getwd` transparently. Consistent with "all I/O injected"
  invariant (ADR-2/ADR-3).
- T-07: testdata fixture instead of modifying real `skills.registry.yaml` (orchestrator override).
- R-046/R-047 (git-remote identity) DEFERRED per ADR-2; basename+override is the slice approach.
