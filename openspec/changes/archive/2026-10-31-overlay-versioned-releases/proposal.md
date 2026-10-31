# Proposal: Overlay Versioned Releases

## Intent

`labdrian-overlay` has no release concept: `self-update` tracks raw `origin/main` HEAD; the incident — a deployed skill silently stale, its version unnameable. Adopt gentle-ai's model: semver tags, per-target version+digest state, backup/restore, read-only update check (R-001..R-011).

## Scope

### In Scope

- CI cuts releases automatically (settled): new `.github/workflows/release.yml` (`ci.yml` has no release job) on push to `main`; skips if HEAD already tagged; annotated `vX.Y.Z`; conventional-commit bump, default patch; `contents: write`, `fetch-depth: 0`.
- Per-target state file: deployed version + aggregate digest (sha256 over sorted `path:hash` lines; manifest order is unsorted, sorting mandatory).
- R-004: `sync-check`/`status` name the release `apply` brings. R-005: `self-update` fetches tags explicitly, converges to latest tag, D1–D3 kept.
- Read-only `update`; pre-apply timestamped backup — retain 3 per target, auto-prune (settled); `restore`; `doctor` WARN row; `version`; TUI surfacing.
- No migration (settled): old installs lack a state file; R-002 creates it on first `apply` (`never deployed` until then), regardless of location. Design defines pre-first-tag bootstrap.

### Out of Scope

- Git migration, manifest format changes, release channels, Homebrew.
- Legacy `bin/overlay` CI-target confusion (pre-existing).

## Capabilities

### New Capabilities

- `overlay-release-identity`: semver tags, CI release cutting, tag resolution
- `overlay-release-state`: per-target state file + aggregate digest
- `overlay-update-check`: read-only `update` command
- `overlay-backup-restore`: pre-apply backup (retain 3) + `restore`
- `overlay-release-surfacing`: `version`, `doctor` row, TUI state/actions

### Modified Capabilities

- `sync-check-verdicts`: VERDICT/ACTION lines gain release-version fields
- `tui-self-update`: converges to latest release tag, not `origin/main` HEAD

## Approach

Extract `cmd_sync_check`'s sha256 loop into a shared digest function; tag-read helper after explicit tag fetch; backup producer matching the tarball `cmd_capture --from-backup` consumes; extend `ParseSyncCheck`'s line protocol for TUI. Three chained slices per entry contract.

## Affected Areas

- `.github/workflows/release.yml` (new): auto-tag on merge to `main`
- `bin/labdrian-overlay` (modified): self-update, sync-check, status, apply, doctor; new update/restore/version, state/digest helpers
- `tui/run.go`, `model.go`, `view.go` (modified): parse/render version, digest-match, restore
- `tui/*_backend_test.go` (new): scratch-repo tests, tag-push helper

## Risks

- self-update convergence (Med): D1–D3 kept; fixture regression tests
- Digest nondeterminism (Med): sort before hashing; determinism test
- `restore` overwrites live files (Med): `Mutating`/`ConfirmMessage` confirmation
- No tags on origin yet (High): design defines bootstrap fallback
- CI double-tagging (Low): skip-if-tagged guard

## Rollback Plan

`git revert` the chained PRs' merge commits in reverse order (feature-branch-chain). Reverting removes `release.yml` (no further tags); existing tags remain, harmless to old code; user-machine state/backups inert. Unlike the feature's `restore` (rolls back deployed user assets, not this code).

## Dependencies

- `GITHUB_TOKEN` with `contents: write` (tag push).

## Success Criteria

- [ ] `overlay version`/TUI name per-target deployed version and match status.
- [ ] Regression: self-update without apply flags apply-required, naming the version.
- [ ] One annotated semver tag per merge to `main`.
- [ ] `restore` restores the latest backup; max 3 retained.
