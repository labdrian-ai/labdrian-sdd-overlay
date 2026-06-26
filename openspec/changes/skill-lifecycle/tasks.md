# Tasks — skill-lifecycle (issue #29, slice 3)

**Change**: `skill-lifecycle` — mutable registry via `skills add` / `skills remove`
**Spec**: R-050..R-083 (34 requirements), SC-20..SC-38 (19 scenarios)
**Design ADRs**: ADR-5 (manifest sync, SKILL.md-row-only), ADR-6 (full re-emit serializer),
ADR-7 (serializer grammar + quoting), ADR-8 (add/remove logic + slug guard), ADR-9 (atomic + validate pipeline)

---

## Dependency graph

```
T-01 → T-02 ──────────────────────────────────┐
T-03 (independent) ───────────────────────────┼──→ T-06 → T-07 → T-08 → T-09 → T-10
T-04 → T-05 ──────────────────────────────────┘
```

**Parallel group 1** (T-01/T-02, T-03, T-04/T-05 run concurrently)
**Sequential group 2** (T-06 through T-10, after group 1 merges)

---

## Task List

### T-01 — Write failing serializer tests
**Status**: [ ] pending
**Files**: `engine/skills/serialize_test.go` (new), `engine/skills/testdata/golden/` (dir, created by test)
**Requires**: nothing (types.go and parse.go already exist)
**Spec**: SC-20..SC-25; tests for R-050..R-059
**Work unit**: test file only — must fail (no serialize.go yet)

Tests to write (TDD first):
- `TestSerializeRoundTripRealRegistry` (SC-20): read real `skills.registry.yaml`, parse, serialize, re-parse; field-by-field equality for all 18 entries
- `TestSerializeRoundTripAllowedProjects` (SC-21): project-scoped entry with `allowedProjects` round-trips
- `TestSerializeDeterministic` (SC-22): two calls on same Registry → `bytes.Equal`
- `TestSerializeNoUpstreamForCustom` (SC-23): output must not contain `upstream` for custom entry
- `TestSerializeUpstreamForCore` (SC-24): output re-parsed has non-nil `Source.Upstream.Owner`
- `TestSerializeGoldenTwoEntry` (SC-25): golden pattern per go-testing skill; `-update` flag writes `testdata/golden/two_entry.yaml`
- `TestSerializeRejectsUnrepresentable` (ADR-7): values containing `{}[]&*!` or tab must return non-nil error

---

### T-02 — Implement serialize.go
**Status**: [ ] pending
**Files**: `engine/skills/serialize.go` (new), `engine/skills/testdata/golden/two_entry.yaml` (generated on first -update run)
**Requires**: T-01 (tests must exist and fail first)
**Spec**: R-050..R-059
**Work unit**: implementation + golden generation

Implementation details (ADR-7):
- `Serialize(reg Registry) ([]byte, error)` — pure function, no I/O
- `needsQuote(v string) bool`: quote when empty, contains `": "` or `" # "`, or starts with `"` or `'`
- `representable(v string) bool`: fail-loud if value contains `{}[]&*!` or `\t` (tokenizer rejects these even inside quotes)
- Field order: `version`, `skills`; entry: `id`, `path`, `source`, `install`, `lifecycle`
- `source.upstream` block: omit when `entry.Source.Upstream == nil` (R-057)
- `install.allowedProjects` sequence: omit when `len(entry.Install.AllowedProjects) == 0` (R-058)
- `version` value: always double-quoted (R-053 special rule)
- 2-space indentation at every level (R-052)
- Entry order: emit in `Registry.Skills` slice order (R-056)
- After implementing: run `go test -run TestSerializeGoldenTwoEntry -update` to write golden file; commit golden

All SC-20..SC-25 + unrepresentable test must pass. `go test ./engine/skills/... -run TestSerialize` green.

---

### T-03 — Refactor manifest.go: extract loadManifestViewReader
**Status**: [ ] pending
**Files**: `engine/skills/manifest.go` (modified)
**Requires**: nothing (independent refactor)
**Spec**: ADR-9 step 7 precondition (in-memory cross-check)
**Work unit**: pure internal refactor, no new public API

- Extract `loadManifestViewReader(r io.Reader) (ManifestView, error)` containing the scanner loop from `LoadManifestView`
- `LoadManifestView(path string)` becomes a thin wrapper: `os.Open(path)` + `defer f.Close()` + `loadManifestViewReader(f)`
- No behavior change; all existing `manifest_test.go` tests continue to pass
- `loadManifestViewReader` must be unexported (package-internal use only)

`go test ./engine/skills/... -run TestManifest` must stay green with zero diff in test output.

---

### T-04 — Write failing pure-transform tests
**Status**: [ ] pending
**Files**: `engine/skills/lifecycle_test.go` (new, pure section)
**Requires**: nothing (types.go and parse.go already exist)
**Spec**: R-061, R-062, R-069, R-070; ADR-8 (slug guard, nil AllowedProjects)
**Work unit**: test file for AddEntry/RemoveEntry only — must fail (no lifecycle.go yet)

Tests to write:
- `TestAddEntryDefaults`: AddEntry on valid id + empty registry returns entry with all R-062 defaults (`Path=id`, `Type=custom`, `Upstream=nil`, `DefaultScope=global`, `Targets=[claude,opencode,codex]`, `AllowedProjects=nil`, `UpdateStrategy=overlay-only`)
- `TestAddEntryNilAllowedProjects`: explicitly confirm `AllowedProjects` is nil (not `[]string{}`); DeepEqual with ParseRegistry round-trip must hold (ADR-8, ADR-7)
- `TestAddEntrySlugGuard`: ids like `"../evil"`, `"foo bar"`, `"FOO"`, `""` must return non-nil error; `"my-skill"`, `"skill1"`, `"a"` must pass
- `TestAddEntryDuplicate`: AddEntry with id already in `Registry.Skills` returns non-nil error (R-061)
- `TestRemoveEntrySuccess`: removes correct entry, remaining entries preserve relative order (R-070)
- `TestRemoveEntryAbsent`: id not present → non-nil error (R-069)

---

### T-05 — Implement AddEntry / RemoveEntry pure functions
**Status**: [ ] pending
**Files**: `engine/skills/lifecycle.go` (new, pure section only)
**Requires**: T-04 (tests must exist and fail first)
**Spec**: R-061, R-062, R-069, R-070; ADR-8

- `slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)` (package-level, lazy-compiled)
- `AddEntry(reg Registry, id string) (Registry, error)`:
  - slug guard → error `"id %q: invalid slug (must match ^[a-z0-9][a-z0-9-]*$)"` 
  - duplicate scan → error `"id %q: already registered"`
  - append `Entry{ID:id, Path:id, Source:Source{Type:"custom"}, Install:Install{DefaultScope:"global", Targets:[]string{"claude","opencode","codex"}, AllowedProjects:nil}, Lifecycle:Lifecycle{UpdateStrategy:"overlay-only"}}`
  - return new `Registry` (pure — do not mutate input slice)
- `RemoveEntry(reg Registry, id string) (Registry, error)`:
  - scan for presence → error `"id %q: not found in registry"` if absent
  - filter out matching entry, return new `Registry`

All T-04 tests must pass. No I/O. Stdlib only (`regexp`).

---

### T-06 — Write failing I/O core tests
**Status**: [ ] pending
**Files**: `engine/skills/lifecycle_test.go` (extend — I/O section)
**Requires**: T-02 (Serialize), T-03 (loadManifestViewReader), T-05 (AddEntry/RemoveEntry)
**Spec**: R-060..R-075, R-082, R-083; SC-26..SC-35
**Work unit**: I/O test additions — must fail (AddCore/RemoveCore not implemented yet)

Tests to write:
- `TestAddCoreSuccess` (SC-26): t.TempDir, write minimal registry + manifest + `skills/foo/SKILL.md`; call `AddCore`; assert exit 0, registry contains `foo`, manifest has `foo/SKILL.md custom`, `Validate` returns zero divergences
- `TestAddCoreMissingSkillMD` (SC-27): dir `skills/foo/` exists but no `SKILL.md`; assert exit != 0, stderr mentions `"foo"`, both files byte-unchanged
- `TestAddCoreIDAlreadyPresent` (SC-28): registry already has `foo`; assert exit != 0, registry byte-unchanged
- `TestAddCoreRegistryWriteFailureIsAtomic` (SC-29): chmod dir 0555 so temp creation fails; assert exit != 0, both files byte-unchanged
- `TestAddCoreValidateBeforeWrite` (SC-30): inject invalid entry via table or wrapper; assert exit != 0, registry unchanged
- `TestRemoveCoreSuccess` (SC-31): registry with `["existing","foo","other"]`; remove `foo`; assert exit 0, registry has `["existing","other"]`, manifest lacks `foo/SKILL.md`, `Validate` zero divergences
- `TestRemoveCoreIDAbsent` (SC-32): registry has no `foo`; assert exit != 0, stderr non-empty, registry unchanged
- `TestRemoveCoreDoesNotDeleteDir` (SC-33): remove succeeds; `skills/foo/SKILL.md` still accessible via `os.Stat`
- `TestRemoveCoreManifestWriteFailureIsAtomic` (SC-34): make manifest dir read-only; assert exit != 0, registry byte-unchanged
- `TestAddRemoveCycleValidateAligned` (SC-35): add `new-skill`, then remove `new-skill`; after each step `Validate` returns zero divergences; `base` entry unchanged throughout

---

### T-07 — Implement AddCore / RemoveCore + writeFileAtomic
**Status**: [ ] pending
**Files**: `engine/skills/lifecycle.go` (extend — I/O section)
**Requires**: T-06 (tests must exist and fail first)
**Spec**: R-060..R-075, R-077, R-082, R-083; ADR-9

Internal helpers (unexported):
- `writeFileAtomic(path string, data []byte) (string, error)` — `os.CreateTemp` same dir, write, `Sync`, return temp path (caller renames); returns temp name for dual-temp pattern
- `appendManifestLine(src []byte, id string) []byte` — append `<id>/SKILL.md custom\n`; ensure trailing newline before append
- `filterManifestLines(src []byte, id string) []byte` — drop lines whose first field equals `<id>/SKILL.md`; preserve all others verbatim

`AddCore(args []string, readFile readFileFn, statFile func(string)(os.FileInfo,error), stdout, stderr io.Writer, exit func(int))`:
1. Parse flags: `--registry`, `--manifest`, `--source-root`, positional id (first non-flag token; mirrors `RenderInstallCore` loop pattern)
2. `readFile(registryPath)` → `ParseRegistry`
3. `AddEntry(reg, id)` → new registry or error
4. `statFile(sourceRoot+"/"+id+"/SKILL.md")` — must not error (R-060)
5. `Serialize(newReg)` → bytes
6. Validate-before-write: `ParseRegistry(bytes.NewReader(regBytes))` + `reflect.DeepEqual(newReg, reg2)` (R-063)
7. `readFile(manifestPath)` → `appendManifestLine(manSrc, id)` → `loadManifestViewReader` → `Diff(reg2, mv)` must be empty (ADR-9 step 7)
8. Dual-temp: `writeFileAtomic(manifestPath, manBytes)` → `manTemp`; `writeFileAtomic(registryPath, regBytes)` → `regTemp`; on any error: clean up temps, exit 1
9. `os.Rename(manTemp, manifestPath)` then `os.Rename(regTemp, registryPath)` (manifest-first per ADR-9)
10. `fmt.Fprintf(stdout, "added: %s\n", id)`, exit 0

`RemoveCore` mirrors AddCore: uses `RemoveEntry`, `filterManifestLines`, same dual-temp + manifest-first ordering, "removed: <id>".

`statFile` injected (R-077); no `os.Stat` direct call in production path.
Zero third-party imports (R-081).

All T-06 tests must pass. `go test ./engine/skills/...` green.

---

### T-08 — Write failing CLI dispatch tests
**Status**: [ ] pending
**Files**: `engine/skills/skills_test.go` (extend)
**Requires**: T-07 (AddCore/RemoveCore must exist)
**Spec**: R-076, R-080; SC-36, SC-37
**Work unit**: two new test functions

Tests to write:
- `TestSkillsCoreDispatchAddRemove` (SC-36): call `SkillsCore("add", [...], ...)` and `SkillsCore("remove", [...], ...)` with in-memory mocks; assert neither writes `"unknown skills verb"` to stderr
- `TestSkillsCoreUnknownVerbMessage` (SC-37): call `SkillsCore("bogus", nil, ...)` ; assert exit code 1 and stderr contains both `"add"` and `"remove"`

---

### T-09 — Update skills.go: add/remove dispatch + verb error messages
**Status**: [ ] pending
**Files**: `engine/skills/skills.go` (modified)
**Requires**: T-08 (tests must exist and fail first)
**Spec**: R-076, R-080

Changes:
- Add `case "add": AddCore(args, readFile, os.Stat, stdout, stderr, exit)` before default
- Add `case "remove": RemoveCore(args, readFile, stdout, stderr, exit)` before default
- Update `case ""` message: `"error: skills requires a verb: list, status, validate, install, add, remove"`
- Update `default` message: `"error: unknown skills verb %q (supported: list, status, validate, install, add, remove)\n"`

All T-08 tests must pass. Existing `SkillsCore` tests must remain green.

---

### T-10 — Verification: stdlib-only imports + unchanged files confirmation
**Status**: [ ] pending
**Files**: none modified (verification only)
**Requires**: T-09 (all implementation complete)
**Spec**: R-078, R-079, R-081; SC-38

Checks (in order):
1. `go build ./...` — must succeed with zero errors
2. `go test ./engine/skills/... -count=1` — all tests green
3. `go list -f '{{join .Imports "\n"}}' ./engine/skills/...` — confirm no new non-stdlib import paths (R-081, SC-38)
4. Confirm `bin/overlay`, `engine/cmd/main.go`, `cmd_apply`, `cmd_capture` have no uncommitted changes (R-079)
5. Spot-check `bin/labdrian-overlay` cmd_skills function covers `add`/`remove` via existing `--registry/--manifest/--source-root` injection (already confirmed at L1020-1027; R-078)

Pass criterion: all five checks clean.

---

## Parallelism summary

| Group | Tasks | Can run concurrently? |
|-------|-------|----------------------|
| 1A | T-01 → T-02 | Yes (with 1B and 1C) |
| 1B | T-03 | Yes (independent of 1A and 1C) |
| 1C | T-04 → T-05 | Yes (with 1A and 1B) |
| 2 | T-06 → T-07 → T-08 → T-09 → T-10 | Sequential; gate on all of group 1 |

Earliest T-06 start: when T-02, T-03, and T-05 are all done.

---

## Review Workload Forecast

| File | Status | Estimated lines |
|------|--------|----------------|
| `engine/skills/serialize.go` | new | ~115 |
| `engine/skills/serialize_test.go` | new | ~170 |
| `engine/skills/testdata/golden/two_entry.yaml` | new | ~25 |
| `engine/skills/manifest.go` | refactor | ~+10 delta |
| `engine/skills/lifecycle.go` | new | ~285 |
| `engine/skills/lifecycle_test.go` | new | ~265 |
| `engine/skills/skills.go` | modified | ~+15 delta |
| **Total** | | **~885** |

**Estimated changed lines: ~885**
**400-line budget risk: High**
**Chained PRs recommended: Yes**
**Decision needed before apply: Yes**

Proposed slice boundaries (feature-branch-chain strategy, tracker = `feature/skill-lifecycle`):

- **PR #1** (targets tracker): Serializer + manifest refactor
  - `serialize.go` (new), `serialize_test.go` (new), `testdata/golden/two_entry.yaml` (new), `manifest.go` (refactor)
  - Tasks: T-01, T-02, T-03
  - Estimated lines: ~320

- **PR #2** (targets PR #1 branch): Pure transforms
  - `lifecycle.go` (AddEntry/RemoveEntry only), `lifecycle_test.go` (pure tests only)
  - Tasks: T-04, T-05
  - Estimated lines: ~150

- **PR #3** (targets PR #2 branch): I/O cores + CLI dispatch + verification
  - `lifecycle.go` (AddCore/RemoveCore + writeFileAtomic complete), `lifecycle_test.go` (I/O tests complete), `skills.go` (dispatch)
  - Tasks: T-06, T-07, T-08, T-09, T-10
  - Estimated lines: ~415

> Note: PR #3 at ~415 lines is marginally over the 400-line budget. If the team requires strict compliance, T-08/T-09 (skills.go dispatch, ~35 lines) can be split into PR #4, bringing PR #3 to ~380 lines.
