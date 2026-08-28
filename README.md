# labdrian-sdd-overlay

A customization layer over `gentle-ai` (an SDD-driven, multi-agent dev runtime) that lets your overlaid skills and rules **survive `gentle-ai` updates**, via a two-branch model: `upstream` (pristine vendor baseline) + `main` (your customizations on top).

**What it does:**

- Tracks vendor-managed and custom skills in a git overlay, so `gentle-ai sync`/`upgrade` never silently clobbers your changes.
- Deploys your overlaid skills to **three agent runtimes**: Claude Code (`~/.claude/skills`), opencode (`~/.config/opencode/skills`), and codex (`~/.codex/skills`).
- Deploys Claude Code **agent definitions** (e.g. GADU) to `~/.claude/agents/` — agents are Claude Code-only; opencode/codex receive the portable skill form instead.
- Adds a **deterministic minimalism-scoping layer**: a Go engine + Claude Code hooks (`UserPromptSubmit` → propagate, `PreToolUse`/`Agent` → gate-task) that inject a minimalism contract **only** into the code-writing SDD phases (`sdd-tasks`/`sdd-apply`) and exclude it from all others. Deterministic on Claude Code; documented platform limits apply on opencode/codex.
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

Use `--target <name>` on `apply`, `status`, `capture`, and `sync-check`. Default for `apply`/`status`/`sync-check` is `all` (all three targets). Default for `capture` is `claude`.

The `agent` route (see [Tracked files](#tracked-files-overlaymanifest)) additionally deploys to `~/.claude/agents` (claude target only). `--target opencode` or `--target codex` on an agent row is a no-op — zero applicable targets.

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
| `install-hooks` | **modifies** | Build the Go engine binary + wire `UserPromptSubmit`/`PreToolUse`/`Agent` hooks into `~/.claude/settings.json` (backs up to `.bak` first). Run once to activate scoping. |
| `uninstall-hooks` | **modifies** | Remove the two overlay hook entries from `~/.claude/settings.json`, leaving all other keys intact. |
| `status-hooks` | read-only | Check engine binary, hooks wired, contract readable — exits 0 if all healthy; missing binary exits non-zero with `run 'overlay install-hooks'` guidance. |
| `doctor [--fix]` | read-only | Host-toolchain preflight: go, gentle-ai, discovery tools (bat/rg/fd/sd/eza), engine binary, skill registry — plus a per-target version/digest consistency row (WARN only, never fails the exit code). `--fix` best-effort installs missing discovery tools via Homebrew. |
| `validate-entry-contract --schema PATH --instance PATH` | read-only | Validate a pre-SDD entry candidate against the version-matched schema and deterministic cross-field rules. |
| `install-alias [name]` | **modifies** | Symlink `labdrian` (or a custom name) into `~/.local/bin`. Run once per machine. |

### The 3 workflows

**1. Day-to-day — nothing to do.**
The hooks run in the background: `UserPromptSubmit` (propagate) keeps skill registries fresh across sessions; `PreToolUse`/`Agent` (gate-task) injects the minimalism contract into `sdd-tasks`/`sdd-apply` automatically.

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

The pre-SDD entry bundle is tracked as four inseparable assets: `inception-pipeline/SKILL.md`, `_shared/pre-sdd-contracts.md`, `_shared/entry-contract.schema.json`, and `_shared/actuals-record.schema.json`. Version `2.0.0` also uses the isolated validator in `tools/entry-contract-validator`; its dependency is intentionally not added to `engine/go.mod`.

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
three artifacts from that one canonical source:

- `agents/GADU.md` — Claude Code agent definition (deployed to `~/.claude/agents`)
- `opencode/agents/GADU.md` — Opencode-native agent definition
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
    Build the Go engine binary and wire the two deterministic-scoping hooks into
    ~/.claude/settings.json (UserPromptSubmit + PreToolUse/Agent). Backs up settings.json
    to settings.json.bak before modifying. Run once to activate; inert until then.

overlay uninstall-hooks
    Remove the two overlay hook entries from ~/.claude/settings.json, leaving all
    other keys and hooks intact.

overlay status-hooks
    Check overlay installation health: binary present, hooks wired, contract readable.
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
    - skills/gadu-operator/SKILL.md
    from engine/gadu/persona/body.md.

overlay validate-entry-contract --schema PATH --instance PATH
    Build and run the isolated v2 entry-contract validator from a temporary binary.
    Relative paths resolve from the caller's working directory. Exit codes 2-6 are
    preserved; no installed skill root is modified.

overlay skills <verb>
    Manage the skills registry (skills.registry.yaml) and overlay.manifest.
    list         [--registry <path>]                                               print sorted registry entries (id, source type, update strategy, targets)
    status       [--registry <path>]                                               print count summary (total / core / custom)
    validate     [--registry <path>] [--manifest <path>] --source-root <path>      cross-check registry vs manifest and skills/ on disk vs manifest; exit 1 on any divergence
    install      [--registry <path>] [--source-root <path>] [--project-id <id>]   copy project-scoped skills into <cwd>/.claude/skills/
    add          <id> [--registry <path>] [--manifest <path>] [--source-root <path>] [--repo <url>] [--ref <sha>]  register a skill (custom or external)
    remove       <id> [--registry <path>] [--manifest <path>]                      unregister a skill from registry and manifest
    sync-manifest [--registry <path>] [--manifest <path>]                          regenerate */SKILL.md rows from registry; preserves all non-skill lines

overlay --help
    Show this help.
```
