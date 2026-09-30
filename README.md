# labdrian-sdd-overlay

A customization layer over `gentle-ai` (an SDD-driven, multi-agent dev runtime) that lets your overlaid skills and rules **survive `gentle-ai` updates**, via a two-branch model: `upstream` (pristine vendor baseline) + `main` (your customizations on top).

**What it does:**

- Tracks vendor-managed and custom skills in a git overlay, so `gentle-ai sync`/`upgrade` never silently clobbers your changes.
- Deploys your overlaid skills to **three agent runtimes**: Claude Code (`~/.claude/skills`), opencode (`~/.config/opencode/skills`), and codex (`~/.codex/skills`).
- Deploys Claude Code **agent definitions** (e.g. GADU) to `~/.claude/agents/` — agents are Claude Code-only; opencode/codex receive the portable skill form instead.
- Adds a **deterministic minimalism-scoping layer**: a Go engine + Claude Code hooks (`UserPromptSubmit` → propagate, `PreToolUse`/`Agent` → gate-task) that inject two managed contracts (minimalism, anti-generic design) **only** into the code-writing SDD phases (`sdd-tasks`/`sdd-apply`) and exclude them from all others. Deterministic on Claude Code; documented platform limits apply on opencode/codex.
- Cuts and tracks **named releases** (semver tags via CI) as an additional layer on top of the upstream/main model — versioning, per-target state, and rollback. See [Releases](#releases) below.

## Quick start — clone, then `labdrian tui` from anywhere

The goal: clone the repo, install one global command, and from **any** directory run `labdrian tui` to see whether your skills are out of date and update them on the spot.

```bash
# 1. Clone to any directory you like (~/labdrian-sdd-overlay is a sensible default,
#    but the tool resolves its own location — any path works).
git clone https://github.com/labdrian-ai/labdrian-sdd-overlay.git ~/labdrian-sdd-overlay
cd ~/labdrian-sdd-overlay

# 2. Materialize the upstream branch locally.
#    A fresh clone only checks out `main`; sync-check / the TUI need a local `upstream` ref.
git branch --force upstream origin/upstream

# 3. Install the global `labdrian` command (symlinks bin/labdrian-overlay into ~/.local/bin).
bin/labdrian-overlay install-alias

# 4. Make sure ~/.local/bin is on your PATH. If `labdrian` is not found, add this to
#    your ~/.bashrc or ~/.zshrc and reopen the shell:
#      export PATH="$HOME/.local/bin:$PATH"
```

**Prerequisite:** the TUI runs `go run .`, so you need **Go installed** (`brew install go` or https://go.dev/dl).

Now, from any directory:

```bash
labdrian tui     # see per-target drift / gentle-ai sync state, and apply updates
```

The TUI shows whether each target is in sync with gentle-ai and lets you re-capture and re-apply your overlay without leaving the dashboard. Prefer the CLI? See [Usage](#usage) below.

## What this is

`gentle-ai sync` and `upgrade` overwrite `~/.claude/skills/` with pristine vendor files. This repo uses two long-lived git branches to track the vendor baseline and your customizations separately, then merges and deploys with a single command.

- **`upstream` branch**: pristine vendor baseline extracted from gentle-ai upgrade backups
- **`main` branch**: your customized overlay — merges on top of each new upstream

## Deploy targets

| Target | Path |
|--------|------|
| `claude` | `~/.claude/skills` |
| `opencode` | `~/.config/opencode/skills` |
| `codex` | `~/.codex/skills` |

Use `--target <name>` on `apply`, `status`, `capture`, and `sync-check`. Default for `apply`/`status`/`sync-check` is `all` (all three targets, plus `pi` when it is genuinely installed). Default for `capture` is `claude`.

The `agent` route (see [Tracked files](#tracked-files-overlaymanifest)) additionally deploys to `~/.claude/agents` (claude target only). `--target opencode` or `--target codex` on an agent row is a no-op — zero applicable targets.

### Pi (via gentle-pi)

Pi is a **package target**, not a per-file copy target: `--target pi` on `apply`/`status`/`sync-check` builds a `labdrian-pi` package (`package.json`, `skills/`, `agents/`, `extensions/`, `mcp.json`) into `$STATE_DIR/pi/labdrian-pi` from the same `skills.registry.yaml`/`agents/` source every other target reads, instead of copying files into a runtime-owned directory.

- `apply --target pi` builds the package, then — when a `pi` CLI is on `PATH` — runs `pi install <path>` so the change is picked up on the next Pi session. Without `pi` on `PATH` it prints the install hint instead.
- `status --target pi` reports honest per-entry proof: built, in sync with the current manifest, and listed in `~/.pi/agent/settings.json`'s `packages` array (read-only — this overlay never writes that file itself). It always discloses that `pi --no-extensions` bypasses the contract-gate extension and `pi --no-skills` bypasses skill discovery for that session, since neither flag's use can be detected at runtime (short aliases `-ne` and `-ns`).
- Uninstalling runs `pi remove <path>` (not `pi uninstall`, which does not exist) and removes the built package directory — it never edits `~/.pi/agent/settings.json` or `~/.pi/agent/mcp.json` directly, and never touches any gentle-pi- or pi-engram-owned entry.
- A `before_agent_start` package extension injects the same bare contract-path line Claude/OpenCode/Codex receive into the `sdd-tasks`/`sdd-apply` system prompt, composing with (not overwriting) gentle-pi's own handler output.
- `longterm-mem register --target pi` writes its MCP entry into the package's own `mcp.json`; `--target all` only attempts Pi when the package is both built and genuinely installed.
- **GADU as a real Pi subagent.** `apply --target pi`, right after `pi install`, ensures GADU has a working `subagent_*` dispatch runner: when `~/.pi/agent/settings.json` already lists `npm:gentle-pi` at version 2.6.0 or later, its **native** `subagent_*` tools are used and the third-party [Pi Subagents extension](https://www.npmjs.com/package/pi-subagents-j0k3r) is never installed — installing it while native support is present leaves gentle-pi's own tools unregistered, so if that obsolete extension is already present the output discloses the conflict and names the exact removal command (`pi remove npm:pi-subagents-j0k3r`) without removing it (it is not overlay-owned). Only when gentle-pi's native subagents are unavailable (older gentle-pi, or none installed) does it fall back to installing `npm:pi-subagents-j0k3r` via a fixed `pi install` argv — unless `pi-subagents` (the alternate package name) is already installed, or `LABDRIAN_PI_SKIP_SUBAGENTS=1` opts out. Either way it then links the package's own generated `agents/GADU.md` at `~/.pi/agent/agents/GADU.md`, so GADU is dispatchable through whichever runner is active instead of only being a relayed persona. That link is overlay-owned: ownership is proven by `readlink` equality with the stable package path (only the inode changes across a `swap` rebuild), a pre-existing foreign file or symlink is left untouched and reported as a conflict, and `status --target pi` reports the subagent runner's state (native/legacy/conflict/absent) and the link's state (missing/current/stale/conflict) as two separate, honest entries. `runtime uninstall --target pi` removes only that symlink — never the extension package, never gentle-pi's own package, and never any gentle-pi- or pi-engram-owned file.
- **Pi-specific GADU model and body.** The `agents/GADU.md` the Pi package ships is built from `pi/agents/GADU.md` (falling back to the generic `agents/GADU.md` only on an older checkout that predates it), not the same bytes Claude Code and opencode receive. It declares `model: pi-claude-cli/claude-sonnet-5` and carries a deliberately compact body (under 1.5 KB) that defers the full persona to the `gadu-operator` skill. Both choices trace to findings verified live 2026-09-13 against Pi 0.85.1 / gentle-pi 2.6.0 / `@saccolabs/pi-claude-cli` 0.8.1: gentle-pi's native `subagent_run` only completes a child whose agent file names a `pi-claude-cli/<model>` id (an `anthropic/*` or `openai-codex/*` model, or an absent `model:`, ends the child with an error), and the `pi-claude-cli` bridge — which relays the agent's system prompt to a fresh Claude Code process via `--append-system-prompt-file` — hangs indefinitely on a ~7 KB prompt (GADU's full persona body included) while a compact one completes normally.

## Tracked files (overlay.manifest)

| File | Type |
|------|------|
| `sdd-spec/SKILL.md` | managed (vendor+overlay) |
| `sdd-tasks/SKILL.md` | managed (vendor+overlay) |
| `sdd-verify/SKILL.md` | managed (vendor+overlay) |
| `sdd-verify/references/report-format.md` | managed (vendor+overlay) |
| `sdd-verify/strict-tdd-verify.md` | managed (vendor+overlay) |
| `requirements-from-transcripts/SKILL.md` | custom (overlay only) |

**managed**: tracked on both `upstream` and `main`. Gets merged when upstream updates.
**custom**: only on `main`. Never on upstream. Added as a customization with no vendor counterpart.
**external**: `source.type: external` — records provenance metadata only (`repo` = origin URL, `ref` = vendored commit/ref). The overlay **never fetches, clones, or executes any remote resource**. Vendoring is a human responsibility; `--repo`/`--ref` are inert labels for a file you have already reviewed and committed locally.

**`route` column (optional third column):** each manifest row may end with `skill` or `agent` (default: `skill` when omitted). Bare two-column rows are unchanged. The `agent` route tells the installer to source from `agents/<path>` and deploy to `~/.claude/agents` instead of any skills directory. Non-deployable rows (engine source files, root-level files) are recognized by the installer and skipped silently.

## Usage

`labdrian` is the alias to `bin/labdrian-overlay`, installed via `labdrian-overlay install-alias`. All commands below work as `labdrian <command>` or `bin/labdrian-overlay <command>` from the repo root. (The pristine `bin/overlay` is a vendored copy of gentle-ai upstream — labdrian never edits it, so it never conflicts on sync.)

### Launch

```bash
labdrian tui          # recommended: full TUI dashboard
```

The TUI wraps the CLI — everything below is also available as individual commands. The
dashboard surfaces per-target drift for **agent files** (e.g. the GADU agent in
`~/.claude/agents`) as a dedicated Agents sub-section alongside skills. Read-only skills
registry actions (`skills validate`, `list`, `status`) are available directly from the
action menu without leaving the dashboard. `skills validate` also cross-checks the
`skills/` directory itself against `overlay.manifest` and exits non-zero on any
divergence. The dashboard also shows each target's recorded release version and
digest-match status, and offers a confirmed "Restaurar respaldo" (restore) action —
gated on a backup actually existing for the selected target(s) — routed through the
same confirm→run→result pattern as apply/self-update.

### Action map

| Command | Mode | What it does |
|---------|------|--------------|
| `status` | read-only | Per-file drift between repo and each live target |
| `sync-check` | read-only | **The compass** — tells you exactly what to do (see verdicts below); VERDICT lines also carry release version and digest-match per target |
| `apply [--target claude\|opencode\|codex\|all]` | **modifies** | Merge upstream→main, then deploy overlay to target(s). Shows confirmation before mutating. Backs up each target's currently-deployed managed files first. |
| `capture [--target claude\|opencode\|codex]` | **modifies** | Pull a gentle-ai update into the `upstream` branch. Single target only. |
| `self-update` | **modifies** | Fast-forward local `main` (never the current branch) to the latest published release tag — never past it, even if `origin/main` carries untagged commits beyond it. Falls back to raw `origin/main` HEAD convergence before any release tag exists. Refuses on a dirty tracked tree, local-ahead main, or no `origin` remote. |
| `update` | read-only | Report the latest published release version and each target's recorded version (up-to-date / behind / never deployed). Never mutates anything. |
| `restore --target claude\|opencode\|codex [--list] [--backup TIMESTAMP]` | **modifies** | Roll a single target back to one of its retained backups (up to 3, auto-pruned; default: most recent). Refuses `--target all`. `--list` shows retained backups without changing anything. |
| `version` (also: `--version`) | read-only | Print this clone's current release version and each target's recorded deployed version. |
| `install-hooks` | **modifies** | Build the Go engine binary + wire `UserPromptSubmit`/`PreToolUse`/`Agent` hooks, including the workflow projection family and the skills approve guard, into `~/.claude/settings.json` (backs up to `.bak` first). Run once to activate scoping; re-run it after an upgrade that adds a hook family, then restart Claude Code to load the hooks. |
| `uninstall-hooks` | **modifies** | Remove the overlay hook entries (the minimalism and design pairs, the SessionEnd sync-trigger, the review-receipt and shaper guard entries, the three projection entries, and the two approve guard entries) from `~/.claude/settings.json`, including entries left by contracts retired in earlier versions, leaving all other keys intact. |
| `status-hooks` | read-only | Check engine binary, hooks wired, contracts readable — exits 0 if all healthy; missing binary exits non-zero with `run 'overlay install-hooks'` guidance. |
| `doctor [--fix]` | read-only | Host-toolchain preflight: go, gentle-ai, discovery tools (bat/rg/fd/sd/eza), engine binary, skill registry — plus a per-target version/digest consistency row (WARN only, never fails the exit code). `--fix` best-effort installs missing discovery tools via Homebrew. |
| `validate-entry-contract --schema PATH --instance PATH` | read-only | Validate a pre-SDD entry candidate against the version-matched schema and deterministic cross-field rules. |
| `install-alias [name]` | **modifies** | Symlink `labdrian` (or a custom name) into `~/.local/bin`. Run once per machine. |

### The 3 workflows

**1. Day-to-day — nothing to do.**
The hooks run in the background: `UserPromptSubmit` (propagate) keeps skill registries fresh across sessions; `PreToolUse`/`Agent` (gate-task) injects the minimalism and anti-generic-design contracts into `sdd-tasks`/`sdd-apply` automatically. Review candidate coverage and skill discovery are handled by gentle-ai itself since v2.7.0; the overlay no longer ships contracts for them.

**2. gentle-ai released an update.**
```bash
labdrian sync-check                         # UPSTREAM_CHANGED detected
labdrian capture --target claude            # pull new vendor files into upstream
labdrian apply                              # re-merge your customizations + redeploy
labdrian sync-check                         # confirm: healthy
```

**3. You edited a skill.**
```bash
# edit skills/<path>.md in your editor
labdrian apply                              # redeploy to all targets
labdrian sync-check                         # confirm in-sync
```

The pre-SDD entry bundle is tracked as four inseparable assets: `inception-pipeline/SKILL.md`, `_shared/pre-sdd-contracts.md`, `_shared/entry-contract.schema.json`, and `_shared/actuals-record.schema.json`. Version `2.1.0` also uses the isolated validator in `tools/entry-contract-validator`; its dependency is intentionally not added to `engine/go.mod`. The version is a compatibility set, not an exact-match lock: `2.1.0` still validates contracts written by the `2.0.0` bundle.

`labdrian apply` propagates all four tracked assets to Claude, OpenCode, and Codex. Restart any already-running client after deployment so it reloads the updated skill and shared contracts. Validate a candidate without deploying anything:

```bash
labdrian validate-entry-contract \
  --schema skills/_shared/entry-contract.schema.json \
  --instance /path/to/entry-candidate.json
```

**4. One-time hooks setup (run once per machine).**
```bash
labdrian install-hooks                      # build engine, wire hooks into settings.json
labdrian status-hooks                       # all green?
```

### The compass — sync-check verdicts

`sync-check` compares three things per target: live file vs. `main` (overlay) and live file vs. `upstream` (vendor baseline).

| Verdict | Meaning | Action |
|---------|---------|--------|
| `UPSTREAM_CHANGED` | gentle-ai updated a file; overlay not yet re-captured | `capture` → `apply` |
| `OVERLAY_NOT_DEPLOYED` | your customization exists in the repo but isn't live | `apply` |
| healthy (no flags) | everything in sync | nothing to do |

Each VERDICT line also carries `REPO_BEHIND_ORIGIN`, `REPO_BEHIND_RELEASE`, `RECORDED_VERSION`, and `DIGEST_MATCH` — see [Releases](#releases) below.

```
VERDICT:claude:UPSTREAM_CHANGED=5 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=0 REPO_BEHIND_RELEASE=0 RECORDED_VERSION=v1.2.0 DIGEST_MATCH=no
ACTION:claude: gentle-ai sync detected: run 'overlay capture --target claude' then 'overlay apply'
```

> **Convention:** commands that **read only** are always safe to run. Commands marked **modifies** show a confirmation first and, for hook operations, write a `.bak` backup before touching `~/.claude/settings.json`.

## Releases

The upstream/main model above is what makes your customizations survive a `gentle-ai` sync — that's unchanged. On top of it, the overlay repo itself now ships **named releases**: CI cuts an annotated semver tag (`vX.Y.Z`) on every push to `main` (conventional-commit bump — `feat:` → minor, `!`/`BREAKING CHANGE:` → major, else patch; skips if `HEAD` is already tagged; bootstraps `v1.0.0` on the very first tag), the same model `gentle-ai` itself uses for its own releases.

| Command | What it tells you |
|---------|--------------------|
| `overlay version` (or `--version`) | This clone's release version — what `apply` would deploy right now — plus each target's recorded deployed version |
| `overlay update` | Read-only: the latest **published** release vs. each target's recorded status (up-to-date / behind / never deployed) |
| `overlay self-update` | Converges local `main` to the latest **published release tag** — not raw `origin/main` HEAD |

Every `overlay apply` automatically backs up each target's currently-deployed managed files before overwriting them (retains the last 3 per target, auto-pruning older ones). `overlay restore --target claude|opencode|codex [--list] [--backup TIMESTAMP]` rolls that target back to one of them.

## Normal update cycle (per target)

After every `gentle-ai sync` or `gentle-ai upgrade`:

```bash
# 1. Refresh upstream with new vendor files
overlay capture --from-backup ~/.gentle-ai/backups/<new-backup>/snapshot.tar.gz
# (or: overlay capture --target claude   — to pull from ~/.claude/skills/ directly)

# 2. Merge upstream into main and deploy to all targets
overlay apply

# 3. Check drift at any time
overlay status
overlay status --target opencode
```

That's it. If there are merge conflicts, `overlay apply` exits 1 and tells you exactly which files to resolve.

## sync-check — validating gentle-ai sync state

`sync-check` compares three things per target:

- **UPSTREAM_CHANGED**: the live file at the target differs from the upstream (vendor) baseline. This means gentle-ai updated the file but your overlay has not been re-captured/re-applied yet.
- **OVERLAY_NOT_DEPLOYED**: the repo's `main` version of the file differs from (or is missing at) the live target. This means `overlay apply` hasn't been run for this target.

```bash
overlay sync-check                    # check all three targets
overlay sync-check --target claude    # check only ~/.claude/skills
overlay sync-check --check-origin     # also fetch origin for a live REPO_BEHIND_ORIGIN/REPO_BEHIND_RELEASE count
```

Each target section ends with a `VERDICT` line and an `ACTION` recommendation. The VERDICT line also
reports `REPO_BEHIND_ORIGIN`, `REPO_BEHIND_RELEASE`, `RECORDED_VERSION`, and `DIGEST_MATCH` — see
[Releases](#releases):

```
VERDICT:claude:UPSTREAM_CHANGED=5 OVERLAY_NOT_DEPLOYED=0 REPO_BEHIND_ORIGIN=0 REPO_BEHIND_RELEASE=0 RECORDED_VERSION=v1.2.0 DIGEST_MATCH=no
ACTION:claude: gentle-ai sync detected: run 'overlay capture --target claude' then 'overlay apply'
```

## One-time bootstrap

> **Note:** This is only for *creating* the overlay from scratch (seeding `upstream`/`main` from a backup tarball). If you cloned the repo, the branches already exist — skip this and use [Quick start](#quick-start--clone-then-labdrian-tui-from-anywhere) instead.

```bash
# Clone or copy this repo anywhere, then cd into it:
cd labdrian-sdd-overlay   # the directory you cloned into
chmod +x bin/labdrian-overlay
bin/labdrian-overlay bootstrap
```

Bootstrap will:
1. Init the git repo
2. Extract managed files from the backup tarball onto `upstream`
3. Create `main` from `upstream`, then layer in your current customized files from `~/.claude/skills/`

## Adding a new tracked skill

1. Add a line to `overlay.manifest`:
   - `<relative-path> managed` — if it has a vendor counterpart
   - `<relative-path> custom` — if it's purely your own addition
2. If managed: run `overlay capture --from-backup <tarball>` so upstream gets the vendor copy
3. Copy your customized version into `skills/<path>` and commit on `main`
4. Run `overlay apply` to deploy to all targets (or `--target <name>` for one)

**To add an agent instead**: place the file under `agents/`, add a manifest row `agents/<NAME>.md  custom  agent`, and run `overlay apply`. The agent deploys to `~/.claude/agents` only (Claude Code); no skill path is touched.

## GADU — shipped portable operator

The overlay ships **GADU**, a portable operator persona generated from a single canonical source: `engine/gadu/persona/body.md`.

Running `overlay gadu-generate [--check]` forwards to the engine with
`OVERLAY_DIR` set to the repository root. Without `--check`, it regenerates
four artifacts from that one canonical source:

- `agents/GADU.md` — Claude Code agent definition (deployed to `~/.claude/agents`)
- `opencode/agents/GADU.md` — Opencode-native agent definition
- `pi/agents/GADU.md` — Pi-native subagent definition: `model: pi-claude-cli/claude-sonnet-5` and a compact (< 1.5 KB) body that defers the full persona to the `gadu-operator` skill — see the Pi section above for why. `pipkg.Build` copies this file into the built package's `agents/GADU.md` when it is present, falling back to the generic `agents/GADU.md` on an older checkout.
- `skills/gadu-operator/SKILL.md` — portable skill (deployed to all three skill runtimes)

All generated files carry a `<!-- GENERATED — DO NOT EDIT. Source: engine/gadu/persona/body.md. Run: gentle-ai-overlay gadu-generate -->` header. Edit the canonical source, then regenerate — do not edit the output files directly.

## CLI reference

```
overlay bootstrap
    One-time setup: init repo, seed upstream, create main

overlay capture [--target claude|opencode|codex] [--from-backup <tarball>]
    Refresh upstream from target (default: claude).
    --from-backup reads from a specific backup tarball instead of live files.

overlay apply [--target claude|opencode|codex|all]
    Merge upstream into main and deploy to target(s) (default: all). Backs up each
    target's currently-deployed managed files first (retains last 3, auto-pruned),
    then records the deployed release version + content digest per target.

overlay self-update
    Fast-forward ONLY local main (never the current branch), then return to the
    branch you were on. Converges to the latest published release tag (never past
    it, even when origin/main carries untagged commits beyond that tag); before the
    first release tag exists, falls back to legacy origin/main-HEAD convergence.
    Refuses (exit 1) on a dirty tracked tree, a local main ahead of origin/main, or
    no 'origin' remote. Untracked files never block it.

overlay update
    Read-only: report the latest published release and each target's recorded
    version (up-to-date/behind/never deployed). Refreshes only cached tags/remote
    refs -- never a branch head, the working tree, target files, or state.

overlay restore --target claude|opencode|codex [--list] [--backup TIMESTAMP]
    DESTRUCTIVE: rolls a single target back from one of its retained backups (up to
    3, auto-pruned; default: most recent). Refuses --target all. --list shows
    retained backups (timestamp, version) without changing anything. --backup
    TIMESTAMP picks a specific one. Exits non-zero without touching any file when
    the target has no backups. Performs zero git operations.

overlay version (also: --version)
    Read-only: print this clone's current release version (from local main) and
    each target's recorded deployed version, naming any that are behind.
    Never-deployed targets are reported honestly, never fabricated.

overlay status [--target claude|opencode|codex|all]
    Show branch, diff stat upstream..main, and per-file drift per target (default: all).

overlay sync-check [--target claude|opencode|codex|all] [--check-origin|--fetch]
    Validate gentle-ai sync state: UPSTREAM_CHANGED and OVERLAY_NOT_DEPLOYED per
    target (default: all). Also reports REPO_BEHIND_ORIGIN, REPO_BEHIND_RELEASE,
    RECORDED_VERSION, and DIGEST_MATCH on each VERDICT line. Default is a
    cached-ref comparison (no network call); --check-origin (alias --fetch) runs
    'git fetch origin' first for a live count.

overlay install-hooks
    Build the Go engine binary and wire the overlay's hook families into
    ~/.claude/settings.json: the deterministic-scoping pairs (minimalism-contract and
    anti-generic-design), a SessionEnd entry that fires a non-blocking longterm-mem
    sync at session close, the review-receipt and shaper clearance guards, the workflow
    projection hooks, and the skills approve guard (below). Backs up settings.json
    to settings.json.bak before modifying. Run once to activate; inert until then.
    Re-run it after an upgrade that adds a hook family, then restart Claude Code, which
    loads hooks only when it starts.

overlay uninstall-hooks
    Remove every overlay hook entry (each family install-hooks wires, projection and approve
    guard included) from ~/.claude/settings.json, including entries left by contracts retired
    in earlier versions, leaving all other keys and hooks intact.

overlay status-hooks
    Check overlay installation health: binary present, hooks wired, contracts readable.
    If the engine binary is missing, exits non-zero and directs users to run `overlay install-hooks`.
    Exits 0 if all OK, 1 if any check fails. Safe to run at any time.

overlay doctor [--fix]
    Read-only host-toolchain preflight: checks go (required), gentle-ai, bat/rg/fd/sd/eza,
    the engine binary, a non-empty skill-registry, and a per-target release
    version/digest consistency row. Prints PASS/WARN/FAIL per check; exits non-zero
    only on a hard FAIL (the version/digest row is WARN-only, never fails the exit
    code). --fix: after the checks, attempt to install any missing discovery tools
    (bat rg fd sd eza) via Homebrew (best-effort, non-fatal), then re-check and
    report the result.

overlay tui
    Launch the Go/Bubbletea TUI front-end (target selection + gentle-ai sync dashboard).

overlay install-alias [name]
    Symlink a `labdrian` command into ~/.local/bin (run once per machine), then `labdrian tui`.

overlay gadu-generate [--check]
    Forward to the engine `gadu-generate` command with OVERLAY_DIR set to this repo root.
    Regenerates (or checks):
    - agents/GADU.md
    - opencode/agents/GADU.md
    - pi/agents/GADU.md (compact body, model: pi-claude-cli/claude-sonnet-5)
    - skills/gadu-operator/SKILL.md
    from engine/gadu/persona/body.md.

overlay validate-entry-contract --schema PATH --instance PATH [--exists-root PATH]
    Build and run the isolated v2 entry-contract validator from a temporary binary.
    Relative paths resolve from the caller's working directory. Exit codes 2-7 are
    preserved; no installed skill root is modified. Exit 7 means the instance
    declares no contract_version at all: a pre-v2 legacy contract that was not
    validated, as distinct from a corrupt one. Optional --exists-root stats every
    declared openspec_path under that root; it is off by default and is an
    inception-time check for a live change directory, never for archived history.

overlay skills <verb>
    Manage the skills registry (skills.registry.yaml) and overlay.manifest.
    list         [--registry <path>]                                               print sorted registry entries (id, source type, update strategy, targets)
    status       [--registry <path>]                                               print count summary (total / core / custom)
    validate     [--registry <path>] [--manifest <path>] --source-root <path>      cross-check registry vs manifest and skills/ on disk vs manifest, and that every global skill has a valid approval record (the 37 skills present at the Phase 8 base are grandfathered while their bytes are unchanged); exit 1 on any divergence
    install      [--registry <path>] [--source-root <path>] [--project-id <id>]   install the project-scoped skills admitted for the project into <cwd>/.claude/skills/ and <cwd>/.agents/skills/, replacing only what it installed and left unmodified (see below)
    adopt        [--registry <path>] [--source-root <path>] [--project-id <id>]   record skill directories already in the project as installed by `skills install`, only when they are exactly the current source
    add          <id> [--registry <path>] [--manifest <path>] [--source-root <path>] [--repo <url>] [--ref <sha>]  register a skill (custom or external); refused, with nothing written, unless skills/<id>/.approval.json approves the exact SKILL.md bytes (see approve)
    remove       <id> [--registry <path>] [--manifest <path>]                      unregister a skill from registry and manifest
    sync-manifest [--registry <path>] [--manifest <path>]                          regenerate */SKILL.md rows from registry; preserves all non-skill lines
    approve      --id <id> --approver <label> --source-root <path>                 record a human approval of skills/<id>/SKILL.md in skills/<id>/.approval.json, bound to the SHA-256 of its exact bytes
                 The engine cannot prove a human ran approve: the record only proves it matches the exact bytes of the skill beside it.
                 Approval is a human step: a PreToolUse hook, installed by install-hooks, denies the agent running approve or writing the record by hand (see
                 `skills guard-hook` below). It is a speed bump, not a security boundary; a person runs approve in a terminal.
    What install owns. install records every file it writes, with its SHA-256, in the project lock
    (.labdrian/procedural-skills.lock.json, in an "installs" array beside the procedural skills; an older program that
    reads a lock with installs refuses it instead of misreading it). On the next run it replaces only the files that
    still match their record: what the source changed, added, or dropped, and what a person deleted. It never overwrites
    anything else, and it refuses, naming the path, in these cases: a hand-edited file of a skill it installed; a skill
    directory it did not install, including every install made before it kept records (a foreign directory); a file the
    source now wants to write where an unrecorded file already is; and a destination that leaves the project or makes
    .claude/skills and .agents/skills the same directory. A file you keep beside an installed skill is left alone. Every
    refusal is reported and then nothing is written, for any skill: all or nothing per invocation. A second install with
    no source change writes nothing and prints "unchanged: <id>" (otherwise "installed: <id>" or "updated: <id>").
    Nothing is adopted silently: a foreign directory stays foreign until `skills adopt` records it, and adopt does so only
    when the directory is exactly the current source (same files, same bytes, nothing else), naming each file that
    differs when it is not. adopt only records; it writes no skill file, and one runtime directory is enough (install
    adds the other). The approval record and a writer's temporary file are never copied, by install or by the Pi package
    build (engine/pipkg), which share one rule for it.
    Locking: add, remove, sync-manifest, and approve hold an exclusive lock, and validate and install a shared one, on
    .skills.registry.yaml.lock beside the registry (created by the first write, never removed, git-ignored). Shared holders
    never create it, so validate and install work on a read-only overlay; validate, which reads files that must agree,
    reads again under a real lock when the first writer ever creates the lock file while it reads. A writer refuses, with
    nothing locked, when the registry it would lock does not exist, so a raw call made from another directory leaves no
    lock file behind. A lock that stays taken for 2 seconds means another skills command is in progress: the verb changes
    nothing and reports it with exit 2 (retry). Exit 1 is unchanged: the verb ran or was refused, or the lock could not be
    created at all, which retrying will not fix.
    Project lock: project-register, project-revise, project-retire, install and adopt also take an exclusive lock on the
    project root directory itself (--project-root, or the working directory for install and adopt, read once before any
    lock), so no file is created in the project and nothing needs git-ignoring; a busy one is exit 2 like the overlay
    lock. A filesystem that cannot lock a directory refuses the verb (exit 1, nothing written); the lock is never skipped
    or replaced by a file in the project, and install and adopt refuse, with nothing locked, when the working directory
    cannot be resolved. A verb that holds both takes the overlay lock first, then the project lock, and releases them in
    reverse, so install and adopt can wait up to twice the lock bound (2 seconds each) when both are busy. project-status
    takes a shared lock on the project, so it never reports a project between a writer's renames.

overlay shaper <verb>
    Forward Shaper handoff verbs to the engine unchanged. Exit codes: 0 ready, 3 draft, 2 invalid, 1 error.
    assess           --root <worktree> --handoff <path> --goal <path> [--view]     read-only readiness assessment; --view prints the exact presented view
    clearance record --root <worktree> --handoff <path> --goal <path> --stdin      store a human clearance decision read only from stdin (normally driven by
                                                                                   the Pi /shaper-clear dialog; deny guards block model-run invocations)
    A ready result is not a signature: any process running as the same OS user, including any installed
    Pi extension, can forge a clearance record. Readiness grants no execution authority.
    Handoff versions 2 and 3 can reach ready: each acceptance item is {"criterion": ..., "verification":
    {"check": ...} or {"adjudication": ...}}, and the presented view shows every criterion with its planned
    check or adjudication. Checks are planned verification only; nothing runs them, and their results are
    downstream fulfillment evidence, not a readiness prerequisite. Handoff version 1 stays draft
    (acceptance_verification_unrepresentable).
    Version 3 adds the full bounded plan: roles [{role, responsibility}], tests, risks [{risk, mitigation}],
    estimates [{stage, low_minutes, high_minutes}] (agent effort to execute each stage), memory_scope, and
    delivery_limit. Parsing rejects contradictions deterministically: duplicate roles, and estimates that are
    missing, duplicated, name an unknown stage, are not positive, or have low_minutes above high_minutes;
    such a handoff is invalid and never ready. Any item byte-identical to a Goal non_goal is refused. The presented view shows
    memory_scope and delivery_limit beside the Goal's memory_scope and delivery_boundary for human judgment.
    A version 2 ready output states that those plan fields are absent; a version 3 one states they are present.

overlay roles <verb>
    Forward reusable-role handoff verbs to the engine unchanged. Roles are data only: a closed
    vocabulary (prototyper, shaper, estimator, builder, sweeper, polisher, reviewer, delivery,
    with sweeper and polisher optional) and a typed RoleHandoff v1 record chained by SHA-256 to
    the previous record. No verb launches an agent, dispatches work, or grants execution
    authority.
    validate --file <path>                                                       read-only: strictly parse and validate one RoleHandoff record
    next    --project <id> --goal <id> --chain <id>                              read-only: allowed next roles from the chain's current position
    resume  --project <id> --goal <id> --chain <id>                              read-only: current role, whether it is interrupted, and why
    append  --project <id> --goal <id> --chain <id> --stdin                      append one record read only from stdin; refused on identity mismatch,
                                                                                  a broken hash chain, a seq gap, or an invalid role transition
    match-shaper --root <worktree> --handoff <path>                              report which of a Shaper handoff v3's free-form roles equal a
                                                                                  member of the reusable-role vocabulary, without changing v3 semantics

overlay memory <verb>
    Forward memory-directive verbs to the engine unchanged. Read-only: it only plans what memory
    a caller may read (scope, sources, and identifying filters) and never queries or writes
    memory itself; executing a plan is a runtime adapter's responsibility outside this overlay
    (Phase 7).
    plan --profile <name> [--goal <path>] [--goal-directive <path>] [--handoff-directive <path>]
        Resolve <name>'s default MemoryDirective (derived from that workflow profile's
        memory_policy; it is the ceiling of what that profile can read, so every source is
        granted by at least one profile), optionally narrow it with a directive read from
        --goal-directive and
        then --handoff-directive (each narrower may only shrink scope and reuse a subset of the
        running sources; any widening is refused with a named reason), and print the resulting
        query plan as JSON. --goal, when given, is parsed with the Goal v2 schema and supplies
        the plan's project_id (and goal_id, for scope goal) filters. Exit 0 on a resolved plan,
        2 on a refused or invalid input, 1 on a usage error or a failed write of the plan.

overlay workflow <verb>
    Forward standalone workflow lifecycle verbs (Phase 6) to the engine unchanged: create,
    start, pause, resume, stage, verify, close, and status, plus the session-binding verbs bind,
    unbind, and binding (Phase 7, described after status). A workflow is its own append-only,
    hash-chained event log stored outside the repository at
    $XDG_STATE_HOME/labdrian/workflows/<project_id>/<workflow_id>.jsonl (or
    $HOME/.local/state/... when XDG_STATE_HOME is unset), keyed by project_id so every git
    worktree of a project observes the same workflow. Lifecycle operations never need Gentle AI,
    gentle-pi, a runtime, memory, or auth to be present: a declared dependency that is
    unavailable is recorded as an "unavailable" observation on the event, never hidden; a
    dependency is recorded "available" only when its file or binary is seen by stat (the Engram
    database, the longterm-mem registration record, gentle-ai on PATH), never opened, run, or
    trusted, and the detail says what presence does not prove. No verb here executes a workflow step or check; that is a runtime adapter's
    responsibility outside this overlay (Phase 7).
    create --project <id> --workflow <id> --goal <path> --profile <name> [--role-chain <id>]
        Create a workflow: binds the Goal's SHA-256 digest, the named Workflow Profile, and,
        when --role-chain is given, that role chain's current head digest (the chain may grow
        afterward, but verify requires this exact record to still be present).
    start | pause | resume --project <id> --workflow <id>
        Advance the workflow's status (created -> running -> paused -> running -> ...).
    stage --project <id> --workflow <id> --stage <name>
        Record the Workflow Profile's next declared stage; rejected if <name> is not exactly
        that next stage.
    verify --project <id> --workflow <id> --goal <path>
        Structural-only re-verification (no execution): the event hash chain, the Workflow
        Profile, the recorded stage order, the Goal's digest (re-read from --goal, never from a
        path recorded at create), and, if referenced, the role chain. --goal is required on
        every verify call; the CLI never persists a Goal file path.
    close --project <id> --workflow <id> --outcome completed|abandoned [--reason <text>]
        Close the workflow. completed requires an immediately preceding successful verify;
        abandoned is legal from any non-closed state (even if the recorded profile no longer
        resolves) and requires --reason.
    status --project <id> --workflow <id>
        Read-only: the on-disk classification (absent, owned, foreign, malformed, drifted, or
        unavailable) and, when owned, the replayed state. Never appends anything.
    bind --project <id> --workflow <id>
        Record that the git repository containing the working directory follows this workflow.
        The binding is a pointer stored outside the repository, at
        $XDG_STATE_HOME/labdrian/bindings/<repo-key>.json (or $HOME/.local/state/... when
        XDG_STATE_HOME is unset). <repo-key> is the SHA-256 of the repository's git common
        directory, so every worktree of a repository, and every symlinked spelling of its path,
        shares one binding. The repository is found by walking up from the working directory
        for a .git entry, by hand, without running git; outside a repository bind is refused.
        The workflow must exist, be owned, and not be closed. Binding the workflow the
        repository is already bound to changes nothing. A binding to a workflow that is still
        active (created, running, or paused), or whose log cannot be read right now and so may
        still be active, is never replaced silently: bind is refused, names the bound workflow,
        and asks for unbind first. A binding to a workflow that is closed, gone, corrupt, or not
        ours (drifted, malformed, or foreign) is stale, and bind replaces it, but only if the
        binding is still exactly the one it judged stale: if another process changed it in
        between, bind exits 2, leaves it alone, and asks you to run 'workflow binding' and retry. A binding
        file that is not ours (foreign or malformed) is never overwritten. Prints the
        classification and the binding as JSON. A binding records the association only; it does
        not change the workflow.
        Bind and unbind take turns per repository: one that cannot get the repository's lock
        within 2 seconds is refused (exit 2) and can be retried. bind prints the binding only if
        it is the one requested; if another process changed it meanwhile, bind exits 2 and says so.
    unbind
        Remove the binding of the repository containing the working directory and print
        {"removed": true} or {"removed": false}. Idempotent: unbinding a repository that is not
        bound succeeds. A foreign or malformed binding file is refused and left untouched.
    binding
        Read-only: print the binding's classification (absent, owned, foreign, malformed, or
        unavailable) and, when it is owned, the binding plus the bound workflow's classification
        and status (absent when its log is gone). A workflow store that cannot be read is
        reported in that section as data, not as an error.
    The lifecycle verbs print the resulting classification and state (or, for status, the
    current one) as JSON on stdout and report errors on stderr. Exit 0 on success, 2 on a
    refused or invalid operation (an illegal transition, a failed verify, non-owned on-disk
    state; for the binding verbs, no git repository, a workflow that cannot be bound, a
    binding file that is not ours or cannot be used (foreign, malformed, unavailable), another
    bind or unbind in progress for the repository (busy), or a binding another process changed
    meanwhile), 1 on a usage error (including an unknown flag). Provenance
    (worktree root, git HEAD) is observed by walking the .git directory by hand; it never runs
    the git binary or any other subprocess, and any part it cannot read is left empty rather
    than failing the operation.

gentle-ai-overlay runtime capabilities [--target claude|codex|pi|opencode|all]
    Engine verb (Phase 7): there is no `overlay runtime` wrapper, so run it on the installed engine
    binary, ~/.claude/bin/gentle-ai-overlay. Read-only and declarative: it prints, as JSON, what
    each runtime adapter declares it supports (default --target all; a single target prints one
    declaration). A declaration has one claim per capability -- installation, projection, dispatch,
    cancellation, persistence, restart, authentication, memory-enforcement -- each supported,
    partial, or unsupported. A supported or partial claim names the tests that prove it as
    <directory under engine/>:<TestName>, and a guard test verifies every named test exists; a
    partial or unsupported claim states its limit in "detail". "untested" appears only on a runtime
    that cannot be exercised on this machine (opencode). The output states what the engine's own
    tests prove, never what a runtime happened to do: a capability no test proves is declared
    unsupported, with the limit written. It reads no configuration, HOME, or file and starts no
    session. Exit 0 on success, 2 on an unknown --target value, 1 on a usage error (including an unknown flag).

gentle-ai-overlay runtime probe [--target claude|codex|pi|all]
    Engine verb (Phase 7), run on the installed engine binary like runtime capabilities. Read-only and
    stat only: it prints, as JSON ({"version":1,"observations":[...]}), one observation per signal, from the
    real HOME and PATH: whether each selected runtime's credentials file is present (credentials:claude-code,
    credentials:codex, credentials:pi; default --target all) and, on every run, whether the Engram database
    (memory:engram), the longterm-mem registration record (memory:longterm-mem), and the gentle-ai binary on
    PATH (gentle-ai-review) are present; memory:procedural-skills has no check and is always unavailable. It
    never opens, reads, hashes, or prints the contents of any of those files, never runs gentle-ai, and never
    names a path in its output, so "available" says a file or binary is present and nothing else: a present
    credentials file is not proof that the runtime is authenticated, a present database is not a healthy one, and
    a binary on PATH has not been run (each detail says so, and a test parses the prober's source to forbid opening
    files). The workflow verbs record the same observations on their events. The longterm-mem record is looked for
    at the default state directory only (~/.labdrian-overlay), not under a custom --state-dir. Exit 0 on success,
    2 on an unknown --target value, 1 on a usage error (including an unknown flag).

gentle-ai-overlay projection hook --event UserPromptSubmit|PreToolUse
    Internal hook command (Phase 7): Claude Code is meant to run it before each prompt (UserPromptSubmit) and before
    each tool call (PreToolUse); a person does
    not. There is no `overlay projection` wrapper, so it runs on the installed engine binary,
    ~/.claude/bin/gentle-ai-overlay. `install-hooks` installs it into Claude Code settings.json
    (one UserPromptSubmit entry and two PreToolUse entries, the file-edit tools and the longterm-mem query tool);
    on an existing install, re-run `install-hooks` and restart Claude Code to load it, because Claude Code reads
    hooks only at start and `status-hooks` reports degraded until the entries are in place. Its tests feed it hook JSON.
    UserPromptSubmit and PreToolUse are the events supported.
    UserPromptSubmit: put the workflow the repository is bound to (see `workflow bind`) into the session's
    context, so a new or restarted session is told the same thing as the last one. PreToolUse: gate a tool
    call against that workflow (below).
    Input: the hook JSON on stdin, at most 1 MiB. Only hook_event_name and cwd are read (cwd only when
    absolute, else the process's working directory is used). The session id and the prompt are
    ignored, so the output depends on neither.
    Output: nothing, or exactly one JSON object,
    {"hookSpecificOutput":{"hookEventName":"UserPromptSubmit","additionalContext":"..."},"systemMessage":"..."},
    with either part left out when it is empty. additionalContext (at most 16 KiB, the same bytes for
    the same state, no timestamps) states the workflow id and project, the profile, the status, the
    goal id and a short digest, the recorded, current, and next stage, the memory plan (a read-only
    plan: scope, sources, project_id, goal_id, write=none), the dependencies the last event recorded
    as unavailable, and the Claude Code capabilities the engine declares partial or unsupported. A
    paused workflow is announced as paused, to be advanced only after `workflow resume`. systemMessage is
    one warning line for the user.
    Silent (no output on either stream) when the repository has no binding, the working directory is
    not in a repository, or the input is not a usable hook input. A binding, or a bound workflow, that
    cannot be followed (foreign, malformed, drifted, unavailable, or gone) gives one warning and
    projects nothing; so does a binding store that cannot be opened or read (a real error, not an absent
    binding) and so does an internal error the hook recovered from (the warning is one short sanitized
    line; the full text goes to stderr). A closed workflow is announced in one line and its binding is
    removed, best effort and only if it is still the binding the hook read; the note says whether the
    binding was removed, left alone (another process had changed it), or the removal failed, in which
    case it names `labdrian workflow unbind` and repeats on every prompt until the binding is gone.
    Read-only otherwise: UserPromptSubmit never appends to a workflow log or rewrites a binding (its one
    write is removing the binding of a closed workflow, above), starts no process, and makes no network
    call. Exit 0 always, so it can never block a prompt (it never exits 2), except 1 on a command line
    it does not understand.

    PreToolUse (the gate): reads the same hook JSON (hook_event_name, cwd, tool_name, and tool_input;
    everything else is ignored) and is strictly read-only: it never writes or unbinds, not even for a
    closed workflow (UserPromptSubmit does that). It decides from the binding and the bound workflow:
      - Paused workflow: Write, Edit, MultiEdit, and NotebookEdit are denied, with a reason that names the
        workflow and project and the two ways out, `labdrian workflow resume --project <p> --workflow <w>`
        or `labdrian workflow unbind`. It never denies Bash (a shell command cannot be classified
        reliably), a read, or any other tool.
      - Memory gate, for a created, running, or paused workflow: a call to the longterm-mem `query` tool
        (the MCP name mcp__longterm-mem__query, or a plugin-prefixed mcp__<plugin>_longterm-mem__query) is
        denied when its `project` argument is missing, not a string, empty, or different from the project
        of the workflow's memory plan (the profile ceiling the context states; the reason names both
        projects and says to query the plan's project or unbind). A plan with no project (scope none, the
        standalone-minimal profile) permits no query at all. It never denies `get`, `promote`, Engram
        tools, or any write, and it does not check the plan's sources: a plan that omits longterm-mem does
        not deny a query to its own project.
      - Everything else is allowed: no binding, a binding or workflow it cannot follow (foreign, malformed,
        drifted, unavailable, gone), a closed workflow, an unrecognized status, a plan that cannot be
        computed, or input it cannot use. It never blocks on an unknown state.
    Output: a denial is exactly one JSON object,
    {"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"..."}},
    with exit 0 (the JSON decides; exit 2 is never used). An allow prints nothing, or only a
    {"systemMessage":"..."} warning when the gate had something to check and could not (a memory plan it
    cannot compute, a query whose input is not an object, a recovered internal error, which also goes to
    stderr in full); it never prints permissionDecision "allow", which would bypass Claude Code's normal
    permission flow. It stays silent for a binding or workflow it cannot follow, because it runs on every
    tool call and UserPromptSubmit already warns once per prompt. An in-flight tool call cannot be
    interrupted: the gate acts at the next tool call. Like the projection, it does nothing until you re-run
    `install-hooks` and restart Claude Code.

gentle-ai-overlay skills guard-hook
    Internal hook command: Claude Code is meant to run it before each Bash and file-edit tool call; a person does
    not. It runs on the installed engine binary, ~/.claude/bin/gentle-ai-overlay. `install-hooks` installs it into Claude
    Code settings.json (two PreToolUse entries, one for Bash and one for Write, Edit, MultiEdit, and NotebookEdit);
    on an existing install, re-run `install-hooks` and restart Claude Code to load it, because Claude Code reads
    hooks only at start and `status-hooks` reports degraded until the entries are in place.
    It denies the agent running `skills approve` (approval records that a human reviewed the exact SKILL.md bytes)
    and writing a skill's `.approval.json` record with a file-edit tool. The reason names the verb, says approval is a
    human step, and gives the human the command to run in their own terminal:
    `labdrian skills approve --id <id> --approver <name>`. Reading the record, the other skills verbs, and file
    contents that merely mention the verb are never denied.
    It is a speed bump, not a security boundary. It matches text only: the entry point (labdrian, labdrian-overlay, or
    gentle-ai-overlay, with or without a path) followed by `skills approve` anywhere in a Bash command, including after
    `cd x &&` and inside `sh -c '...'`, or a file tool whose target file is named `.approval.json` (the last path component
    is compared, case-insensitively, in any directory; a longer name that merely ends in it is not matched). It is not a shell parser, so
    it can be bypassed (an alias under another name, a variable that holds the entry point, a script written to a file
    and run, an encoded command, or a shell redirection into the record), and it can deny a command that only spells
    the invocation, such as an `echo`, a commit message, or a heredoc line naming `labdrian skills approve`; `rg` and
    `grep` searches for it are not denied, unless the `rg` segment is handed a program to run with `--pre` or
    `--hostname-bin` (bare or as `--flag=value`; flags that only start with the same letters, such as `--pretty` and
    `--pre-glob`, do not count). Reword such a command. The record's real guarantee is unchanged: it matches
    the exact bytes of the skill beside it.
    Input: the hook JSON on stdin, at most 8 MiB (input over that is allowed, unjudged). Only tool_name and the command,
    file_path, and notebook_path fields of tool_input are read.
    Output: nothing, or exactly one JSON object,
    {"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"..."}},
    with exit 0 (the JSON decides; exit 2 is never used). An allow prints nothing and never permissionDecision "allow".
    It never blocks on an error: input it cannot read, a failing stdin or stdout, and a recovered internal error all
    allow the call (unlike the shaper clearance guard, which fails closed for its narrower markers, because this hook runs
    on every Bash and file-edit call). A recovered internal error also shows one short sanitized `systemMessage` naming
    the approve guard and saying the tool call was not checked and was not denied, so a guard that stopped guarding does
    not look like one that allowed; the full error goes to stderr. It is never a permission decision. With the engine binary missing the hook does nothing, and `status-hooks` reports it.
    Exit 0 always, except 1 on a command line it does not understand.

overlay --help
    Show this help.
```
