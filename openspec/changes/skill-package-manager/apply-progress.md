# Apply Progress: skill-package-manager

Branch: `skill-package-manager/pr-2-validate` (stacked on pr-1-parser)
Last updated: 2026-06-25

---

## PR-1 — Zero-dep YAML subset parser (COMPLETE)

- [x] T-01: `engine/skills/types.go` — Registry, Entry, Source, Upstream, Install, Lifecycle structs
- [x] T-02: `engine/skills/parse_test.go` + `engine/skills/testdata/*.yaml` — 14 table-driven test cases
- [x] T-03: `engine/skills/parse.go` — hand-rolled strict YAML subset parser with line-numbered errors
- [x] T-04: `engine/skills/load_test.go` — 3 Load adapter tests using t.TempDir()
- [x] T-05: `engine/skills/load.go` — Load(path) file adapter

All T-01..T-05 tests passing + 4 post-verify fixes applied.
`cd engine && go test ./... && go vet ./...` — all 6 packages ok, vet clean.
go.mod unchanged: still `go 1.21`, no `require` block.

### Post-verify fixes (commit 26ece60)
- FIX 1: inline comment rejection — unquoted scalars containing ` # ` error with "inline comments are not supported"; quoted values preserve `#` inside.
- FIX 2: unterminated quote detection — `unquoteScalar` now returns `(string, error)` and errors on `"unterminated` or `'unterminated` patterns.
- FIX 3: R-005 test coverage — added `upstream_empty_owner_core_entry` test locking the already-present validateEntry check.
- SUGGESTION-2: block scalar detection extended to `|-`, `|+`, `>-`, `>+` etc.; standalone `|`/`>` in `parseContent` now emits "block scalars not supported" instead of generic format error. Total: 20 test cases in TestParseRegistry, all passing.

---

## PR-2 — ManifestView + Validate (COMPLETE)

Branch: `skill-package-manager/pr-2-validate`
Commits: 56dccbc (manifest), ca99ebb (validate)

- [x] T-06: `engine/skills/manifest_test.go` — 7 test cases for LoadManifestView (skill_dirs_extracted, infra_prefixes_excluded, non_skill_md_rows_ignored, managed_maps_to_core, custom_maps_to_custom, missing_file, blank_and_comment_lines_skipped)
- [x] T-07: `engine/skills/manifest.go` — ManifestEntry{Dir,Tag}, ManifestView map[string]ManifestEntry, LoadManifestView(path) with INFRA_PREFIXES exclusion (engine/, _shared/)
- [x] T-08: `engine/skills/validate_test.go` — TestDiff (5 cases: aligned, missing_in_manifest, missing_in_registry, non_skill_rows_ignored, tag_mismatch, all_divergences_collected) + TestValidate (3 integration cases)
- [x] T-09: `engine/skills/validate.go` — DivergenceClass enum (MISSING_IN_MANIFEST, MISSING_IN_REGISTRY, TAG_MISMATCH, MIXED_TAG), Divergence{Class,Path,Detail}, Diff(reg,mv)[]Divergence (full scan), Validate(reg,manifestPath)([]Divergence,error)

All T-06..T-09 tests passing.
`cd engine && go test ./... && go vet ./...` — all 7 packages ok, vet clean.
go.mod unchanged: still `go 1.21`, no `require` block.

---

---

## PR-3 — CLI commands + engine wiring + bash passthrough (COMPLETE)

Branch: `skill-package-manager/pr-3-cli` (stacked on pr-2-validate)
Commits: 674eca9 (list), cf164fc (status), ae15376 (skills dispatcher), 42cff56 (main.go wiring), 2478338 (main_test.go), 598e8a3 (labdrian-overlay)

- [x] T-10: `engine/skills/list_test.go` + `testdata/golden/list_deterministic_output.txt` — TestRenderListCore (deterministic_output SC-06, missing_registry_file SC-07, invalid_registry)
- [x] T-11: `engine/skills/list.go` — readFileFn type, parseRegistryFlag, RenderList (sorted), RenderListCore (injectable I/O)
- [x] T-12: `engine/skills/status_test.go` — TestRenderStatusCore (count_report SC-08, empty_registry, missing_file, no_manifest_access R-026 guard)
- [x] T-13: `engine/skills/status.go` — RenderStatus, RenderStatusCore (registry-only, never reads manifest)
- [x] T-14: `engine/skills/skills_test.go` — TestSkillsCore (verb_list, verb_status, verb_validate with temp files, unknown_verb, empty_verb)
- [x] T-15: `engine/skills/skills.go` — SkillsCore verb dispatcher + RenderValidateCore (--registry/--manifest flags, fail-loud on divergence)
- [x] T-16: `engine/cmd/main.go` — case "skills": runSkills, runSkillsCore, skills import, usage() updated
- [x] T-17: `engine/cmd/main_test.go` — TestRunSkillsCore_list, TestRunSkillsCore_unknown_verb, TestApplyIgnoresRegistry (SC-13)
- [x] T-18: `bin/labdrian-overlay` — cmd_skills(), skills) dispatch case, usage lines for list/status/validate

All T-10..T-18 tests passing.
`cd engine && go test ./...` — all 7 packages ok.
`bash -n bin/labdrian-overlay` — clean.
go.mod unchanged: still `go 1.21`, no `require` block.

---

## Remaining (PR-4)

- [ ] T-19..T-20 (PR-4): Initial registry population + manifest tracking rows
