# Design: Detect Local Repo Behind origin/main in sync-check

## Technical Approach

Extend the existing bash verdict engine (`bin/labdrian-overlay cmd_sync_check`, lines 795-911) additively: compute `REPO_BEHIND_ORIGIN` once per process (repo-level, not per-target) before the `for t in resolve_targets` loop, then append the same value to every target's `VERDICT:` line. No new Go `runtime.Action` — `engine/runtime` stays a thin lifecycle wrapper (confirmed architecture invariant). On the Go side, extend `tui/run.go`'s `TargetVerdict`, `ParseSyncCheck`, and `classify()` to parse and factor in the new field; extend `tui/view.go` to render a distinct indicator. Strict TDD applies to every phase below: extend/add failing tests first (RED), then implement (GREEN) — this is non-negotiable per project config (`strict_tdd: true`).

## Architecture Decisions

| Decision | Choice | Alternative considered | Rationale |
|---|---|---|---|
| Compute scope | Once per `cmd_sync_check` invocation, before the target loop | Per-target computation | `origin/main` is repo-level, not target-scoped; recomputing per target wastes `git` calls with no behavior change |
| VERDICT field encoding | `REPO_BEHIND_ORIGIN=<n>` or `REPO_BEHIND_ORIGIN=NA` sentinel | Omit the field entirely when unavailable | Explicit `NA` self-documents "checked but unavailable" vs. silent omission, which existing/older parsers could misread as `0` |
| Flag aliasing | `--check-origin` and `--fetch` are synonyms for one boolean | Two distinct behaviors | R-003 treats them as the same opt-in; single boolean keeps bash parsing simple |
| Fetch-failure handling | On `git fetch origin` failure: emit `REPO_BEHIND_ORIGIN=NA` + a `SYNC_CHECK:` warning line (reusing the existing scoped-warning prefix at line 818); do NOT silently fall back to a stale cached count | Silently fall back to cached-ref count | User explicitly asked for live accuracy; presenting a stale count as if fresh would be misleading. Offline checks (UPSTREAM_CHANGED/OVERLAY_NOT_DEPLOYED) still complete per R-004 |
| Guard idiom for new git calls under `set -euo pipefail` | Every new git invocation (`git fetch origin`, `git rev-list HEAD..origin/main --count`) MUST be wrapped in the same `if <git-cmd> 2>/dev/null; then ... else ...NA...; fi` guard idiom already used in this function for exactly this situation (precedent: `bin/labdrian-overlay:854`, `if git cat-file -e "main:${rr_repo_rel}" 2>/dev/null; then ...`) | Let the command's non-zero exit propagate | `bin/labdrian-overlay` runs under `set -euo pipefail` (line 2); an unguarded `git fetch`/`git rev-list` failure (missing `origin/main`, no network) would abort the whole script instead of degrading to `REPO_BEHIND_ORIGIN=NA`, violating R-004's "SHALL still complete the existing OVERLAY_NOT_DEPLOYED/UPSTREAM_CHANGED checks" |
| Go `classify()` signature | Add third param `repoBehindOrigin int`; keep precedence `UPSTREAM_CHANGED > OVERLAY_NOT_DEPLOYED > REPO_BEHIND_ORIGIN > healthy`; new enum value `SyncBehindOrigin` | Leave `classify()` untouched, add a separate bool alongside `Status` | Existing `view.go` rendering is a `switch v.Status` — a new enum value is the smallest diff consistent with that convention. Precedence order keeps R-003 confirmed decision #3 (additive/parallel — never overrides the two existing verdicts) while still satisfying R-006 (never "healthy" while behind origin) |
| `RepoBehindOrigin` unavailable encoding (Go) | `int` field with `-1` sentinel meaning "NA/unavailable" | Separate `RepoBehindOriginKnown bool` field | Keeps `TargetVerdict` primitive-only, consistent with existing fields; documented via a doc comment on the field |
| `ParseSyncCheck` field-parsing for `REPO_BEHIND_ORIGIN` | Add an explicit `case "REPO_BEHIND_ORIGIN":` branch that special-cases the literal string `val == "NA"` FIRST (setting `v.RepoBehindOrigin = -1` directly) BEFORE any `strconv.Atoi` call, only calling `strconv.Atoi(val)` for the non-`"NA"` case | Reuse the existing generic `n, _ := strconv.Atoi(val)` line (`tui/run.go:191`) as-is for this field too | The existing generic line discards the `strconv.Atoi` error (`n, _ := ...`), so both an omitted field and an unguarded literal `"NA"` string silently collapse to Go's zero-value `0` — indistinguishable from "confirmed 0 commits behind / healthy." This reproduces the exact silent-healthy bug class R-006 exists to eliminate; a dedicated pre-check closes that gap |
| Exit code | No change — informational only | Non-zero exit when `REPO_BEHIND_ORIGIN>0` | Matches confirmed product decision #1; CI/automation gating stays unaffected |

## Data Flow

    bash: cmd_sync_check
      --check-origin/--fetch? ──► git fetch origin (warn+NA on failure)
      no origin remote? ────────► REPO_BEHIND_ORIGIN=NA
      no cached origin/main? ───► REPO_BEHIND_ORIGIN=NA
      else ─────────────────────► git rev-list HEAD..origin/main --count
                │
                (both git calls above run under set -euo pipefail and MUST
                 be wrapped in the guard idiom `if <git-cmd> 2>/dev/null;
                 then ... else ...NA...; fi` — precedent: bin/labdrian-overlay:854
                 `if git cat-file -e "main:${rr_repo_rel}" 2>/dev/null; then ...`
                 — so a failed/missing-ref git call degrades to the NA
                 sentinel path instead of aborting the script)
                ▼
      VERDICT:<target>:UPSTREAM_CHANGED=N OVERLAY_NOT_DEPLOYED=M REPO_BEHIND_ORIGIN=<n|NA>
                │  (stdout, one line per target)
                ▼
      tui/run.go ParseSyncCheck() ──► TargetVerdict{..., RepoBehindOrigin: n|-1}
                │
                ▼
      tui/run.go classify(uc, ond, rbo) ──► SyncStatus (incl. new SyncBehindOrigin)
                │
                ▼
      tui/view.go viewDashboard() ──► colored badge + always-visible origin-behind count

## File Changes

| File | Action | Description |
|---|---|---|
| `bin/labdrian-overlay` | Modify | `cmd_sync_check`: parse `--check-origin`/`--fetch`; add `compute_repo_behind_origin()` helper; append `REPO_BEHIND_ORIGIN=<n|NA>` to each target's VERDICT line |
| `tui/run.go` | Modify | `TargetVerdict.RepoBehindOrigin int` (-1 = NA); `ParseSyncCheck` parses new field; `classify()` gains 3rd param + `SyncBehindOrigin` |
| `tui/view.go` | Modify | New case in `viewDashboard()`'s status switch; counts line always shows origin-behind count when known (>=0), independent of `Status` precedence (satisfies R-006's "still visible" requirement) |
| `tui/main_test.go` | Modify | Extend `TestParseSyncCheckDashboard` sample with the new field; update `TestClassifyPrecedence` call sites to 3 args; add a precedence test asserting `REPO_BEHIND_ORIGIN>0` alone does not classify as healthy |
| `engine/installer/sync_check_test.go` | Create | New git-fixture integration tests reusing `route_test.go`'s `overlayScript(t)`/`runOverlay(t, ...)` pattern against real temp git repos |

## Interfaces / Contracts

```go
// tui/run.go
type TargetVerdict struct {
    Target             string
    UpstreamChanged    int
    OverlayNotDeployed int
    RepoBehindOrigin   int // count; -1 = unavailable (no remote/no cached ref/fetch failed)
    Action             string
    Status             SyncStatus
    AgentFiles         []AgentFileEntry
}

const SyncBehindOrigin SyncStatus = iota + 4 // after SyncNeedsCapture

func classify(upstreamChanged, overlayNotDeployed, repoBehindOrigin int) SyncStatus
```

VERDICT line (bash, additive): `VERDICT:$t:UPSTREAM_CHANGED=$u OVERLAY_NOT_DEPLOYED=$o REPO_BEHIND_ORIGIN=$r`

## Testing Strategy

| Layer | What to Test | Approach (RED before GREEN) |
|---|---|---|
| Bash/integration | behind (n>0), even (n=0), no-remote, no-cached-ref, `--check-origin` fetch-failure | New `engine/installer/sync_check_test.go`; build temp git repos with/without `origin`, with/without fetched `origin/main`, using `overlayScript(t)`/`runOverlay(t, ...)`. Write failing assertions first, then implement `compute_repo_behind_origin()` |
| Go unit | `ParseSyncCheck` field extraction (`n` and `NA`→-1); `classify()` 3-arg precedence incl. `SyncBehindOrigin` | Extend `tui/main_test.go`; write failing calls/assertions with new signature before touching `run.go` |
| Rendering | Dashboard shows distinct badge + always-visible count when `RepoBehindOrigin>0`, even under `SyncNeedsApply`/`SyncNeedsCapture` | New assertions in `tui/main_test.go` or a small `tui/view_test.go` (none exists yet) checking `viewDashboard()` output contains the origin-behind marker string |
| Regression | Existing `TestClassifyPrecedence`/`TestParseSyncCheckDashboard` behavior unchanged for the 3 pre-existing states | Update call sites for the new signature; assert original UPSTREAM_CHANGED/OVERLAY_NOT_DEPLOYED precedence untouched |

## Migration / Rollout

No migration required. Purely additive VERDICT field and Go struct/enum extension; `git revert` fully reverses both bash and Go sides per proposal's rollback plan.

## Open Questions

None blocking — all three proposal "Proposal question round" items are resolved by the confirmed product decisions in this session's context (exit code informational-only, `origin` fixed, additive/parallel precedence).
