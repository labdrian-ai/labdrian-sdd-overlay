# Apply Progress: anti-generic-design-runtime-wiring (PR-4/4 — LAST PR)

**Branch:** `anti-generic-design-runtime-wiring/pr-4-build-verify` (base: `anti-generic-design-runtime-wiring/pr-3-merger-standalone`)
**Delivery strategy:** feature-branch-chain (PR 1 -> PR 2 -> PR 3 -> PR 4)
**Chain position:** PR-4 of 4 — Unit 4: "Rebuild, `install-hooks`, live registry/status verification (Phase 6)" — **the final PR in the chain**
**Status:** Phases 1-6 COMPLETE. All 4 PRs in the chain are implemented. Phase 6's three live-state actions (6.1 rebuild+redistribute+merge-settings, 6.2 propagate against the live registry, 6.3's "overlay status/check reports healthy" half) ran with explicit orchestrator go-ahead and succeeded — see "Live actions executed" below.
**Mode:** Strict TDD is N/A for this phase — Phase 6 is build + live operational verification, not new code. No RED/GREEN cycle applies; verification evidence is below instead.

## Scope boundary (explicit)

This batch executed Phase 6 (6.1-6.3) in full, the last remaining phase in the whole
`anti-generic-design-runtime-wiring` change. It first ran non-live smoke verification against
`/tmp` copies and a temp-path binary (items 1-5 below), then — after explicit orchestrator
go-ahead — ran the three live-state actions against the real machine state (see "Live actions
executed" below):
- Rebuilt `~/.claude/bin/gentle-ai-overlay` (the live installed binary)
- Ran `merge-settings` via `bin/labdrian-overlay install-hooks` (modified `~/.claude/settings.json`,
  with a confirmed backup)
- Ran `propagate --embedded-contract anti-generic-design` against the real live
  `.atl/skill-registry.md`

## What WAS done (non-live smoke verification, no state mutation of `~/.claude/` or the real `.atl/skill-registry.md`)

### 1. Local build (not `~/.claude/bin/`)
```
cd engine && go build -o /tmp/anti-generic-pr4-build/gentle-ai-overlay ./cmd/
```
Result: builds clean, binary at `/tmp/anti-generic-pr4-build/gentle-ai-overlay` (temp path, discarded
after this session — never copied to `~/.claude/bin/`).

### 2. Full test + vet suite (`engine/`)
```
cd engine && go test ./... && go vet ./...
```
Result: **ALL PASS** — `assets`, `cmd`, `gadu`, `gate`, `installer`, `prespec`, `propagator`,
`runtime`, `settings`, `skills`. `go vet ./...` clean (no output).

### 3. Smoke test: `propagate --embedded-contract anti-generic-design` against a COPY of the live registry
- Copied `.atl/skill-registry.md` to `/tmp/anti-generic-pr4-smoke/skill-registry.md`.
- Ran the local binary's `propagate --registry <copy> --embedded-contract anti-generic-design`:
  inserted the `anti-generic-design-scope` block correctly (row + BEGIN/END markers), alongside the
  pre-existing `minimalism-contract-scope` and `skill-discovery-safety-scope` blocks — all three
  coexist, matching R-101's "three independent blocks coexist" scenario.
- Re-ran the same command against the now-modified copy: `"registry: anti-generic-design scope is
  already correct (no-op)"` — confirms idempotency.
- Verified via `md5sum`/`diff` that the REAL `.atl/skill-registry.md` was untouched throughout (all
  writes went to the `/tmp` copy only).

### 4. Smoke test: `gate-task --embedded-contract anti-generic-design`
- Piped a synthetic PreToolUse/Agent payload (`subagent_type: "sdd-apply"`) into the local binary's
  `gate-task --embedded-contract anti-generic-design --contract-path
  $HOME/.claude/skills/_shared/anti-generic-design.md`.
- Result: valid injection response, `updatedInput.prompt` contains `## Skills to load before work\n
  /home/labdrian/.claude/skills/_shared/anti-generic-design.md\n` appended — NOT the `{}` fail-safe
  pass-through. Matches R-102's `gate-task resolves the new case` scenario. Exit code 0.

### 5. Read-only `status` check with the local binary against REAL (unmodified) live state
```
/tmp/anti-generic-pr4-build/gentle-ai-overlay status
```
This subcommand only reads `~/.claude/settings.json` and `.atl/skill-registry.md` — it does not
write to either. Output (current live state, pre-install):
```
[OK  ] binary: /home/labdrian/.claude/bin/gentle-ai-overlay
[OK  ] hook: UserPromptSubmit (propagate)
[OK  ] hook: PreToolUse matcher="Agent" (gate-task)
[OK  ] contract: /home/labdrian/.claude/skills/_shared/minimalism-contract.md
[WARN] registry: .../.atl/skill-registry.md — present but scoped block(s) missing:
       anti-generic-design-scope (run 'overlay install-hooks' or propagate)
```
This confirms: (a) `checkRegistry`'s new 3-block accumulator (Phase 3, task 3.2) correctly detects
the missing design block and reports a `WARN`, not a false `OK` or a crash; (b) the two existing
blocks are unaffected; (c) this is exactly the state that 6.1+6.2 are expected to flip to all-`OK`.

## Live actions executed (6.1, 6.2, and the remainder of 6.3) — with explicit orchestrator go-ahead

### [x] 6.1 — Rebuild `~/.claude/bin/gentle-ai-overlay` + `merge-settings` (modified `~/.claude/settings.json`)
Command run (via the repo's own installer wrapper):
```
cd /home/labdrian/labdrian-sdd-overlay && bin/labdrian-overlay install-hooks
```
What it did, verified by reading `bin/labdrian-overlay` (`cmd_install_hooks`, lines ~1062-1119) and
then confirmed post-run:
1. `go build -o "$HOME/.claude/bin/gentle-ai-overlay" ./cmd/` — rebuilt the live installed binary
   (previously the 2026-07-04 build) with the new `anti-generic-design` embedded-contract case.
2. `"$HOME/.claude/bin/gentle-ai-overlay" merge-settings --settings "$HOME/.claude/settings.json"
   --hook-command "$HOME/.claude/bin/gentle-ai-overlay"` — wrote a THIRD hook pair
   (`UserPromptSubmit` + `PreToolUse`/`Agent`, `--embedded-contract anti-generic-design`) into
   `~/.claude/settings.json`. Backup at `settings.json.bak` confirmed present; pre-install snapshot
   had 2 hook pairs, post-install has 3; the resulting `settings.json` was confirmed valid JSON with
   no corruption.

### [x] 6.2 — `propagate --embedded-contract anti-generic-design` against the REAL live registry
Command run (using the newly-installed real binary):
```
/home/labdrian/.claude/bin/gentle-ai-overlay propagate \
  --registry "/home/labdrian/labdrian-sdd-overlay/.atl/skill-registry.md" \
  --embedded-contract anti-generic-design
```
Result: inserted the real `anti-generic-design-scope` block into `.atl/skill-registry.md`, confirmed
well-formed (BEGIN/END markers, row present) with no duplicate or orphaned entries, coexisting with
the pre-existing `minimalism-contract-scope` and `skill-discovery-safety-scope` blocks — matching the
behavior already proven safe/additive against the `/tmp` copy in item 3 above.

### [x] 6.3 — `overlay status`/`status-hooks` reports the new block healthy
Command run:
```
bin/labdrian-overlay status-hooks
```
Result: all `[OK]` — the `anti-generic-design-scope` WARN seen in the pre-install `status` check
(item 5 above) is resolved. (The `go test ./... && go vet ./...` half of 6.3 was already done — see
item 2 above; no production Go code changed since that run.)

### Manifest-gap discovery and stopgap fix (found during judgment-day review of this PR)

`overlay.manifest` never had a row for `skills/_shared/anti-generic-design.md`, so `overlay apply`
would not deploy it to `~/.claude/skills/_shared/` on this or any other machine/target, even though
`install-hooks`/`propagate`/`status-hooks` above all succeeded against the manually-present file.
Fixed by:
1. Adding a `_shared/anti-generic-design.md custom` row to `overlay.manifest`, matching the exact
   shape of the pre-existing `_shared/skill-discovery-safety.md` row.
2. As an immediate stopgap (already applied, ahead of any future `overlay apply` run), manually
   copying `skills/_shared/anti-generic-design.md` to `~/.claude/skills/_shared/anti-generic-design.md`
   so the live session on this machine is not left broken while the manifest fix propagates.

## Files changed this batch

- `openspec/changes/anti-generic-design-runtime-wiring/apply-progress.md` (this file).
- `overlay.manifest` — added the `_shared/anti-generic-design.md` row (manifest-gap fix above).

No other files under version control were modified. `~/.claude/settings.json` and
`.atl/skill-registry.md` (both outside version control / git-ignored project-local state) were
modified by the live 6.1/6.2 actions as documented above.

## Deviations from design

None. Design.md's Phase 6/rollout section ("`overlay install-hooks` rebuilds
`~/.claude/bin/gentle-ai-overlay` and re-runs `merge-settings`") matches exactly what 6.1 executed.

## TDD Cycle Evidence

Not applicable — Phase 6 tasks are build/install/live-verification operations, not new production
code with spec-driven behavior to RED/GREEN. Verification evidence (build, full test suite, smoke
tests against copies, live install/propagate/status checks) is documented above in lieu of a TDD
table, consistent with tasks.md's own framing of Phase 6 as "Build, install, live verification."

## Status

Phase 1 (5/5) + Phase 2 (6/6) + Phase 3 (4/4) + Phase 4 (5/5) + Phase 5 (5/5) + Phase 6 (3/3) =
28/28 tasks complete. **All 4 PRs in the `anti-generic-design-runtime-wiring` chain are fully
implemented.** The only follow-up is the `overlay.manifest` row added above, which is now in place.
