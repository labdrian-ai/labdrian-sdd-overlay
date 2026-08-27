# Tasks: Overlay Versioned Releases

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Lines(risk) | S1:180–340(Med) · S2:140–280(Low) · S3a:220–380(Med) · S3b:150–290(Low) |
| Delivery | ask-on-risk |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: Medium

### Work Units

| Unit | Goal | PR | Test | Harness | Rollback |
|---|---|---|---|---|---|
| 1 | Identity+state/digest | PR1 (tracker) | `go test ./... -run ReleaseBackend` | scratch: `overlay update` | revert |
| 2 | Self-update+VERDICT+`update` | PR2 (PR1) | same | scratch+tag: `self-update`+`sync-check` | revert |
| 3a | Backup/restore+doctor/version | PR3a (PR2) | `go test ./...` | `overlay restore --list` | revert |
| 3b | TUI surfacing | PR3b (PR3a) | same | `overlay tui` | revert |

## Anchors (bin/labdrian-overlay)
cmd_apply:728 · cmd_self_update:862 · compute_repo_behind_origin:1020 · cmd_sync_check:1074(:1187) · cmd_status:928 · cmd_doctor:1502(:1588) · usage/dispatch:92/1920 · run.go:72,169,186,211 · model.go:154 · view.go:134,433 · selfupdate_backend_test.go:99

## Slice 1 — overlay-release-identity (R-001..003) — PR1

- [x] 1.1 RED→GREEN `tui/release_backend_test.go` (new; `pushUpstreamTag` helper) + `resolve_latest_release_tag()` (:403, R-003) — zero-tag→`none`, semver order, explicit fetch first.
- [x] 1.2 RED→GREEN digest tests + `compute_target_digest()` via `route_resolve` (D5) — reorder→identical, mutation→differs, sorted `sha256` lines.
- [x] 1.3 RED→GREEN state tests + awk `state_read_target`/`state_write_target()` (D8, tmp+mv) — first apply creates `state.json`; corrupt→"never deployed"+WARN.
- [x] 1.4 Wire `cmd_apply` (:823) to `state_write_target` per target.
- [x] 1.5 Create `.github/workflows/release.yml`: skip-if-tagged, D7 bump, annotated tag+push, `contents:write`, `fetch-depth:0`.
- [x] 1.6 `bash -n bin/labdrian-overlay`; `cd tui && go test ./... -run ReleaseBackend`.

## Slice 2 — overlay-release-fixes-and-check (R-004..006) — PR2 (base PR1)

- [ ] 2.1 RED→GREEN append release_backend_test.go + rewrite `cmd_self_update` (:862, R-004) onto `resolve_latest_release_tag` — tag not raw HEAD, at-tag no-op, D1 fallback, dirty-tree refusal holds.
- [ ] 2.2 RED→GREEN `compute_repo_behind_release()` (D2, beside :1020) — 0 post-tag-update w/ untagged origin ahead, NA pre-tag; VERDICT (:1187) += `REPO_BEHIND_RELEASE=`/`RECORDED_VERSION=`/`DIGEST_MATCH=`; ACTION names version.
- [ ] 2.3 RED→GREEN extend `cmd_status` (in-sync-at-version) + `cmd_update()` (D6) + usage/dispatch — zero-mutation twice, honest never-deployed, no fabricated version.
- [ ] 2.4 `bash -n bin/labdrian-overlay`; `cd tui && go test ./... -run ReleaseBackend` (TUI untouched — additive fields).

## Slice 3a — overlay-release-backup-restore-doctor (R-007..010) — PR3a←PR2

- [ ] 3a.1 RED→GREEN `tui/restore_backend_test.go` (new) + `backup_target()`/`prune_backups()` in `cmd_apply` — backup before overwrite, no-op→none, 4th prunes oldest (retain 3, D4).
- [ ] 3a.2 RED→GREEN `cmd_restore()` (D4: most-recent, `--list`, `--backup <ts>`, refuses `all`) + usage/dispatch — matches latest, no-backup→exit≠0, zero git ops.
- [ ] 3a.3 RED→GREEN per-target digest row in `cmd_doctor` (before :1588) — drifted→WARN exit 0, in-sync passes, checks unaffected.
- [ ] 3a.4 Add `cmd_version()` + `--version` alias in dispatch; `bash -n bin/labdrian-overlay`; `cd tui && go test ./...`.

## Slice 3b — overlay-release-tui-surfacing (R-011) — PR3b←PR3a

- [ ] 3b.1 RED→GREEN extend `tui/view_test.go` + `run.go` (:72,169,186,211) — TargetVerdict += RecordedVersion/DigestMatch/RepoBehindRelease, parse new keys, classify precedence, Actions()+="Restaurar respaldo".
- [ ] 3b.2 GREEN `model.go` (:154 behindRelease+probe) + `view.go` (:134 repoLine/D2, :433 viewDashboard) — version+digest shown, restore needs backup, confirm names overwrite, banner keys on release-behind.
- [ ] 3b.3 `cd tui && go test ./...`; `bash -n bin/labdrian-overlay && bash -n bin/overlay`.
