# Apply Progress: skill-external-provenance (PR-2)

**Branch:** `skill-external-provenance/pr-2-wiring`
**Status:** COMPLETE

## Tasks completed (PR-2 slice)

### [x] T-05 — manifestTagFor helper + wire validate.go and sync.go
- Added `manifestTagFor(sourceType string) string` in `engine/skills/sync.go` (ADR-13)
- `registryTag` now delegates to `manifestTagFor`
- `tagMatchesSourceType` in `validate.go` now delegates to `manifestTagFor`
- Tests: SC-62, SC-63 in `validate_test.go`; SC-64 in `sync_test.go` — all GREEN
- Commit: `feat(validate,sync): manifestTagFor helper unifies external→custom mapping (ADR-13, R-122..R-125)`

### [x] WARNING-1 — Reject external+upstream in validateEntry
- Added guard in `validateEntry` (`engine/skills/parse.go`) matching the custom+upstream pattern
- Test in `parse_test.go`: `TestParseRegistry/WARNING1_external_with_upstream_errors` — GREEN
- Commit: `fix(parse): reject external+upstream in validateEntry, add cross-field comment (WARNING-1, SUGGESTION-1)`

### [x] SUGGESTION-1 — Cross-field validation comment in parseSource
- Added one-line comment at the cross-field validation site in `parseSource` (parse.go)
- Explains why it lives in parseSource (line-number access), not validateEntry
- Same commit as WARNING-1

### [x] T-06 — AddEntry signature, AddCore, parseFlags --repo/--ref, call-site updates
- `AddEntry` signature changed to `AddEntry(reg Registry, id, repo, ref string)`
- `parseFlags` extended to return `repo, ref` (6 return values)
- `AddCore` has `--ref without --repo` guard (ADR-14)
- All existing call sites updated to `AddEntry(reg, id, "", "")`
- Tests: SC-65..SC-68 + ADR-14 ref-without-repo test — all GREEN
- Commit: `feat(lifecycle,skills): AddEntry provenance params, --repo/--ref flags, update call sites (R-126..R-130, R-133)`

### [x] T-07 — Documentation
- `bin/labdrian-overlay`: usage block updated with `--repo`/`--ref` and no-fetch note
- `README.md`: added **external** note (≤5 lines) in Tracked files section
- Commit: `docs: note external source = vendored provenance, zero fetch (R-131)`

## Verification

- `cd engine && go test ./...` — ALL PASS
- `go vet ./...` — CLEAN
- `bash -n bin/labdrian-overlay` — OK
- `git diff overlay.manifest skills.registry.yaml` — CLEAN (real files unchanged)
- Zero-fetch guard (T-01, PR-1) still passes with no new imports added
