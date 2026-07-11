# Proposal: Detect Local Repo Behind origin/main in sync-check

## Intent

`sync-check`/TUI never compares local HEAD against this repo's own `origin` remote — it only checks overlay-vs-main and main-vs-upstream drift. A user can merge a fix on GitHub, skip `git pull`, and still see a "healthy" result while running a stale clone (GitHub issue #91). This creates false confidence. This change adds a `REPO_BEHIND_ORIGIN` verdict so "healthy" is never reported while the local clone is behind origin.

## Scope

### In Scope
- Default cached-ref comparison (`git rev-list HEAD..origin/main --count` against cached `refs/remotes/origin/main`, no auto-fetch) (R-001)
- Graceful degrade with no remote/no cached ref — existing offline checks unaffected (R-004)
- New `REPO_BEHIND_ORIGIN=<count>` field in the sync-check VERDICT line (R-002)
- Non-silent-healthy rule: never present fully healthy/IN_SYNC while `REPO_BEHIND_ORIGIN>0` (R-006)
- Opt-in `--check-origin`/`--fetch` flag: live `git fetch origin` before comparing (R-003)
- TUI dashboard renders a distinct indicator for the new verdict (R-005)

### Out of Scope
- Automatic `git pull` (informational only)
- Drift detection vs. branches other than main
- Changes to the vendor `UPSTREAM_CHANGED`/overlay-capture mechanism
- Configurable remote name (fixed to `origin` this slice)

## Capabilities

### New Capabilities
- `sync-check-verdicts`: the git-based verdict pipeline in `bin/labdrian-overlay cmd_sync_check` (VERDICT/ACTION line contract) plus its TUI consumption (`tui/run.go ParseSyncCheck`, `tui/view.go` rendering), extended with `REPO_BEHIND_ORIGIN`.

### Modified Capabilities
- None. `runtime-lifecycle` and `tui-polish` are unaffected — verdict logic is bash-owned, not part of `engine/runtime`'s Adapter.

## Approach
Extend `cmd_sync_check` additively: compute the rev-list count (cached by default, fetched under `--check-origin`/`--fetch`), append `REPO_BEHIND_ORIGIN=<count>` to the VERDICT line without changing existing fields. Extend `tui/run.go`'s `ParseSyncCheck`/`TargetVerdict`/`classify` to parse the field and factor it into health classification; extend `tui/view.go` to render it. No new Go `runtime.Action` — bash + TUI parsing only.

## Affected Areas
| Area | Impact | Description |
|------|--------|-------------|
| `bin/labdrian-overlay cmd_sync_check` | Modified | Flag parsing, cached-ref/fetch comparison, VERDICT field |
| `tui/run.go` | Modified | Parse new field, update classification |
| `tui/view.go` | Modified | Render origin-behind indicator |
| `tui/main_test.go` | Modified | Extend precedence/parser tests |

## Risks
| Risk | Likelihood | Mitigation |
|------|------------|------------|
| VERDICT-line change regresses existing parsing | Medium | Strictly additive field; extend existing tests first |
| `--check-origin` network failure breaks offline guarantee | Low | Failure surfaces as scoped warning; other checks still complete |
| No existing bash test harness for `cmd_sync_check` | Medium | Reuse `route_test.go`'s `runOverlay` git-fixture pattern |

## Rollback Plan
Revert the bash/Go diff; VERDICT line reverts to its 3-field format, TUI reverts to the existing 3-state classification. No persisted state — fully revertible via `git revert`.

## Dependencies
None — orthogonal to the in-flight skill-registry/GADU chain.

## Success Criteria
- [ ] `sync-check` reports `REPO_BEHIND_ORIGIN=<count>` with no network call by default
- [ ] No regression for repos without a configured origin remote or cached ref
- [ ] TUI never shows a target as fully healthy while `REPO_BEHIND_ORIGIN>0`
- [ ] `--check-origin`/`--fetch` triggers live fetch only when explicitly passed

## Proposal question round

Open items carried from the requirements brief that shape scope — flagging rather than deciding silently:

1. **CI exit-code gating**: should `REPO_BEHIND_ORIGIN>0` make sync-check's exit code non-zero, or stay informational only? Assumption if unanswered: informational only (matches issue's framing as user-informational, not automation-gating).
2. **Remote name**: always `origin`, or configurable for forks/mirrors? Assumption if unanswered: fixed to `origin` for v1.
3. **Verdict precedence**: shown additively alongside `UPSTREAM_CHANGED`/`OVERLAY_NOT_DEPLOYED`, or does one take visual precedence? Assumption if unanswered: additive/parallel — never overrides or hides the other two.

Reply with corrections or confirm the assumptions to proceed into sdd-spec/sdd-design.
