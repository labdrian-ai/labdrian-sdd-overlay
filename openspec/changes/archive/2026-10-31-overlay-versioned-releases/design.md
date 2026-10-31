# Design: Overlay Versioned Releases

## Technical Approach

Extend `bin/labdrian-overlay` in place: one shared tag-resolution helper and one shared per-target digest function feed `self-update`, `sync-check`, `status`, `apply`, `doctor`, and new `update`/`restore`/`version`. State and backups live under `~/.labdrian-overlay/`. CI cuts semver tags on merge to `main`. The TUI consumes new `KEY=value` VERDICT fields through the existing `ParseSyncCheck` protocol. Slice map: D3/D5/D8 + state/digest helpers → slice 1 (identity/state); D1/D2/D6 + self-update/sync-check/update → slice 2 (fixes+check); D4/D7 + backup/restore/doctor/version/TUI → slice 3 (safety+surfacing).

## Architecture Decisions

| # | Decision | Choice | Alternatives rejected | Rationale |
|---|---|---|---|---|
| D1 | Pre-first-tag bootstrap (OQ1) | CI's first qualifying run on `main` — the merge landing `release.yml` — cuts `v1.0.0` automatically. Until a tag exists: `resolve_latest_release_tag` returns `none`; `self-update` falls back verbatim to today's origin/main convergence, printing a "no release tags yet — legacy convergence" notice; `update`/`version` print "(no releases published yet)"; `apply` records `version=untagged` (digest still real) | Manual first tag; refuse until tagged | Zero regression in the bootstrap window; a scripted first tag cannot be forgotten; daily-production tool warrants 1.0.0 |
| D2 | Standing `tui-self-update` R-007 conflict (OQ2) | Redefine post-update "up to date" as release-based. New repo-level VERDICT field `REPO_BEHIND_RELEASE=<n\|NA>` (commits `main` is behind the newest locally-known `v*` tag; cached-only, like `REPO_BEHIND_ORIGIN`). Tag-mode self-update success guarantees `REPO_BEHIND_RELEASE=0`; `REPO_BEHIND_ORIGIN` semantics stay untouched and may be >0 (untagged commits). TUI amber banner + `u` shortcut re-key on `REPO_BEHIND_RELEASE>0`; raw origin drift demotes to a dim informational line. The delta spec now carries a MODIFIED R-007 stating this (updated this phase — see File Changes) | Keep `REPO_BEHIND_ORIGIN=0` claim (impossible under R-005); silently leave the contradiction | Sync-check-verdicts ownership preserved; legacy zero-tag fallback still satisfies the old claim |
| D3 | State location (OQ3) | Home-scoped `~/.labdrian-overlay/state.json`; backups `~/.labdrian-overlay/backups/<target>/<utc-ts>/` | Repo-scoped `$OVERLAY_DIR/.overlay-state.json` | State describes deployments under `$HOME` — `TARGET_PATHS`, `AGENT_TARGET_PATHS`, `ENGINE_BINARY` are all `$HOME`-anchored; a repo-scoped file lets two clones (OVERLAY_DIR override is documented) hold divergent records of the same live targets. Mirrors `~/.gentle-ai/state.json`; tests already override `HOME`; no `.gitignore` change |
| D4 | Restore UX (OQ4) | Default = most recent backup; `--list` shows retained (≤3) backups (timestamp, version); `--backup <ts>` picks one. TUI action uses most-recent only, ConfirmMessage names timestamp+version. Refuses `--target all`; no backups → exit 1, nothing touched | Most-recent-only | Not gold-plating: apply→bad→apply again makes "most recent" the bad snapshot; with retention 3 a picker flag is the minimal escape hatch |
| D5 | Digest determinism | `compute_target_digest <target> <live\|ref>`: per manifest row via `route_resolve` intersection emit `<repo_rel>:<sha256\|MISSING>`, `LC_ALL=C sort`, `sha256sum` the sorted lines | Manifest-order concatenation; git mktree | Manifest row order is unsorted → order-dependence bug; locale pinned for stable sort |
| D6 | `update` read-only meaning | `update` runs the explicit tag fetch (identity R-003), refreshing only `refs/tags/*`/`refs/remotes/*`; never branch heads, worktree, target files, or state. Offline degrades to cached tags + warning (NA idiom) | Cached-only default | Cached-only fails the "sees origin's new tag" scenario; fetch is byte-idempotent absent remote change, so the zero-mutation scenario holds |
| D7 | Conventional-commit bump | `!`-type or `BREAKING CHANGE:` → major; `feat:` → minor; else patch. Logic inline in `release.yml` only | Bump helper in the CLI | Misbump only mis-sizes an increment, never breaks ordering; consumer-side tag resolution is the tested surface |
| D8 | State I/O | Canonical JSON, atomic tmp+mv write; tolerant awk reader; unparseable → "never deployed" + WARN | jq/python3 hard dependency | Doctor's portability stance: go is the only hard dep |

## Data Flow

    release.yml ──annotated v-tag──▶ origin
    self-update: fetch main+tags(v*) → refusals D1–D3 unchanged → ff main → latest tag (or D1 fallback)
    apply: merge upstream → per-target [ backup(only if ≥1 file changes) → cp → digest → state.json ]
    sync-check/status/doctor/version/update: state.json + compute_target_digest + resolve_latest_release_tag
        → VERDICT/ACTION lines → ParseSyncCheck → TUI dashboard/banner
    restore: backups/<t>/<ts>/ ──cp──▶ target; .meta → state.json

## File Changes

| File | Action | Description |
|---|---|---|
| `.github/workflows/release.yml` | Create | Push-to-main job: skip if HEAD already `v*`-tagged; `git describe` last tag (none → `v1.0.0`); bump per D7; annotated tag + push. `permissions: contents: write`, `fetch-depth: 0`, concurrency group `release-tagging` |
| `bin/labdrian-overlay` | Modify | New helpers `resolve_latest_release_tag` (explicit `refs/tags/v*` fetch first, `--sort=-v:refname`, annotated check), `compute_target_digest`, `state_read_target`/`state_write_target`, `backup_target`/`prune_backups`; `cmd_self_update` tag convergence + already-past-tag no-op (`merge-base --is-ancestor`); `cmd_apply` backup + state write (incl. no-op heal); `cmd_sync_check` VERDICT += `REPO_BEHIND_RELEASE= RECORDED_VERSION= DIGEST_MATCH=`, ACTION names release; `cmd_status` in-sync-at-version; `cmd_doctor` per-target WARN rows (exit unchanged); new `cmd_update`, `cmd_restore`, `cmd_version`; usage + dispatch (`--version` alias) |
| `tui/run.go` | Modify | `TargetVerdict` += RecordedVersion, DigestMatch, RepoBehindRelease (NA sentinel reuse); parse new keys; `SyncBehindRelease` status; classify precedence: capture > apply/digest-mismatch > behind-release > behind-origin; `Actions()` adds "Restaurar respaldo" (Mutating, per-target, confirm names overwrite) and folds `version` into Estado's `Also` |
| `tui/model.go` | Modify | `behindRelease` field (NA-init); probe wiring; `bannerVisible` keys on release-behind |
| `tui/view.go` | Modify | `repoLine` per D2; `viewDashboard` renders per-target version + digest status |
| `tui/release_backend_test.go`, `restore_backend_test.go` | Create | Scratch-repo tests + `pushUpstreamTag` helper (existing harness shape) |
| `openspec/changes/overlay-versioned-releases/specs/tui-self-update/spec.md` | Modify | MODIFIED R-007 added per D2 (done in this phase; Engram spec artifact re-synced) |

## Interfaces / Contracts

VERDICT superset (backward compatible; existing fields byte-identical):

    VERDICT:<t>:UPSTREAM_CHANGED=N OVERLAY_NOT_DEPLOYED=M REPO_BEHIND_ORIGIN=<n|NA> REPO_BEHIND_RELEASE=<n|NA> RECORDED_VERSION=<vX.Y.Z|untagged|NA> DIGEST_MATCH=<yes|no|NA>

    state.json: {"schema":1,"targets":{"claude":{"version":"v1.4.0","digest":"<sha256>","applied_at":"<utc>"}}}

Backup dir holds the full managed live-file set for the target plus `.meta` (prior state entry); restore reinstates files, then recomputes the live digest and persists.

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit | Digest determinism (manifest reorder → identical), single-file mutation changes digest; state schema write/read; corrupt state → never-deployed | Scratch harness, Go-driven bash |
| Integration | Incident regression (push tag, self-update, skip apply → `DIGEST_MATCH=no`, ACTION names version); D1–D3 refusals retained under tag mode; converge-to-tag-not-HEAD; zero-tag fallback; `update` before/after hash zero-mutation; backup-before-overwrite; no-op no-backup; prune-to-3; restore-matches-backup; no-backup error; doctor WARN row, exit 0 | `tui/*_backend_test.go` pattern + `pushUpstreamTag` |
| TUI | Banner on `REPO_BEHIND_RELEASE`, informational origin-drift line, dashboard version strings, restore confirm copy | teatest-style view tests |

## Threat Matrix

| Boundary | Applicability | Design response | Planned RED tests |
|---|---|---|---|
| Documentation-like paths | N/A — files copied byte-wise, never executed; manifest classification unchanged | — | — |
| Git repository selection | Applicable | All git ops stay `cd "$OVERLAY_DIR"`; state/backups deliberately outside any repo; tag fetch uses explicit `refs/tags/v*` refspec | Scratch repo with overridden `OVERLAY_DIR`+`HOME`; stale-tag fetch test |
| Commit state | Applicable | Self-update/apply dirty-tree refusals unchanged; `restore` performs zero git operations | Dirty-tree refusal under tag mode; restore leaves repo bytes untouched |
| Push state | Applicable | CI pushes only `refs/tags/vX.Y.Z`; skip-if-tagged guard prevents double push; `apply --push` unchanged | Consumer-side resolution tests; skip guard is inline in workflow (accepted: not runnable in repo suite) |
| PR commands | N/A — no PR automation | — | — |

## Migration / Rollout

No migration. Old installs gain state lazily: the first `apply` (even no-op) writes it; until then targets report "never deployed". Rollback per proposal: revert chained PRs in reverse; existing tags stay, inert to old code.

## Open Questions

None — OQ1–OQ4 resolved above (D1–D4).
