# labdrian-sdd-overlay

A vendor+overlay git framework that lets your customized gentle-ai skills survive `gentle-ai sync`/upgrade cycles — now with multi-target deploy for Claude Code, OpenCode, and Codex.

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

# 3. Install the global `labdrian` command (symlinks bin/overlay into ~/.local/bin).
bin/overlay install-alias

# 4. Make sure ~/.local/bin is on your PATH. If `labdrian` is not found, add this to
#    your ~/.bashrc or ~/.zshrc and reopen the shell:
#      export PATH="$HOME/.local/bin:$PATH"
```

**Prerequisite:** the TUI runs `go run .`, so you need **Go installed** (`brew install go` or https://go.dev/dl).

Now, from any directory:

```bash
labdrian tui     # see per-target drift / gentle-ai sync state, and apply updates
```

The TUI shows whether each target is in sync with gentle-ai and lets you re-capture and re-apply your overlay without leaving the dashboard. Prefer the CLI? Jump to [Normal update cycle](#normal-update-cycle-per-target).

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
```

Each target section ends with a `VERDICT` line and an `ACTION` recommendation:

```
VERDICT:claude:UPSTREAM_CHANGED=5 OVERLAY_NOT_DEPLOYED=0
ACTION:claude: gentle-ai sync detected: run 'overlay capture --target claude' then 'overlay apply'
```

## One-time bootstrap

> **Note:** This is only for *creating* the overlay from scratch (seeding `upstream`/`main` from a backup tarball). If you cloned the repo, the branches already exist — skip this and use [Quick start](#quick-start--clone-then-labdrian-tui-from-anywhere) instead.

```bash
# Clone or copy this repo anywhere, then cd into it:
cd labdrian-sdd-overlay   # the directory you cloned into
chmod +x bin/overlay
bin/overlay bootstrap
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

## CLI reference

```
overlay bootstrap
    One-time setup: init repo, seed upstream, create main

overlay capture [--target claude|opencode|codex] [--from-backup <tarball>]
    Refresh upstream from target (default: claude).
    --from-backup reads from a specific backup tarball instead of live files.

overlay apply [--target claude|opencode|codex|all]
    Merge upstream into main and deploy to target(s) (default: all).

overlay status [--target claude|opencode|codex|all]
    Show branch, diff stat upstream..main, and per-file drift per target (default: all).

overlay sync-check [--target claude|opencode|codex|all]
    Validate gentle-ai sync state: UPSTREAM_CHANGED and OVERLAY_NOT_DEPLOYED per target (default: all).

overlay tui
    Launch the Go/Bubbletea TUI front-end (target selection + gentle-ai sync dashboard).

overlay install-alias [name]
    Symlink a `labdrian` command into ~/.local/bin (run once per machine), then `labdrian tui`.

overlay --help
    Show this help.
```
