# Apply Progress: skill-project-scope

branch: skill-project-scope/pr-1-planner
last_updated: 2026-06-26
pr_slice: PR-1 (T-01..T-03)

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

## PR-2 Tasks (remaining — NOT started)

- [ ] T-04 — `install.go` executor + CLI entry (`ExecuteInstall`, `RenderInstallCore`)
- [ ] T-05 — `skills.go`: dispatch `install` verb; update unknown-verb error
- [ ] T-06 — `bin/labdrian-overlay`: inject `--source-root` default in `cmd_skills`
- [ ] T-07 — `skills.registry.yaml`: add at least one project-scoped entry

---

## Verification

```
cd engine && go test ./... -count=1 && go vet ./...  → all green (post-CRITICAL-1 fix)
go.mod → unchanged (zero new deps)
```

Rendered error string for SC-11 (exact):
`line 8: install.defaultScope "workspace" is not valid; must be 'global' or 'project'`
