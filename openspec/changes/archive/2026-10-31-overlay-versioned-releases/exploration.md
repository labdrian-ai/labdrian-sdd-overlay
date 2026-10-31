## Exploration: overlay-versioned-releases

Adopt a gentle-ai-style versioned-release model (semver tags, per-target digest
state, backup/restore, extended doctor, read-only update check, TUI surfacing)
for labdrian-overlay's own update mechanism (`bin/labdrian-overlay` + `tui/`).

### Current State

`bin/labdrian-overlay` (1945 lines) has **zero** version/release concept today
(`git tag -l` on this repo returns nothing; no `VERSION`/`CHANGELOG` file
exists; the only "version" string in the file is `validate-entry-contract`'s
unrelated schema-bundle version). Relevant mechanics, verified by full read:

- **`TARGET_PATHS`** (lines 20-24): `claude → ~/.claude/skills`,
  `opencode → ~/.config/opencode/skills`, `codex → ~/.codex/skills`.
- **`AGENT_TARGET_PATHS`** (lines 27-31): only `claude → ~/.claude/agents` and
  (added later, confirmed via `route_resolve`'s `opencode-agent` branch,
  lines 367-374) `opencode → ~/.config/opencode/agents`. **Codex has no agent
  target at all.** This means the three targets are NOT symmetric — an
  `agent`-route manifest row only ever touches `claude`; an
  `opencode-agent`-route row only ever touches `opencode`; only `skill`-route
  rows (the majority) touch all three. This is direct, strong evidence for
  A-01 (below).
- **`cmd_self_update`** (lines 862-922): flagless, repo-level (not
  target-scoped). Order: (1) `die` if no `origin` remote; (2) `die` if dirty
  tracked tree; (3) bounded `git fetch origin main` (main branch ref only);
  (4) `ahead="$(git rev-list --count origin/main..main)"` → `die` if >0
  ("Local main is N commit(s) ahead of origin/main..."); (5)
  `behind="$(git rev-list --count main..origin/main)"` → if 0, "already up to
  date", return, **no checkout at all**; (6) only now: trap-protected
  `git checkout main && git merge --ff-only origin/main`, then return to the
  original branch. **Critically: the ahead-refusal (step 4) and the
  convergence target (step 6) both compare against `origin/main` HEAD, not
  any tag** — there is no tag-fetch anywhere (`git fetch origin main` does
  NOT explicitly fetch tags; git's default opportunistic tag-following only
  grabs a tag if its commit is already among the fetched objects, which is
  not guaranteed for a tag pointing at an older/different commit).
- **`compute_repo_behind_origin`** (lines 1020-1072) + **`cmd_sync_check`**
  (lines 1074-1199): the direct per-file precedent for R-003. For each
  target, for each manifest row resolved via `route_resolve`/`resolve_targets`
  intersection, it computes `live_hash=sha256sum(live_file)` and
  `main_hash=sha256sum(git show main:$repo_rel)`, classifies each file as
  `IN_SYNC` / `OVERLAY_NOT_DEPLOYED` / `UPSTREAM_CHANGED`, aggregates counts,
  and emits one `VERDICT:<target>:UPSTREAM_CHANGED=N OVERLAY_NOT_DEPLOYED=M
  REPO_BEHIND_ORIGIN=<n|NA>` line per target plus an `ACTION:<target>:` hint
  line. **This already fully catches the reported incident at the file
  level**: if `apply` hasn't run after `self-update`, `live_hash != main_hash`
  for the stale file and it is reported as `OVERLAY_NOT_DEPLOYED`, driving
  `ACTION:$t: run 'overlay apply --target $t'`. What is genuinely missing is
  (a) naming *which release version* is available/expected in that ACTION
  text — today it's a bare per-file mismatch count, never a version string —
  and (b) a queryable persisted fact ("target X was last successfully applied
  at vY with digest Z") independent of re-scanning the whole file tree, which
  `doctor`/`version` (R-009/R-010) can read directly.
- **`cmd_apply`** (lines 728-850): per-target loop (779-824) does a plain
  `cp "$src" "$dest"` for every out-of-sync file (line 814), with no backup of
  the pre-overwrite `$dest` content anywhere — R-007/R-008 are genuinely new.
  Note line 17: `TARBALL="$HOME/.gentle-ai/backups/upgrade-20260615T175529Z/snapshot.tar.gz"`
  and `cmd_capture --from-backup <tarball>` (lines 635-660) already know how
  to **consume** a gentle-ai-shaped timestamped-backup tarball
  (`tar xzf ... snapshot.tar.gz`) — the shape gentle-ai's own backups use is
  already a first-class, exercised concept in this codebase, just never
  produced by `labdrian-overlay` itself for its own deploys.
- **`cmd_doctor`** (lines 1502-1613, confirming A-04's premise): already
  exists, already additive-row-shaped (`PASS`/`WARN`/`FAIL` per check: go,
  gentle-ai, discovery tools, engine binary, non-empty skill-registry),
  already has a `--fix` flag, already "exits non-zero only on a hard FAIL."
  It has zero targets-loop today (unlike `cmd_status`/`cmd_sync_check`) — a
  new digest-consistency row needs its own `resolve_targets`-style iteration.
- **`cmd_status`** (lines 928-985): same per-file diff loop shape as
  sync-check but without the UPSTREAM_CHANGED/OVERLAY_NOT_DEPLOYED
  distinction (just `in sync` / `DIFFERS` / `MISSING`).
- The TUI (`tui/model.go`, `tui/view.go`, `tui/run.go`, both backend tests)
  is fully read. The **most recent shipped change**
  (`openspec/changes/archive/2026-08-24-tui-self-update-offer/`, PRs
  #159/#161/#163) already added: `probeBehindOriginCmd`/`ParseSyncCheck`-based
  launch probe (`model.go` `Init()`), the dismissible amber banner
  (`view.go` `repoLine()`), the `u`-key shortcut
  (`model.go`'s `bannerVisible()`/`selfUpdateAction()`), and — important for
  R-004/R-011 — **the "Actualizar repositorio" TUI action already chains
  `apply` via `Action.Also`** (`run.go` lines 95-105:
  `Also: []Action{{Command: "apply", SupportsAll: true}}`). So **the TUI path
  already closes the exact reported incident's loop today**; only a bare CLI
  `overlay self-update` invocation (no TUI) still leaves a target stale until
  a separate manual `apply`. This narrows R-004's real remaining gap to (1)
  CLI-level `self-update` still not auto-applying (a design-phase decision,
  out of this explore's scope to resolve) and (2) naming the release version
  in `sync-check`/`status`/`doctor` output, which today is bare file counts.
- Test harness precedent (`selfupdate_backend_test.go`,
  `capture_backend_test.go`): both are Go-to-bash integration tests that spawn
  the **real** `bin/labdrian-overlay` against hermetic scratch git repos
  (bare `origin.git` + working `clone`, both under `t.TempDir()`), with
  `GIT_CONFIG_GLOBAL=/dev/null`, fixed author/committer env, `OVERLAY_DIR`
  pointed at the scratch clone, and `HOME` pointed at a fresh empty scratch
  dir (critically: **pre-created** `$HOME/.claude/skills` for self-update
  tests since sync-check's target-dir-missing guard skips the VERDICT line
  otherwise — a real CI-vs-local flake this repo already hit once). New
  tag/state/digest/backup/restore tests should follow this exact harness
  shape (`newScratchRepo`, `pushUpstreamCommit`, `runBackendSubcommand`), plus
  a tag-pushing helper analogous to `pushUpstreamCommit`/
  `pushUpstreamBranchCommit`.
- **Aside (not in scope, flagged for awareness)**: `bin/overlay` (308 lines)
  is a much older, much smaller, structurally different script than
  `bin/labdrian-overlay` (1945 lines) — not a symlink, not a stale copy of
  the same content. `openspec/config.yaml`'s shell test layer still runs
  `bash -n bin/overlay && shellcheck bin/overlay`, i.e. it currently
  shellchecks the *wrong*/legacy file, not `bin/labdrian-overlay`. This is a
  pre-existing repo inconsistency, unrelated to R-001..R-011, not part of
  this change's scope.

### Affected Areas

- `bin/labdrian-overlay` — `cmd_self_update` (tag-aware convergence, R-005),
  `cmd_sync_check`/`compute_repo_behind_origin` (digest + version-named
  ACTION text, R-004), `cmd_apply` (pre-overwrite backup, R-007), `cmd_doctor`
  (new consistency row, R-009), new `cmd_update` (R-006), new `cmd_restore`
  (R-008), new `cmd_version` (R-010), `usage()` + dispatch `case` (all new
  commands), a new state-read/write helper module (R-002), a new
  digest-aggregation helper (R-003, extracted from `cmd_sync_check`'s
  per-file loop so `sync-check`/`doctor`/`apply`/`version` share one
  implementation).
- `tui/model.go` — new fields for recorded version/digest-match/backup
  availability per target (parallel to `behindOrigin`/`bannerDismissed`);
  possibly a new msg type if `version`/backup state needs its own probe.
- `tui/run.go` — `TargetVerdict`/`SyncStatus`/`classify()`/`ParseSyncCheck`
  need new fields (recorded version, digest-match, backup availability) and
  parsing; `Actions()` needs new rows for `restore` (and possibly `version`/
  `update`) following the exact existing `Action`/`Also`/`TargetAgnostic`
  shape.
- `tui/view.go` — `viewDashboard()` needs to render the new per-target
  version/restore-availability fields (R-011).
- `tui/selfupdate_backend_test.go`, `tui/capture_backend_test.go` — pattern
  to mirror for all new backend integration tests; a new
  `tag_backend_test.go`/`restore_backend_test.go`-style file is the natural
  home for the new coverage, following the same scratch-repo harness.
- `overlay.manifest` — read-only consumer for R-003's digest computation;
  format itself stays unchanged (confirmed out-of-scope boundary honored).
- New state/backup storage location (path TBD at design; see A-verdicts
  below) — not yet created anywhere in the repo.

### Assumption Resolution (A-01 through A-04)

| ID | Working assumption | Verdict | Evidence |
|---|---|---|---|
| A-01 | Per-target digest (Option A), not one repo-wide digest | **CONFIRMED — stronger than "working assumption"** | `AGENT_TARGET_PATHS` (lines 27-31) proves the three targets are not even symmetric in which files they receive: `codex` gets zero agent-route files, `claude`/`opencode` each get a distinct agent subset, and `route_resolve` (lines 336-383) already computes a per-target file-set intersection for every existing command (`status`, `sync-check`, `apply`, `capture`). A repo-wide digest would either (a) hash files a given target never even receives, corrupting the comparison, or (b) require re-deriving per-target file-set filtering anyway to build it correctly — at which point it *is* a per-target digest with an extra concatenation step. No design or implementation reason favors Option B. |
| A-02 | Single implicit "stable" line, no channel flag | **CONFIRMED** | No `--channel`/branch-per-channel concept exists anywhere; single `origin` remote, single clone per machine (git-clone model, not gentle-ai's Homebrew-formula-per-channel model); zero evidence of a consumer needing beta/nightly. Building channel infra now is unjustified scope. |
| A-03 | `self-update` converges to latest tag only; existing ahead-of-origin refusal semantics (D1-D3) stay intact | **CONFIRMED, with one concrete gap to flag for design** | The ahead-refusal (`cmd_self_update` step 4, line ~891-894) and the up-to-date/convergence steps (step 5-6, lines ~897-921) **already compare against `origin/main` HEAD, not a tag** — so A-03's premise ("main can be ahead of the last release without tripping the existing refusal, since that refusal compares against origin/main, not the tag") is structurally already true and needs no refusal-logic change. **Gap**: step 3's fetch (`git fetch origin main`, line 883-885) does not explicitly fetch tags, so a design must add explicit tag fetching (e.g. `git fetch --tags origin main` or an explicit `refs/tags/*:refs/tags/*` refspec) before tag resolution, or the latest tag may not be locally visible yet. |
| A-04 | Extend existing `doctor`, do not add a second health command | **CONFIRMED** | `cmd_doctor` (lines 1502-1613) is already exactly the additive-row shape R-009 wants (`PASS`/`WARN`/`FAIL` lines, `--fix` opt-in, "exits non-zero only on hard FAIL"). It currently has no per-target loop (unlike `cmd_status`/`cmd_sync_check`), so the new row needs its own `resolve_targets`-style iteration — a small, additive, non-breaking extension, not a rearchitecture. |

### Concrete Integration Points

1. **Version tag read** (new): no existing code reads tags. New function
   alongside `compute_repo_behind_origin` (e.g. `resolve_latest_tag <ref>`)
   using `git tag --sort=-v:refname --merged <ref> 'v*'` or
   `git describe --tags --abbrev=0 <ref>` — must run only after an explicit
   tag fetch (the A-03 gap above).
2. **Version tag write**: left to design/`sdd-tasks` — R-001's own
   Ambiguities note explicitly defers "manual maintainer step vs. a scripted
   `overlay release` command" to design; no evidence in this explore pins one
   over the other.
3. **Per-target digest compute**: extract `cmd_sync_check`'s existing
   per-file `sha256sum` loop (lines 1130-1141, already iterating
   `route_resolve`-filtered files per target) into a reusable function, e.g.
   `compute_target_digest <target>`, returning one aggregate sha256 over
   sorted `<repo_rel>:<filehash>` lines. **Determinism pitfall to flag for
   design**: `overlay.manifest`'s row order is not alphabetically sorted
   today; concatenating in manifest-file order (as `parse_manifest`/
   `all_tracked_files` naturally does) makes the aggregate digest
   order-dependent on a file that could be reordered without any content
   change — the aggregation step MUST explicitly sort before concatenating.
4. **Digest comparison / stale-surfacing fix (R-004)**: complements, does not
   replace, today's live-vs-`main:`-tree per-file comparison in
   `cmd_sync_check`/`cmd_status`. Concrete change: when `ACTION:$t:` names
   "run apply", also resolve and print the target release version this
   would bring the target to (via the R-001 tag-read function), e.g.
   `ACTION:claude: run 'overlay apply --target claude' (new release v1.5.0
   available)`. The per-file detection mechanism itself needs no rewrite.
5. **State persistence (R-002)**: no file exists yet anywhere in the repo.
   Two live conventions to choose between at design time: (a) repo-scoped,
   mirroring `overlay.manifest`'s own precedent, e.g.
   `$OVERLAY_DIR/.overlay-state.json` (would need a `.gitignore` addition,
   today's `.gitignore` only lists `*.swp *.swo *~ .DS_Store`); or (b)
   home-scoped, mirroring gentle-ai's own `~/.gentle-ai/state.json`
   convention literally referenced at line 17 of this same file, e.g.
   `$HOME/.labdrian-overlay/state.json`. Repo-scoped fits this tool's
   git-clone (not Homebrew-install) distribution model better since
   `OVERLAY_DIR` is already the one stable identity concept in play.
6. **Backup mechanism (R-007)**: the codebase already has a full
   gentle-ai-shaped backup **consumer** (`TARBALL` at line 17,
   `cmd_capture --from-backup`'s `tar xzf` extraction, lines 635-660) but no
   **producer**. Two shapes to choose between (see Approaches below); the
   integration point either way is the same: inside `cmd_apply`'s per-target
   deploy loop (lines 792-821), immediately before the `cp "$src" "$dest"`
   at line 814, when `$dest` exists and differs from `$src` — snapshot
   `$dest`'s current bytes into a lazily-created (first-difference-only,
   matching R-007's no-op-apply-means-no-backup acceptance scenario)
   timestamped backup location before overwriting.
7. **Restore (R-008)**: new `cmd_restore`, symmetric to `cmd_apply`'s deploy
   loop (`route_resolve`/`all_tracked_files` shape) but reading FROM the most
   recent (or, per R-008's own open ambiguity, a specified) backup for the
   target and copying files back into `$rr_dest` (live paths) — the inverse
   direction of the same loop already proven in `cmd_apply`/`cmd_capture`.
8. **`doctor` extension (R-009)**: add a `resolve_targets`-style loop inside
   `cmd_doctor` (it has none today) that reads each target's recorded
   version+digest from the R-002 state file, recomputes the live digest via
   the R-003 function, and prints one `PASS`/`WARN` (recommend WARN, not
   FAIL, to preserve `doctor`'s existing "exits non-zero only on hard FAIL"
   contract — a digest mismatch is recoverable via `apply`, the same
   WARN-not-FAIL tier as today's "engine binary absent" check) row per
   target.
9. **`version` command (R-010)**: new `cmd_version`, thin reporting layer:
   repo version = R-001's tag-read function on `main`; per-target version =
   R-002's state-file read; comparison = simple semver string/tuple compare.
10. **TUI surfacing (R-011)**: `tui/run.go`'s `ParseSyncCheck` already parses
    machine-readable `VERDICT:`/`ACTION:` lines from `sync-check` stdout —
    the natural integration point is extending that exact same line-based
    protocol with new fields (recorded version, digest-match, backup
    availability) rather than inventing a second output channel; `Actions()`
    already has an established `Also`-chaining pattern (used today for
    self-update→apply) that a restore action can reuse.

### Approaches (core mechanism: release identity + per-target digest)

**Release identity**

1. **Annotated git tags, resolved via `git tag`/`git describe`** (matches
   R-001's literal wording and the gentle-ai precedent)
   - Pros: git-native, no new file/format to keep in sync with reality;
     directly satisfies R-001's stated requirement; works uniformly with the
     existing fetch/compare idioms already in `cmd_self_update`/
     `compute_repo_behind_origin`.
   - Cons: requires an explicit tag-fetch fix (the A-03 gap above); ordering/
     "latest" resolution needs a semver-aware sort (`--sort=-v:refname`) since
     plain lexical tag sort would misorder e.g. `v1.9.0` vs `v1.10.0`.
   - Effort: Low-Medium.

2. **A committed `VERSION` file at repo root, bumped by a release commit**
   - Pros: trivially readable (`cat VERSION`) without any git plumbing;
     no fetch-tags gap to fix.
   - Cons: **contradicts R-001's explicit requirement wording** ("identify
     each published release... as an annotated git tag") and the confirmed
     gentle-ai reference model (obs #3060: semver + Homebrew formula/tag, no
     committed version file); introduces a second source of truth that can
     silently drift from the actual released commit if a maintainer forgets
     to bump it on a release; adds a new tracked file whose manifest
     classification would need a decision, which the requirements brief's
     Out-of-Scope section leans against ("no manifest rewrite... unless
     evidence requires it" — no such evidence exists here).
   - Effort: Low, but dominated by Approach 1 given R-001's own wording.

   **Recommendation**: Approach 1 (git tags) — it's what R-001 already
   specifies and it reuses proven fetch/compare idioms already in this file;
   Approach 2 is documented mainly to show why a superficially-simpler
   alternative was rejected.

**Per-target digest computation/storage**

1. **Aggregate sha256 over sorted `path:filehash` lines per target**
   (directly extends `cmd_sync_check`'s existing per-file loop, and matches
   R-003's literal wording: "derived from the sha256 hashes of every
   currently-deployed managed file")
   - Pros: near-zero new hashing logic — reuses the exact `sha256sum`
     invocations already proven in `cmd_sync_check` (lines 1130-1167);
     naturally supports a future "which file differs" drill-down by keeping
     the per-file lines around; deterministic once sorted.
   - Cons: must explicitly sort by path before concatenating (manifest row
     order is not sorted today — a real correctness pitfall, noted above);
     O(n) file reads per check, but that's already true of `sync-check` today
     (no regression).
   - Effort: Low-Medium.

2. **Git tree hash via `git hash-object`/`git mktree` over the live target
   files, instead of a manual sha256-concatenation**
   - Pros: leverages git's own well-tested content-addressed hashing
     instead of hand-rolled aggregation logic.
   - Cons: significantly higher complexity and real footguns — writing
     blobs (`git hash-object -w`) for live target files (paths like
     `~/.claude/skills/...`) would pollute the OVERLAY repo's own object
     database with objects unrelated to any real commit, and the live
     target's directory shape differs from the repo's `skills/`-rooted tree
     shape, so a synthetic tree would need careful remapping just to be
     comparable; over-engineered relative to what R-003 literally asks for.
   - Effort: Medium-High.

   **Recommendation**: Approach 1 — it is what R-003 specifies verbatim and
   is a minimal, low-risk extension of code already proven correct in
   production (`cmd_sync_check`); Approach 2 would trade a solvable
   determinism footgun (fixable with one `sort`) for a much larger, riskier
   surface (object-database pollution, tree-shape remapping) for no
   observable benefit at this tool's scale.

### Recommendation

Proceed to `sdd-propose`. All four flagged assumptions (A-01..A-04) are
confirmed against the live codebase, one with materially stronger evidence
than the brief anticipated (A-01) and one with a concrete implementation gap
now identified ahead of design (A-03's tag-fetch requirement). The core
mechanism should be: annotated git tags (Approach 1) for release identity,
and a sorted-and-concatenated per-target sha256 aggregate (Approach 1) for
the digest, both chosen because they are exactly what R-001/R-003 specify and
both directly extend code already proven in this file rather than
introducing new architectural surface. `cmd_apply`'s deploy loop and
`cmd_capture --from-backup`'s existing tarball-consumption logic are the
concrete backup/restore integration seams; `ParseSyncCheck`'s existing
line-based VERDICT/ACTION protocol is the concrete TUI integration seam.

### Risks

- R-005 changes `cmd_self_update`'s convergence target on a heavily-guarded,
  production-relied command (D1-D3 refusals); the ahead-of-origin refusal
  logic itself needs no change (confirmed above), but the tag-fetch gap must
  be closed correctly or "latest tag" resolution will silently see stale data.
- The digest-aggregation determinism pitfall (manifest row order is not
  sorted) is a real, easy-to-miss correctness bug if `sdd-design`/`sdd-apply`
  don't explicitly sort before concatenating.
- R-008 `restore` is destructive-by-design (overwrites live target files) —
  needs careful confirmation UX, following the existing `Mutating`/
  `ConfirmMessage` TUI pattern already established for `apply`/`self-update`.
- State-file location (repo-scoped vs. home-scoped) is still an open design
  decision with real tradeoffs (see integration point 5) — not resolved by
  this explore, correctly deferred to `sdd-design` per the brief itself.
- `bin/overlay` (the legacy 308-line script) is shellchecked by
  `openspec/config.yaml`'s test layer instead of `bin/labdrian-overlay` — a
  pre-existing, unrelated repo inconsistency noted for awareness, not in
  scope for this change.

### Ready for Proposal

Yes — no blocking finding surfaced. All four open assumptions are resolved
(three straightforwardly confirmed, one confirmed with a concrete gap now
named ahead of time), concrete integration points are identified file-by-file
with line references, and both required mechanism comparisons favor the
approach the requirements brief already specifies, with a documented
rationale for rejecting the alternative in each case. `sdd-propose` can
proceed directly against R-001..R-011 as ordered in the requirements brief.
