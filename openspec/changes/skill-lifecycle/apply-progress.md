# Apply Progress — skill-lifecycle

**Branch**: skill-lifecycle/pr-3-io-cli (stacked on pr-2-transforms)
**Last updated**: 2026-06-26
**Batch**: 3 of 3 (ALL COMPLETE)

## PR-1 Tasks

- [x] T-01 — Write failing serializer tests
  - `engine/skills/serialize_test.go` created
  - Tests: SC-20..SC-25, ADR-7 unrepresentable guard
  - Confirmed RED before implementation

- [x] T-02 — Implement serialize.go
  - `engine/skills/serialize.go` created
  - `Serialize(Registry)([]byte, error)` pure function
  - `needsQuote`, `representable`, `checkRepresentable` helpers
  - Real 18-entry registry round-trips via DeepEqual
  - `testdata/golden/two_entry.yaml` generated and committed
  - All SC-20..SC-25 pass; `go test ./... && go vet ./...` green

- [x] T-03 — Refactor manifest.go: extract loadManifestViewReader
  - `engine/skills/manifest.go` modified
  - `loadManifestViewReader(r io.Reader)(ManifestView, error)` extracted
  - `LoadManifestView` is now a thin open+delegate wrapper
  - All existing manifest tests pass with zero behavior change

## PR-2 Tasks

- [x] T-04 — Write failing pure-transform tests
  - `engine/skills/lifecycle_test.go` created
  - Tests: TestAddEntryDefaults, TestAddEntryNilAllowedProjects, TestAddEntrySlugGuard,
    TestAddEntryDuplicate, TestRemoveEntrySuccess, TestRemoveEntryAbsent,
    TestAddEntryOrderPreservation, TestAddEntryRoundTrip
  - Confirmed RED before implementation (build failed: AddEntry/RemoveEntry undefined)

- [x] T-05 — Implement AddEntry / RemoveEntry pure functions
  - `engine/skills/lifecycle.go` created
  - `slugRe = regexp.MustCompile(^[a-z0-9][a-z0-9-]*$)` package-level
  - `AddEntry`: slug guard + duplicate guard + defaults (AllowedProjects=nil)
  - `RemoveEntry`: presence guard + order-preserving filter
  - All T-04 tests pass; `go test ./... && go vet ./...` green
  - go.mod unchanged; zero new deps

## PR-3 Tasks

- [x] T-06 — Write failing I/O core tests (lifecycle_test.go I/O section)
  - Extended `engine/skills/lifecycle_test.go` with SC-26..SC-35 tests
  - Helper functions: minimalRegistry, minimalManifest, setupFixture
  - Tests: TestAddCoreSuccess, TestAddCoreMissingSkillMD, TestAddCoreIDAlreadyPresent,
    TestAddCoreRegistryWriteFailureIsAtomic, TestAddCoreValidateBeforeWrite,
    TestRemoveCoreSuccess, TestRemoveCoreIDAbsent, TestRemoveCoreDoesNotDeleteDir,
    TestRemoveCoreManifestWriteFailureIsAtomic, TestAddRemoveCycleValidateAligned
  - Confirmed RED before implementation (AddCore/RemoveCore undefined)

- [x] T-07 — Implement AddCore / RemoveCore + writeFileAtomic
  - Extended `engine/skills/lifecycle.go` with I/O section
  - `writeFileAtomic`: os.CreateTemp + Write + Sync + Close → temp path
  - `appendManifestLine`: appends `<id>/SKILL.md custom` with trailing-newline guard
  - `filterManifestLines`: removes lines whose first field is `<id>/SKILL.md`
  - `parseFlags`: extracts --registry/--manifest/--source-root and first positional id
  - `AddCore`: 8-step pipeline (parse→AddEntry→stat SKILL.md→Serialize→validate→manifest cross-check→dual-temp→rename manifest-first)
  - `RemoveCore`: mirrors AddCore with RemoveEntry + filterManifestLines
  - Bug fixed: RemoveEntry normalizes to nil when result is empty (parse→serialize round-trip invariant)
  - All SC-26..SC-35 tests pass

- [x] T-08 — Write failing CLI dispatch tests (skills_test.go)
  - Added TestSkillsCoreDispatchAddRemove (SC-36) and TestSkillsCoreUnknownVerbMessage (SC-37)
  - Confirmed RED before skills.go update

- [x] T-09 — Update skills.go: add/remove dispatch + verb error messages
  - Added `case "add"` and `case "remove"` to SkillsCore
  - Added `stripVerb` helper to remove verb token before passing args to AddCore/RemoveCore
  - Updated empty-verb and default error messages to include add/remove
  - All T-08 tests pass; all existing SkillsCore tests green

- [x] T-10 — Verification: stdlib-only imports + unchanged files confirmation
  - `go build ./...` + `go vet ./...` clean
  - `go test ./...` all green (7 packages)
  - `go list -f '{{join .Imports "\n"}}' ./skills/...` — all stdlib (bufio, bytes, fmt, io, io/fs, os, path/filepath, reflect, regexp, sort, strings)
  - `bin/overlay`, `engine/cmd/main.go`, `cmd_apply`, `cmd_capture` — no uncommitted changes
  - `bin/labdrian-overlay`: bash syntax clean, usage block updated with add/remove lines
  - Real registry `validate` → `registry and manifest aligned (18 skills)` — non-regression confirmed
  - go.mod unchanged

## Commits

### PR-1 (skill-lifecycle/pr-1-serializer)
- `b31277d` feat(skills): add pure Serialize function with round-trip invariant (T-01/T-02)
- `8b69b9f` refactor(skills): extract loadManifestViewReader for in-memory callers (T-03)

### PR-2 (skill-lifecycle/pr-2-transforms)
- `55da0b0` feat(skills): add pure AddEntry/RemoveEntry transforms with slug guard (T-04/T-05)

### PR-3 (skill-lifecycle/pr-3-io-cli)
- `073158c` feat(skills): add AddCore/RemoveCore with atomic write + validate-before-write (T-06/T-07)
- `3888781` feat(skills): dispatch add/remove verbs and update supported-verb error messages (T-08/T-09)
- `5ab4320` docs(overlay): add add/remove to skills verb usage block (T-10)
