# Verify Report — skill-package-manager (PR-4 + whole-change integration)

**Date**: 2026-06-25
**Branch**: skill-package-manager/pr-4-registry-data
**Verdict**: CRITICAL: 1, WARNING: 0, SUGGESTION: 1 — NO-GO until CRITICAL resolved

---

## 1. Test Suite

```
cd engine && go clean -testcache && go test ./...
?     engine/assets    [no test files]
ok    engine/cmd       0.018s
ok    engine/gate      0.002s
ok    engine/prespec   0.004s
ok    engine/propagator 0.002s
ok    engine/settings  0.005s
ok    engine/skills    0.003s
```

`go vet ./...`: clean (no output).
`go.mod`: unchanged — `module github.com/labdrian-ai/labdrian-sdd-overlay/engine`, `go 1.21`, no require block (ADR-1 preserved).
`bash -n bin/labdrian-overlay`: SYNTAX OK.
`bin/overlay`: zero diff vs main.

---

## 2. The 9 Manifest Rows (`git diff main -- overlay.manifest`)

```diff
+engine/skills/types.go managed
+engine/skills/parse.go managed
+engine/skills/load.go managed
+engine/skills/manifest.go managed
+engine/skills/validate.go managed
+engine/skills/list.go managed
+engine/skills/status.go managed
+engine/skills/skills.go managed
+skills.registry.yaml custom
```

8 `engine/skills/*.go` managed rows + 1 `skills.registry.yaml` custom row = 9 total.

The 8 engine rows are consistent with existing `engine/` rows in overlay.manifest and are **INERT**: apply resolves paths as `skills/${path}`, `engine/` lives at repo root, so apply/capture skip them (design's inert-engine-rows discovery). R-043 satisfied.

`skills.registry.yaml custom`: see CRITICAL-1.

---

## 3. End-to-End Acceptance Gate

```
$ engine skills validate --registry skills.registry.yaml --manifest overlay.manifest
registry and manifest aligned (18 skills)
EXIT: 0
```

```
$ engine skills list --registry skills.registry.yaml
chat-thread-analyzer          custom  overlay-only  claude,opencode,codex
gadu-orchestrate              custom  overlay-only  claude,opencode,codex
genesis-delivery-workflow     custom  overlay-only  claude,opencode,codex
genesis-design-system         custom  overlay-only  claude,opencode,codex
inception-pipeline            custom  overlay-only  claude,opencode,codex
kadia-content-guard           custom  overlay-only  claude,opencode,codex
kadia-ui-fix                  custom  overlay-only  claude,opencode,codex
kadia-visual-qa               custom  overlay-only  claude,opencode,codex
prespec-malandra              custom  overlay-only  claude,opencode,codex
project-architect             custom  overlay-only  claude,opencode,codex
project-inception             custom  overlay-only  claude,opencode,codex
project-manifest              custom  overlay-only  claude,opencode,codex
requirements-from-transcripts custom  overlay-only  claude,opencode,codex
roadmap-maker                 custom  overlay-only  claude,opencode,codex
sdd-spec                      core    vendor-merge  claude,opencode,codex
sdd-tasks                     core    vendor-merge  claude,opencode,codex
sdd-time-estimation           custom  overlay-only  claude,opencode,codex
sdd-verify                    core    vendor-merge  claude,opencode,codex
EXIT: 0
```

```
$ engine skills status
Total:  18
Core:   3
Custom: 15
Status: OK
EXIT: 0
```

18 skills, 3 core/vendor-merge, 15 custom/overlay-only, all targets = `claude,opencode,codex`. Sorted by id. Deterministic.

---

## 4. Registry YAML Correctness

- Parses under strict subset parser: confirmed (validate exit 0)
- Core entries (sdd-spec, sdd-tasks, sdd-verify): `source.type=core`, `upstream.owner=gentleman-programming`, `lifecycle.updateStrategy=vendor-merge`
- Custom entries (15): `source.type=custom`, no upstream block, `lifecycle.updateStrategy=overlay-only`
- Infra dirs (`engine/`, `_shared/`): excluded — no registry entries
- One entry per SKILL.md dir: 18 manifest SKILL.md rows map to 18 registry entries, 1:1
- No inline comments: confirmed
- No duplicate IDs: confirmed (validate would exit 1 otherwise)
- File lives at repo root: confirmed
- ADR-3 invariants: `core<->vendor-merge` and `custom<->overlay-only` enforced by ParseRegistry

---

## 5. R-NNN Compliance Sweep

| Requirement | Status | Evidence |
|---|---|---|
| R-001..R-012 (parse schema/rules) | PASS | parse_test.go 20 table-driven cases |
| R-013..R-018 (population from manifest) | PASS | manifest_test.go 7 cases |
| R-019..R-023 (skills list) | PASS | engine skills list: 18 lines, sorted, exit 0 |
| R-024..R-027 (skills status) | PASS | engine skills status: Total 18, Core 3, Custom 15, exit 0 |
| R-028..R-035 (skills validate) | PASS | engine skills validate: aligned, exit 0; validate_test.go 3 integration cases |
| R-036..R-038 (bash router) | PASS | bin/labdrian-overlay cmd_skills(), bash -n clean |
| R-039 (commands READ-ONLY) | PASS | No write ops in any skills subcommand |
| R-040 (deploy pipeline unchanged) | PASS | bin/overlay: zero diff vs main |
| R-041 (apply ignores registry) | PASS | TestApplyIgnoresRegistry (SC-13) passes |
| R-042 (new code scope) | PASS | All new code in engine/skills/*.go, engine/cmd/main.go, bin/labdrian-overlay |
| R-043 (engine/skills/*.go as managed) | PASS | 8 rows in manifest diff |
| R-044 (skills.registry.yaml as custom) | CONFLICT | Row present (spec satisfied) but ADR-5 forbids it |

No deferred non-goals leaked in. All red lines (R-039..R-042) are clean.

---

## CRITICAL Issues

### CRITICAL-1: spec R-044 directly conflicts with design ADR-5

**Spec R-044** (openspec/changes/skill-package-manager/specs/spec.md):
> "overlay.manifest SHALL contain a row for skills.registry.yaml tagged custom."

**Design ADR-5** (openspec/changes/skill-package-manager/design.md):
> "skills.registry.yaml lives at repo root and is NOT added to overlay.manifest."

The implementation added `skills.registry.yaml custom` to overlay.manifest, following the spec and T-20.

**Functional impact**: zero. The row is inert — apply resolves `skills/skills.registry.yaml` which doesn't exist at repo root, so it's skipped. The deploy pipeline is unchanged. SC-13 (`TestApplyIgnoresRegistry`) passes.

**Resolution options** (choose one before merging):

A. Remove `skills.registry.yaml custom` from overlay.manifest and update spec R-044 to align with ADR-5. Removes an inert-but-conceptually-wrong row.

B. Update ADR-5 to accept the row as a tracking-only entry, consistent with how engine/*.go rows are treated (tracked but inert). Simpler — no code or manifest change needed; only a docs update.

Recommended: **option B**. The row is harmless, documents what the file is, and follows the established pattern of inert engine rows.

---

## SUGGESTION Issues

### SUGGESTION-1: sdd-time-estimation is custom/overlay-only

If sdd-time-estimation is eventually upstreamed to gentleman-programming, its registry entry will need updating to `core/vendor-merge`. No action required now; flag for future registry maintenance.

---

## PR-4: NO-GO
## Whole-change: NO-GO

The single blocker is a spec-design contradiction (CRITICAL-1). All tests pass, the e2e acceptance gate is clean, and the deploy pipeline is untouched. Resolve CRITICAL-1 (option B is a one-line ADR-5 amendment) and the change is ready to archive and merge.
