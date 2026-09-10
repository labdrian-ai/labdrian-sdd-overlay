# Proposal: longterm-mem sync triggers at session close and archive

## Intent

`longterm-mem sync` exists but nothing invokes it automatically; the vault only fills when an agent remembers to call `promote`/`sync`. With curated-`topic_key` eligibility landed (`longterm-mem-promotion-scoping`, 0a461d9), an unattended direct-write sync is now safe enough to wire to two natural moments: Claude Code `SessionEnd` (DEC-001) and SDD change archive (whole project, DEC-002; direct write, DEC-003). Every trigger must be best-effort: never fail, block, or delay the host (R-003).

## Scope

### In Scope
- Slice 1 `session-end-hook` (R-002, R-003): a fifth Labdrian-owned hook family, `SessionEnd`, installed/removed/status-checked by `engine/settings` with its own identity token, coexisting with the `moshi-hook` and `gentle-ai` entries already in that array; plus the shared non-blocking runner as an engine subcommand.
- Slice 2 `archive-trigger` (R-001, R-003): an overlay-owned archive call site that reuses the runner.
- Outcome logging to an operator-discoverable file.

### Out of Scope
- Any edit to the managed `sdd-archive/SKILL.md` or to `longterm-mem` sync/eligibility internals.
- A `Stop` trigger, change-scoped archive sync, or dry-run step (DEC-001..003).
- Retroactive sync of past sessions/archives; removing the explicit `promote`/`sync` path.

## Capabilities

### New Capabilities
- `longterm-mem-sync-triggers`: non-blocking runner contract (skip/failure/timeout/log semantics), the `SessionEnd` hook entry, and the archive-time call site.

### Modified Capabilities
- `runtime-lifecycle`: Claude Lifecycle Support now includes the `SessionEnd` family in owned-state install, uninstall, and honest `supported` status.

## Approach

**Runner** (engine subcommand, e.g. `gentle-ai-overlay longterm-mem-sync --event session-end|archive --cwd DIR`):
1. Locate `$STATE_DIR/bin/longterm-mem` (`~/.labdrian-overlay`); absent -> log `skip:no-binary`, exit 0.
2. Re-exec itself detached (`Setsid`, stdio to the log file) so the hook returns immediately; the child runs `longterm-mem sync` with cwd = project dir under a bounded timeout (~60 s).
3. Classify the child's exit: vault-not-configured -> `skip:no-vault`; timeout -> `timeout`; other non-zero -> `failure`. Append one line per firing to `~/.labdrian-overlay/logs/sync-trigger.log`. Parent always exits 0.

**SessionEnd hook**: command shaped like the existing entries (`command -v <engine> && <engine> longterm-mem-sync --event session-end --cwd "${CLAUDE_PROJECT_DIR:-$PWD}" || true`), dedup identity = engine path + `longterm-mem-sync` token; `mergeHooks`/`removeHooks` only touch entries matching that identity.

**Archive call site**: `skills/inception-pipeline/SKILL.md` Closure-Feedback gains a final step invoking the runner with `--event archive`. Rationale: it is the only overlay-owned surface that already fires after the engine archives and exists precisely to avoid forking `sdd-archive`; behavior stays deterministic in Go, the skill only names the command. A `bin/labdrian-overlay longterm-mem sync-trigger` verb is the operator-facing wrapper.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `engine/settings/settings.go`, `settings_test.go` | Modified | SessionEnd family: builder, identity, merge/remove, status |
| `engine/cmd/main.go` (+ new `engine/synctrigger/`) | New | Runner subcommand and tests |
| `bin/labdrian-overlay` | Modified | `longterm-mem sync-trigger` verb; install/uninstall messages |
| `skills/inception-pipeline/SKILL.md` | Modified | Closure-Feedback step |
| `openspec/specs/runtime-lifecycle/spec.md` | Modified | Delta spec |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Unattended direct write promotes noise; hygiene of non-`sdd/` `topic_key`s now decides vault content (issue #293) | Med | R-030 local-edit precedence; log every run; explicit `sync` unchanged for review |
| Detached child outlives session or races a manual sync | Low | Timeout; `longterm-mem` per-directory index lock (ca33796) |
| Existing installs report `partial` until `install-hooks` reruns | High | Documented; `labdrian doctor` names the remedy |
| Changes archived outside inception-pipeline miss the archive trigger | Med | SessionEnd catches them at session close |

## Rollback Plan

`labdrian uninstall-hooks` removes only the SessionEnd entry (`.bak` retained); revert the two slice PRs. The vault keeps already-promoted pages (harmless; local-edit precedence).

## Dependencies

- `longterm-mem-promotion-scoping` (merged 0a461d9).

## Success Criteria

- [ ] `settings_test.go`: SessionEnd entry added once, coexists with foreign entries, removed cleanly, never added to `Stop`.
- [ ] Runner tests: missing binary, missing vault, failing sync, timeout -> exit 0 + log line.
- [ ] Scripted check: session close and closure-feedback each produce a whole-project sync log entry.
