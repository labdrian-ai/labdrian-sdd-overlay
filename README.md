# labdrian-sdd-overlay

A customization layer over `gentle-ai` (an SDD-driven, multi-agent dev runtime) that lets your overlaid skills and rules **survive `gentle-ai` updates**, via a two-branch model: `upstream` (pristine vendor baseline) + `main` (your customizations on top).

**What it does:**

- Tracks vendor-managed and custom skills in a git overlay, so `gentle-ai sync`/`upgrade` never silently clobbers your changes.
- Deploys your overlaid skills to **three agent runtimes**: Claude Code (`~/.claude/skills`), opencode (`~/.config/opencode/skills`), and codex (`~/.codex/skills`).
- Deploys **agent definitions**: the [role agents](#role-agents--one-source-three-runtimes) of Claude Code to `~/.claude/agents/`, those of Codex to `~/.codex/agents/` as TOML, and those of Pi inside the built package (see [Pi](#pi-via-gentle-pi)); opencode receives none.
- Adds a **deterministic minimalism-scoping layer**: a Go engine + Claude Code hooks (`UserPromptSubmit` → propagate, `PreToolUse`/`Agent` → gate-task) that inject two managed contracts (minimalism, anti-generic design) **only** into the agents that write code (minimalism into `shaper` and `builder`, anti-generic design into `builder`) and exclude them from the review agents. Deterministic on Claude Code; documented platform limits apply on opencode/codex.
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

The `agent` route (see [Tracked files](#tracked-files-overlaymanifest)) additionally deploys to `~/.claude/agents` (claude target only), and the `codex-agent` route deploys `codex/agents/<role>.toml` to `~/.codex/agents` (codex target only). `--target opencode` or `--target codex` on an `agent` row, and any other target on a `codex-agent` row, is a no-op — zero applicable targets.

### Pi (via gentle-pi)

Pi is a **package target**, not a per-file copy target: `--target pi` on `apply`/`status`/`sync-check` builds a `labdrian-pi` package (`package.json`, `skills/`, `agents/`, `extensions/`, `mcp.json`) into `$STATE_DIR/pi/labdrian-pi` from the same `skills.registry.yaml` every other target reads and the Pi agent files of `pi/agents/`, instead of copying files into a runtime-owned directory.

- `apply --target pi` builds the package, then — when a `pi` CLI is on `PATH` — runs `pi install <path>` so the change is picked up on the next Pi session. Without `pi` on `PATH` it prints the install hint instead.
- `status --target pi` reports honest per-entry proof: built, in sync with the current manifest, and listed in `~/.pi/agent/settings.json`'s `packages` array (read-only — this overlay never writes that file itself). It always discloses that `pi --no-extensions` bypasses the contract-gate extension and `pi --no-skills` bypasses skill discovery for that session, since neither flag's use can be detected at runtime (short aliases `-ne` and `-ns`).
- Uninstalling runs `pi remove <path>` (not `pi uninstall`, which does not exist) and removes the built package directory — it never edits `~/.pi/agent/settings.json` or `~/.pi/agent/mcp.json` directly, and never touches any gentle-pi- or pi-engram-owned entry.
- A `before_agent_start` package extension injects the same bare contract-path line Claude/OpenCode/Codex receive into the `shaper`/`builder` system prompt, composing with (not overwriting) gentle-pi's own handler output.
- `longterm-mem register --target pi` writes its MCP entry into the package's own `mcp.json`; `--target all` only attempts Pi when the package is both built and genuinely installed.
- **The role agents as real Pi subagents.** `apply --target pi`, right after `pi install`, ensures the role agents have a working `subagent_*` dispatch runner: when `~/.pi/agent/settings.json` already lists `npm:gentle-pi` at version 2.6.0 or later, its **native** `subagent_*` tools are used and the third-party [Pi Subagents extension](https://www.npmjs.com/package/pi-subagents-j0k3r) is never installed — installing it while native support is present leaves gentle-pi's own tools unregistered, so if that obsolete extension is already present the output discloses the conflict and names the exact removal command (`pi remove npm:pi-subagents-j0k3r`) without removing it (it is not overlay-owned). Only when gentle-pi's native subagents are unavailable (older gentle-pi, or none installed) does it fall back to installing `npm:pi-subagents-j0k3r` via a fixed `pi install` argv — unless `pi-subagents` (the alternate package name) is already installed, or `LABDRIAN_PI_SKIP_SUBAGENTS=1` opts out. Either way it then links every agent of the package — the [role agents](#role-agents--one-source-three-runtimes) `agents/<role>.md` — at `~/.pi/agent/agents/<name>.md`, so each is dispatchable through whichever runner is active instead of only being a relayed persona. Each link is overlay-owned: ownership is proven by `readlink` equality with the stable package path (only the inode changes across a `swap` rebuild), a pre-existing foreign file or symlink is left untouched and reported as a conflict (the other agents are still linked), and `status --target pi` reports the subagent runner's state (native/legacy/conflict/absent) and the state of each link (missing/current/stale/conflict) as separate, honest entries. The package build, the first step of `apply --target pi` (and of `pipkg build`), also removes the overlay-owned link of an agent the package no longer ships (the `GADU.md` link that every deploy before the role agents' PR 5 made), by the same `readlink` proof, whether or not a `pi` is installed and whether or not `pi install` succeeds, and says so in its output (`GADU.md link removed (the package no longer ships it).`); a file or link that is not the overlay's is never removed or rewritten, and `status --target pi` names such a link while it is still there. `runtime uninstall --target pi` removes only those symlinks — never the extension package, never gentle-pi's own package, and never any gentle-pi- or pi-engram-owned file.
- **Pi-specific agent files.** The package's `agents/` holds every `pi/agents/*.md` of the overlay (the role agents), each copied as it is: never the Claude file of the same name in the overlay's `agents/`, and an overlay with no `pi/agents` builds a package with no agent. Each declares a `model: pi-claude-cli/<model>` ref and carries a compact body. Both choices trace to findings verified live 2026-09-13 against Pi 0.85.1 / gentle-pi 2.6.0 / `@saccolabs/pi-claude-cli` 0.8.1: gentle-pi's native `subagent_run` only completes a child whose agent file names a `pi-claude-cli/<model>` id (an `anthropic/*` or `openai-codex/*` model, or an absent `model:`, ends the child with an error), and the `pi-claude-cli` bridge — which relays the agent's system prompt to a fresh Claude Code process via `--append-system-prompt-file` — hangs indefinitely on a ~7 KB prompt while a compact one completes normally.

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

**`route` column (optional third column):** each manifest row may end with `skill` or `agent` (default: `skill` when omitted). Bare two-column rows are unchanged. The `agent` route tells the installer to source from `agents/<path>` and deploy to `~/.claude/agents` instead of any skills directory; the `codex-agent` route sources from the path itself (`codex/agents/<role>.toml`) and deploys to `~/.codex/agents`. Non-deployable rows (engine source files, root-level files) are recognized by the installer and skipped silently. The route of every row is decided by the engine, not by the script: `apply`, `status`, `sync-check`, `capture`, `restore` and `bootstrap` ask `gentle-ai-overlay manifest routes` (see the CLI reference) for it, and the script builds the engine binary on first use when it is missing or older than its source (Go is already a prerequisite). A manifest with a `longterm-mem/` row that has no route, or one outside `skill`, `agent`, `opencode-agent`, `codex-agent` and `mcp`, is refused whole before any row is copied, said or restored.

## Usage

`labdrian` is the alias to `bin/labdrian-overlay`, installed via `labdrian-overlay install-alias`. All commands below work as `labdrian <command>` or `bin/labdrian-overlay <command>` from the repo root. (The pristine `bin/overlay` is a vendored copy of gentle-ai upstream — labdrian never edits it, so it never conflicts on sync.)

`bin/labdrian-overlay` is an entry point and the code is in the modules of `bin/lib/`, one concern each (`targets.sh`, `manifest_routes.sh`, `deploy_state.sh`, `apply.sh`, `longterm_mem.sh`, `doctor.sh`, ...). The entry point finds its own real location (a symlink, such as the one `install-alias` makes, or another directory changes nothing), sources the modules in the order of its `_modules` list, which is an order of dependency, and dispatches the command. The script is the entry point and `bin/lib/` together: copy or package them together, and a clone that is missing a module stops with an error that names it. They ship as git ships the rest of the clone, so `self-update` and `apply` bring them.

### Launch

```bash
labdrian tui          # recommended: full TUI dashboard
```

The TUI wraps the CLI — everything below is also available as individual commands. The
dashboard surfaces per-target drift for **agent files** (e.g. the role agents in
`~/.claude/agents`) as a dedicated Agents sub-section alongside skills. Read-only skills
registry actions (`skills validate`, `list`, `status`) are available directly from the
action menu without leaving the dashboard. `skills validate` also cross-checks the
`skills/` directory itself against `overlay.manifest` and exits non-zero on any
divergence. The dashboard also shows each target's recorded release version and
digest-match status, and offers a confirmed "Restaurar respaldo" (restore) action —
gated on a backup actually existing for the selected target(s) — routed through the
same confirm→run→result pattern as apply/self-update.

The target list on the first screen is not kept by the TUI: it is read from
`labdrian targets` at launch, so it is always what `--target all` acts on (today that
includes `pi`). The TUI sends `--target all` only when you leave every listed target
selected, and re-checks the list with the backend right before it does; with any target
unticked it runs the action once per ticked target instead. Capture and restore are
per-file operations that the backend refuses for a package target such as `pi`, so they
run on the selected `copy` targets only. If the list cannot be read the screen says so and
offers no target action. Likewise the backup that "Restaurar respaldo" offers is read with
`restore --target <t> --list`, so it follows the backend's `STATE_DIR`; a target whose
backups the backend cannot confirm is not offered a restore.

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
| `targets` | read-only | Print the target catalog, one target per line as `<name><TAB><kind>` (`copy` or `package`), in the order `--target all` expands to. The TUI reads it instead of keeping its own list. |
| `version` (also: `--version`) | read-only | Print this clone's current release version and each target's recorded deployed version. |
| `install-hooks` | **modifies** | Build the Go engine binary + wire `UserPromptSubmit`/`PreToolUse`/`Agent` hooks, including the workflow projection family and the skills approve guard, into `~/.claude/settings.json` (backs up to `.bak` first). Run once to activate scoping; re-run it after an upgrade that adds a hook family, then restart Claude Code to load the hooks. If `$CODEX_HOME` (default `~/.codex`) already exists, also merge three Codex memory hooks (`SessionStart`, `PreToolUse`, `PostToolUse`) into `hooks.json`, with a `.bak` backup. Trust each new handler once in the Codex TUI (`/hooks`, key `t`); upgrading from SessionStart alone needs two more trusts, without changing the existing SessionStart trust. Never edits trust state or `config.toml`. |
| `uninstall-hooks` | **modifies** | Remove the overlay hook entries (the minimalism and design pairs, the SessionEnd sync-trigger, the review-receipt and shaper guard entries, the workflow projection hooks, and the approve guard entries) from `~/.claude/settings.json`, including entries left by contracts retired in earlier versions, and remove our three Codex memory entries from `$CODEX_HOME/hooks.json` (default `~/.codex`). Foreign entries and keys are preserved. |
| `status-hooks` | read-only | Check engine binary, hooks wired, contracts readable; when Codex's home exists, check all three memory entries and name missing or drifted events with the `install-hooks` remedy. Present never means trusted (`/hooks`, key `t`). No Codex home means no extra line. Exit 0 healthy, 1 failed, 2 warnings alone. |
| `doctor [--fix]` | read-only | Host-toolchain preflight: go, gentle-ai, discovery tools (bat/rg/fd/sd/eza), engine binary, the skill registry of the project (judged by the engine, with the check `status` makes: absent is fine, empty or unreadable fails, one without a scoped block warns) — plus a per-target version/digest consistency row (WARN only, never fails the exit code). `--fix` best-effort installs missing discovery tools via Homebrew. |
| `validate-entry-contract --schema PATH --instance PATH` | read-only | Validate a pre-SDD entry candidate against the version-matched schema and deterministic cross-field rules. |
| `install-alias [name]` | **modifies** | Symlink `labdrian` (or a custom name) into `~/.local/bin`. Run once per machine. |

### The 3 workflows

**1. Day-to-day — nothing to do.**
The hooks run in the background: `UserPromptSubmit` (propagate) keeps skill registries fresh across sessions; `PreToolUse`/`Agent` (gate-task) injects the minimalism and anti-generic-design contracts into the agents their frontmatter names (`shaper`, `builder`) automatically, and, while a workflow is bound to the repository, appends the memory instructions of that workflow to every sub-agent's prompt through the minimalism entry alone (C23 M6), the block `SessionStart` puts into the session itself. Review candidate coverage and skill discovery are handled by gentle-ai itself since v2.7.0; the overlay no longer ships contracts for them.

**2. gentle-ai released an update.**
```bash
labdrian sync-check                         # UPSTREAM_CHANGED detected
labdrian capture --target claude            # pull new vendor files into upstream
labdrian apply                              # re-merge your customizations + redeploy
labdrian skills validate                    # exit 1 lists the global skills the merge changed: approve each one (see "Normal update cycle")
labdrian sync-check                         # confirm: healthy
```

**3. You edited a skill.**
```bash
# edit skills/<path>.md in your editor
labdrian apply                              # redeploy to all targets
labdrian sync-check                         # confirm in-sync
```

The pre-SDD bundle is tracked as four assets: `inception-pipeline/SKILL.md`, `_shared/pre-sdd-contracts.md`, `_shared/entry-contract.schema.json`, and `_shared/actuals-record.schema.json`. The skill left the bundle's version: `inception-pipeline` `3.0.0` enters the labdrian role chain (`skills/role-chain/SKILL.md`) and no longer writes the entry contract, so version `2.1.0` now covers the contract assets only, and the isolated validator in `tools/entry-contract-validator` (its dependency is intentionally not added to `engine/go.mod`) checks the archived contracts. The version is a compatibility set, not an exact-match lock: `2.1.0` still validates contracts written by the `2.0.0` bundle.

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

# 3. Check the approvals of the global skills the merge may have changed
overlay skills validate

# 4. Check drift at any time
overlay status
overlay status --target opencode
```

If there are merge conflicts, `overlay apply` exits 1 and tells you exactly which files to resolve.

**After the merge, review the skills it changed (step 3).** A global skill's approval record binds the exact bytes of its `SKILL.md`, so a merge that changes a skill (core skills included, baseline ones too) makes `skills validate` exit 1 and report `APPROVAL_MISSING` (a baseline skill whose bytes changed) or `APPROVAL_STALE` (a skill with a record). For every global skill it reports, review the change (for example `git diff` of `skills/<id>/SKILL.md`), and only then run, in a terminal:

```bash
labdrian skills approve --id <id> --approver <name>
```

An agent cannot do this for you: the approve guard denies it (a speed bump, not a security boundary). A baseline skill whose only hard-lint findings are its size or its description (`body-hard-budget`, `description-max`, `description-one-line`) is approved with `warning:` lines on stderr instead of being refused; any other hard finding, such as a missing front matter, refuses it like any skill. Run `skills validate` again until it exits 0. Until you do, the merged skills are deployed but not approved, and `skills validate` keeps exiting 1.

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

## Role agents — one source, three runtimes

Each of the eight roles (builder, shaper, polisher, prototyper, sweeper, reviewer, verifier, delivery) has one source, `roles/<role>/body.md`: the core prefix every role starts with and a delta of a few lines. `engine/agentgen` holds one table of what differs between the runtimes (the tools, the model, the thinking level and the description of each role) and writes the file of each runtime from the source and the table. `overlay gadu-generate [--check]` runs it, forwarding to the engine with `OVERLAY_DIR` set to the repository root (the verb keeps the name it had when the same run also wrote the GADU operator, which these agents replaced). It writes:

- `agents/<role>.md` — Claude Code (deployed to `~/.claude/agents`): `name`, `description`, `model`, `tools`.
- `pi/agents/<role>.md` — Pi (copied into the built package): a `pi-claude-cli/<model>` ref, `thinking`, and the Pi names of the same tools (`read`, `grep`, `find`, `bash`, `edit`, `write`, `codegraph` and the Engram tools by their bare names).
- `codex/agents/<role>.toml` — Codex: `name`, `description`, `developer_instructions` and `model_reasoning_effort`; no `model` (the parent passes it when it spawns the agent) and no tool list.

`apply` deploys them: Claude's through the `agent` rows as before, Codex's through `codex-agent` rows of `overlay.manifest` to `~/.codex/agents` (tagged `custom`, like every agent file this overlay writes itself, so a `capture` never reads them back from a deployed copy), and Pi's through the package build, which copies every `pi/agents/*.md` into the package and links each one at `~/.pi/agent/agents` (see [Pi](#pi-via-gentle-pi)).

A role with no Engram tool in its Claude list (today the reviewer) loses, in all three files, the one line of the core prefix that sends a role to Engram topics. The role file never carries a generated header: its edit surface is the source, and `gadu-generate --check` fails (locally and in the engine job of CI) when a committed file differs from what the generator writes, or a role source is missing or cannot be projected. Edit `roles/<role>/body.md` or the table in `engine/agentgen/roles.go`, then regenerate.

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

overlay targets
    Read-only: print the target catalog, one target per line as <name><TAB><kind>
    (kind: copy or package), in the order --target all expands to. Takes no
    arguments. Front-ends read this list instead of keeping their own, so they
    never act on a target they did not show.

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
    If CODEX_HOME (default ~/.codex) already exists, also merge three memory hooks into
    hooks.json by ownership: SessionStart for instructions, PreToolUse and PostToolUse for
    memory reads (the tool commands pass --source codex). Foreign values keep their bytes;
    edited array layout may change. Writes atomically with a .bak backup; an identical merge
    writes nothing. Trust each new handler once in the Codex TUI (/hooks, key t). Upgrading
    from SessionStart alone needs two more trusts; its existing command bytes and trust stay
    unchanged. No trust state or config.toml is changed. Never creates a Codex home.

overlay uninstall-hooks
    Remove every overlay hook entry (each family install-hooks wires, projection and approve
    guard included) from ~/.claude/settings.json, including entries left by contracts retired
    in earlier versions, leaving all other keys and hooks intact. Also removes only our Codex
    SessionStart, PreToolUse and PostToolUse entries from CODEX_HOME/hooks.json (default
    ~/.codex), if that home exists.

overlay status-hooks
    Check overlay installation health: binary present, hooks wired, contracts readable.
    If the engine binary is missing, exits non-zero and directs users to run `overlay install-hooks`.
    Exits 0 if all OK, 1 if any check fails, 2 for warnings alone. Safe to run at any time.
    Checks all three Codex memory entries only when CODEX_HOME (default ~/.codex) exists;
    names missing or drifted events with the install-hooks remedy. Present does not mean
    trusted: each new handler needs human trust in the Codex TUI (/hooks, key t).

overlay doctor [--fix]
    Read-only host-toolchain preflight: checks go (required), gentle-ai, bat/rg/fd/sd/eza,
    the engine binary, the skill registry of the project (judged by the engine, the check
    `status --only registry` makes: absent passes, empty or unreadable fails, one without
    a scoped block warns; with no engine, or one too old to know the question, it warns that it was not checked, and an
    engine that ran and ended with a code it does not give for a report fails doctor), and a per-target release
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
    Regenerates (or checks), for each of the eight roles
    - agents/<role>.md
    - pi/agents/<role>.md
    - codex/agents/<role>.toml
    from roles/<role>/body.md and the table of engine/agentgen/roles.go.
    The verb keeps the name it had when the same run also wrote the GADU operator.

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
    candidates   [--project-root <abs>] [--status <status>]... [--kind <kind>] [--drafts] [--json]
                 read-only Engram candidate and draft listing with reconciliation anomalies; source failures exit 2
    validate     [--registry <path>] [--manifest <path>] --source-root <path>      cross-check registry vs manifest and skills/ on disk vs manifest, and that every global skill has a valid approval record (the 35 of the 37 global skills present at the Phase 8 base, which is `main` at `53f545d`, the commit the skill lifecycle was built on, that are still registered — `gadu-operator` and `gadu-orchestrate` were retired — are grandfathered while their bytes are unchanged); exit 1 on any divergence
    install      [--registry <path>] [--source-root <path>] [--project-id <id>]   install the project-scoped skills admitted for the project into <cwd>/.claude/skills/ and <cwd>/.agents/skills/, replacing only what it installed and left unmodified (see below)
    adopt        [--registry <path>] [--source-root <path>] [--project-id <id>]   record skill directories already in the project as installed by `skills install`, only when they are exactly the current source
    add          <id> [--registry <path>] [--manifest <path>] [--source-root <path>] [--repo <url>] [--ref <sha>]  register a skill (custom or external); refused, with nothing written, unless skills/<id>/.approval.json approves the exact SKILL.md bytes (see approve)
    remove       <id> [--registry <path>] [--manifest <path>]                      unregister a skill from registry and manifest
    sync-manifest [--registry <path>] [--manifest <path>]                          regenerate */SKILL.md rows from registry; preserves all non-skill lines
    approve      --id <id> --approver <label> --source-root <path>                 record a human approval of skills/<id>/SKILL.md in skills/<id>/.approval.json, bound to the SHA-256 of its exact bytes
                 The engine cannot prove a human ran approve: the record only proves it matches the exact bytes of the skill beside it.
                 A skill that fails the hard lint is refused, except a baseline skill (the 35 grandfathered global skills, which predate the lint budget and will be
                 rewritten in a later feature): for those, approve prints each hard finding as a `warning: [lint:<rule>] ... (baseline skill: approved with lint findings)`
                 line on stderr, still records the approval of the exact bytes, and exits 0, so an upstream merge that changes one of them can be approved. Only the
                 legacy rules warn (body-hard-budget, description-max, description-one-line); any other hard finding, such as a missing front matter, refuses a
                 baseline skill too. `add` is unchanged: it still refuses a hard lint finding for every skill.
                 Approval is a human step: a PreToolUse hook, installed by install-hooks, denies the agent running approve or writing the record by hand (see
                 `skills guard-hook` below). It is a speed bump, not a security boundary; a person runs approve in a terminal.
    lint         <path> | --rules                                                  lint a SKILL.md file against the authoritative rule table, or print the table; exit 1 on any hard finding (warnings never block)
    project-register --project-root <abs> --candidate <key> [--dry-run] [--registry <path>] <draft-file>
                 register an agent-owned, project-tier procedural skill: write <root>/.claude/skills and <root>/.agents/skills and the project lock; --dry-run prints the plan and writes nothing
    project-revise   --project-root <abs> --candidate <key> [--dry-run] [--registry <path>] <draft-file>
                 revise an agent-owned project skill the same way
    project-status   --project-root <abs> [--registry <path>] [<id>]
                 report project-tier ownership and global supersession from the project lock (read-only)
    project-retire   --project-root <abs> [--dry-run] [--reason <text>] [--absorbed-into <id>] [--registry <path>] <id>
                 retire an agent-owned project skill; an explicit decision that nothing in the engine takes for you, not even the retirement detector; --dry-run prints the removal plan
    The project tier stays autonomous: project-register, project-revise, and project-retire need no approval record.
    What the approval digest binds. The record holds the SHA-256 of the exact bytes of SKILL.md, so any byte change makes it stale. A tree-wide rewrite of
    line endings or whitespace (for example a git attribute change such as `text=auto eol=crlf`, or an editor that normalizes whitespace on save) therefore
    changes every skill at once: each baseline skill then needs `approve` again (validate reports APPROVAL_MISSING) and so does each approved skill
    (APPROVAL_STALE). That is the intended guarantee (exact bytes), not a bug; do such a rewrite deliberately, and approve the skills afterwards.
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
    Handoff versions 2 to 4 can reach ready: each acceptance item is {"criterion": ..., "verification":
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
    Version 4 is version 3 with memory_scope replaced by memory_directive, a strict MemoryDirective v1 object
    ({"version": 1, "scope": "none"|"goal"|"project", "sources": [...], "write": "none"}, the same object
    `memory plan` reads); an unknown or case-variant key inside it is refused. A version 4 handoff pairs with a
    Goal version 3, which likewise replaces memory_scope with memory_directive; handoff versions 1 to 3 pair
    with a Goal version 2, and any other pairing is refused (goal_invalid). Goals versions 1 and 2, and handoff
    versions 1 to 3, still parse and narrow nothing. The version 4 view (headed view 4) shows each directive on
    one line, for example memory_directive "version=1 scope=goal sources=[engram,longterm-mem] write=none"
    beside goal_memory_directive.

overlay roles <verb>
    Forward reusable-role handoff verbs to the engine unchanged. Roles are data only: a closed
    vocabulary (prototyper, shaper, estimator, builder, sweeper, polisher, reviewer, delivery,
    with sweeper and polisher optional) and a typed RoleHandoff record chained by SHA-256 to
    the previous record. A v2 record may carry an optional `memory_directive` (a MemoryDirective
    v1 object) that narrows what the next role reads; a chain may move from v1 to v2, never
    back. No verb launches an agent, dispatches work, or grants execution authority.
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
        query plan as JSON. --goal, when given, is parsed with the Goal v2 or v3 schema and
        supplies the plan's project_id (and goal_id, for scope goal) filters; a Goal v3's own
        memory_directive narrows first, before --goal-directive, as it does in a workflow created
        from that Goal. Exit 0 on a resolved
        plan, 2 on a refused or invalid input, 1 on a usage error or a failed write of the plan.
        This form tries a directive before a workflow records it.
    plan --project <id> --workflow <id>
        Print the plan the memory gate applies to the workflow, computed from its log alone, in
        the same JSON: the profile's ceiling (from the profile snapshot the log carries) narrowed
        by the Goal's directive, then the shaper handoff's, then the role handoff directive in
        force (the latest narrowed event's, or the one recorded at create; a role handoff with no
        directive means the base). The project filter is the identity the workflow recorded, or
        its slug when it recorded none. A workflow created before narrowers (a log of version 1
        or 2) holds none, so its plan is the profile's ceiling. It does not combine with
        --profile, --goal, --goal-directive or --handoff-directive (a usage error, exit 1), and
        --project and --workflow go together. Exit 2 for a workflow that is not owned or a plan
        that cannot be computed.
    plan --instructions [--project <id> --workflow <id>]
        Print that plan as the memory instructions an agent follows (C23 M6), the same block the
        SessionStart hook puts into a session and gate-task appends to a sub-agent's prompt:
        the sources to read from, the project each source is read with (Engram by the name it
        gives the workflow's identity, longterm-mem by the plan's project), under scope goal the
        goal to pass to every longterm-mem query and the `goal:<id>/` topic_key prefix of a goal's
        Engram records, that writes are not limited, and the T1/T2 procedural-candidate contract;
        under scope none it says to read no memory. With --project and --workflow it is that
        workflow's; without them, `labdrian memory plan --instructions` reads the binding of the
        repository of the working directory, and when no workflow is bound it prints that no
        workflow is bound to this repository and exits 0. A binding or workflow that cannot be
        followed, or a closed one, is refused with the hook's own warning (exit 2). It does not
        combine with --profile, --goal, --goal-directive or --handoff-directive (exit 1).

overlay workflow <verb>
    Forward standalone workflow lifecycle verbs (Phase 6) to the engine unchanged: create,
    start, pause, resume, narrow, stage, verify, close, and status, plus the session-binding verbs bind,
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
    create --project <id> --workflow <id> --goal <path> --profile <name> [--role-chain <id>] [--handoff <path>]
        Create a workflow: binds the Goal's SHA-256 digest, the named Workflow Profile, and,
        when --role-chain is given, that role chain's current head digest (the chain may grow
        afterward, but verify requires this exact record to still be present). A new workflow
        is a log of version 3, which also records the memory narrowers there are: the
        memory_directive of a Goal v3, the directive of the role chain's head record (a role
        handoff v2 that carries one), and, with --handoff, the SHA-256 of the shaper handoff
        file's bytes and its memory_directive. --handoff is read like --goal and must be a
        shaper handoff v4 (the only version with a directive) of the Goal's project_id and
        goal_id, paired with a Goal v3; any other is refused. A directive that widens the plan
        (a scope above the running one, or a source not in it) is refused with its name, for
        example "the role handoff memory directive widens the plan: adds source longterm-mem not
        present in the current scope", and nothing is written.
    start | pause | resume --project <id> --workflow <id>
        Advance the workflow's status (created -> running -> paused -> running -> ...).
    narrow --project <id> --workflow <id>
        Record the role handoff the bound role chain received since the log last recorded its
        head: one narrowed event with the chain's new head digest and the directive that record
        carries (none when it carries none, which means the base: the ceiling narrowed by the Goal
        and the shaper handoff). Only the latest role directive applies, over the base, so a later
        role can get back to the base and never past it. Legal while created, running, or paused;
        it changes no status, and a completed close still needs a verify right before it. Refused,
        writing nothing, for a workflow with no role chain, a chain that no longer verifies or no
        longer holds the last recorded head, a chain with no new handoff, a directive that widens
        the plan, and a workflow created before narrowers (version 1 or 2: create it again).
        Prints the state; the narrowers appear in it only when there are any.
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

gentle-ai-overlay manifest routes --overlay-root <dir> --target <name> --skills-root <dir> [--agents-root <dir>] [--manifest <file>] [--managed]
    Engine verb (Phase 9, B1): there is no `overlay manifest` wrapper, so run it on the installed engine binary,
    ~/.claude/bin/gentle-ai-overlay. It decides where each row of overlay.manifest goes, and is what `apply`, `status`,
    `sync-check`, `capture`, `restore` and `bootstrap` ask. Read-only: it prints one line per
    row, in the order of the manifest, in five columns separated by a tab: the row's path; its route (`skill`, `agent`, `opencode-agent`, `codex-agent`, `mcp`,
    `non-deployable`, or the third column when it is none of these, which takes the skill route under its own name);
    where its file is in the overlay repository (`skills/<path>` for a skill, `agents/<path>`, the path itself for the
    others); the absolute path the overlay copies it from (empty for a row that is never deployed); and the absolute
    path it is copied to for --target (empty when the row goes nowhere for it: an agent for a runtime other than Claude
    Code, a native agent for one other than OpenCode, a Codex agent for one other than Codex, an mcp row, a row that is
    never deployed). A column with nothing to
    say is empty, and an empty column is never followed by one that is not. --managed keeps the rows whose second column
    is `managed`. The script owns the directories of its targets and hands them over: --skills-root is where --target
    keeps its skills, --agents-root where it keeps its native agents (needed when the manifest routes an agent to claude,
    a native agent to opencode or a Codex agent to codex); --manifest defaults to <overlay-root>/overlay.manifest. A manifest is read as the script
    always read it: a line is blank or a comment when only white space comes before its end or a `#`, and columns are
    separated by spaces and tabs alone. A path named twice takes the route of the first row that names it. A row under
    longterm-mem/ with no route, or one outside skill, agent, opencode-agent, codex-agent and mcp, is refused ("route_resolve:
    longterm-mem row '<path>' declares ..." on stderr), so are an unknown --target and a manifest that is there and
    cannot be read; a manifest that is not there has no row, and a warning on stderr says so (as the script always
    read it). Exit 0 on success; 1 on a usage error (including an unknown flag) or a refusal, with nothing on stdout,
    so a caller never acts on part of a manifest.

gentle-ai-overlay registry repair-sdd --registry <path> --target <name> --skills-root <dir>
    Engine verb (Phase 9, B2): there is no `overlay registry` wrapper, so run it on the installed engine binary,
    ~/.claude/bin/gentle-ai-overlay. It is what `overlay repair-sdd-registry` and `overlay skill-registry refresh` ask, after
    gentle-ai has regenerated the registry. gentle-ai leaves the sdd-* phase skills out of the skill registry it writes, because
    they are executor contracts and not skills a person invokes; SDD delegation still needs their paths, so this puts a row back
    for each one installed in --skills-root (`<dir>/<name>/SKILL.md`; --target only names the target in what it says), with the
    scope `executor`, above the `## Loading protocol` heading of the registry at --registry, and a note that says why above
    the `## Skills` heading. The note and the rows of an earlier repair are replaced; every other line stays as it was. A project
    that has no registry gets one. The description of a row is the text after the first `description:` line of the skill (one
    double quote off each end, every `|` escaped), or `SDD phase executor: <name>.` when it has none. It takes the lock
    `propagate` takes (`<registry>.lock`, waiting up to 2 seconds), so a repair and the `propagate` hook that runs on every prompt
    take turns, and it writes the registry whole through a temporary file and a rename (mode 0644), so a reader never sees half of it.
    Each executor that is not installed is said on stderr (`WARN: missing <target> SDD executor skill: <path>`), and the run is
    summed up on stdout: `repair-sdd-registry: registry=<path> target=<name> sdd_executors=<n> missing=<m>`. Two things the script
    always did and the engine keeps: the rows land after the blank line that precedes the loading protocol heading, and a second
    repair adds a blank line above the note and one above that heading. Exit 0 on success; 1 on a usage error (including an
    unknown flag), a registry without the headings `## Skills` and `## Loading protocol` (`ERROR: registry has no '<heading>'
    section: <path>`), a lock that stays taken, or a file that cannot be read or written, with the registry as it was.

gentle-ai-overlay installed-targets record|release --state-dir <dir> --target <name>
gentle-ai-overlay installed-targets list --state-dir <dir>
gentle-ai-overlay installed-targets assess --state-dir <dir> [--purge] [--unregister-failed]
gentle-ai-overlay installed-targets retire --state-dir <dir> --binary <path> [--purge]
    Engine verbs (Phase 9, B3): there is no `overlay installed-targets` wrapper, so run them on the installed engine binary,
    ~/.claude/bin/gentle-ai-overlay. They are what `overlay longterm-mem install` and `uninstall` ask. longterm-mem is one
    binary, deployed once at `<state dir>/bin/longterm-mem` and shared by every runtime it is registered with, and the overlay
    keeps a record of the runtimes it registered it with: the file `<dir>/longterm-mem/installed-targets`, one name a line. It
    is the only thing that says whether a runtime still depends on the binary, and the binary goes when it is empty and not
    before. `record` adds `--target` to it, once, after longterm-mem has registered that runtime (a name this build cannot
    install for is refused), and appends it after whatever the file held, in one write (a write that stops short leaves a partial last line, which the reader drops). `release` takes it out, after longterm-mem has
    removed that runtime's entry, rewriting the record from the names it vouches for, through a temporary file and a rename
    (mode 0600), and a sync of the directory after it so that a crash cannot bring the old record back (a directory that cannot
    be synced is exit 1, with the record already the new one): garbage is compacted away, a name only a newer build knows stays, and the last name leaves an empty record,
    which is not an absent one. `list` prints the names, one a line. A name is a bare word of at most 64 bytes that begins with a
    letter or a digit and goes on with letters, digits, dots, underscores and dashes; a NUL byte, a blank or padded line, a
    carriage return and a line the file ends in the middle of are not names and are dropped when the record is read. A
    record that is there and cannot be read is never appended to or rewritten, and `list` fails on it: "I could not look" is
    not "there is nothing". `assess` says what an uninstall comes to once its targets are dealt with, from the record as it is
    and without the lock: exit 0 and nothing said when the registration record and the shared binary may go (the record is
    there and holds nothing, or `--purge`), exit 3 and the reason when they are kept (`--unregister-failed` says that a
    target was not unregistered; the record cannot be read; there is no record, which means that what is installed is not
    known and never that nothing is; or a target is still installed, and the names are printed). `retire` removes the binary
    at `--binary` and then the record, and says which (`==> longterm-mem binary removed: <path>` or `already absent`), when the
    record read again under the lock says that nothing depends on the binary or `--purge` forces it: a target recorded since
    `assess` keeps the binary, and a record that cannot be read does too. `--binary` is the path the overlay deployed the binary
    to and wrote into every runtime's entry; the engine does not derive it. Every read-modify-write (`record`, `release`,
    `retire`) is one critical section under an advisory lock on `<dir>/.installed-targets.flock`, a file of its own beside
    the directory of the record, waiting up to 2 seconds for a holder; the kernel lets it go when the process ends, so there
    is no abandoned lock to break (the script's lock was a directory, `.installed-targets.lock`, and one a killed run left
    behind is ignored). The words are the script's (`ERROR: longterm-mem: ...`, `WARN: longterm-mem: ...`). Exit 0 on success;
    1 on a usage error (including an unknown flag), a lock that stays taken, or a file that cannot be read or written, with the
    record as it was; and 3 from `assess` when the registration record and the binary are kept.

gentle-ai-overlay merge-codex-hooks --hooks <path> --hook-command <binary-path>
gentle-ai-overlay uninstall-codex-hooks --hooks <path> --hook-command <binary-path>
    Merge/remove three owned Codex entries: SessionStart, matcher startup|resume|clear|compact,
    and PreToolUse and PostToolUse, with an anchored matcher for only the four Engram reads
    (including plugin prefixes) and longterm_mem query/get. Each runs projection hook --event
    <event>, with --source codex on the tool entries only and shell quoting from the Claude
    command builder. Ownership requires the binary path and projection hook in the same
    command. Foreign values keep their bytes; edited array layout may change.
    Writes atomically with <path>.bak; identical merge or absent uninstall writes nothing.
    Malformed hooks.json is refused unchanged. Exit 0 success, 1 usage error, 2 invalid or
    unwritable file. The parent directory must exist. Neither verb edits config.toml, trusts a
    hook, or starts Codex. Trust each new handler once in the Codex TUI (/hooks, key t): two
    more trusts on a SessionStart-only install, whose original entry and trust stay unchanged.

gentle-ai-overlay status [--only registry] [--cwd <dir>]
    Engine verb: `overlay status-hooks` runs it on the installed engine binary, ~/.claude/bin/gentle-ai-overlay. It is the doctor
    of an installation: one line per check -- the binary, the hook families and guards in Claude Code's settings.json, the
    minimalism contract and the skill registry of the project -- as `[OK  ]`, `[WARN]` or `[FAIL]` with the words that say why.
    Exit 0 when every check is OK, 1 when one failed, 2 when none failed and one warns. `--cwd` names the directory whose
    `.atl/skill-registry.md` is looked for (the working directory by default). `--only registry` reports that registry alone, as
    the one line the full report would give, and exits with the code of that one check: a registry that is not there is OK (a
    project that does not use the overlay is no problem), one that is empty or white space only or cannot be read fails, one
    that lacks a scoped block (`minimalism-contract-scope`, `anti-generic-design-scope`) warns, and one that has both is OK. It
    is how `overlay doctor` judges the registry of a project, so the two never judge it differently. The arguments are read
    strictly, left to right. A flag takes its value as the next argument or after an equals sign in its own (`--cwd=<dir>`,
    `--only=registry`); the second is how a directory whose name starts with two dashes is given (`--cwd=--odd`), since a
    flag is never taken for the value of another. Any other argument, a flag without its value (missing, empty, or another
    flag in its place, so `--cwd --only registry` is a `--cwd` with no value), and an `--only` that names no check are usage
    errors, exit 1 with the reason on stderr and nothing on stdout. A usage error is not exit 2, which is the code of a
    degraded report. The callers in this repository are the overlay script's `status-hooks` (no argument) and its `doctor`
    (`--only registry --cwd <dir>`), and a test holds both.

gentle-ai-overlay runtime capabilities [--target claude|codex|pi|opencode|all]
    Engine verb (Phase 7): there is no `overlay runtime` wrapper, so run it on the installed engine
    binary, ~/.claude/bin/gentle-ai-overlay. Read-only and declarative: it prints, as JSON, what
    each runtime adapter declares it supports (default --target all; a single target prints one
    declaration). A declaration has one claim per capability -- installation, projection, dispatch,
    cancellation, persistence, restart, authentication, memory-enforcement, skills -- each supported,
    partial, or unsupported. A supported or partial claim names the tests that prove it as
    <directory under engine/>:<TestName>, and a guard test verifies every named test exists; a
    partial or unsupported claim states its limit in "detail". "untested" appears only on a runtime
    that cannot be exercised on this machine (opencode). The output states what the engine's own
    tests prove, never what a runtime happened to do: a capability no test proves is declared
    unsupported, with the limit written. It reads no configuration, HOME, or file and starts no
    session. Exit 0 on success, 2 on an unknown --target value, 1 on a usage error (including an unknown flag).
    Codex memory-enforcement is partial: trusted MCP read hooks check plan sources, query/search project,
    and goal-scoped query goal. Codex fails open on hook failure; procedural skills, Bash and apply_patch
    are not enforced, reads by id have no project validation, and agent_id is ignored. The M11 acceptance
    table separately proves evidence supported for successful MCP reads only and availability partial.
    These are isolated engine hook process proofs, not live Codex validation (C23 M12b).
    The skills claim (Phase 8) says what carrying the overlay's governed skills into a runtime's skill
    directories is proven to do, and it is partial for all four runtimes. The project tier -- skills install
    and skills adopt (.claude/skills and .agents/skills), and project-register, project-revise, and
    project-retire -- is proven by engine tests: ownership by hash, foreign files kept, locks, the approval
    record that skills add needs for a global skill. The global tier is deployed by `labdrian apply` (the
    overlay script), not by the engine, and there is no standalone global installer: its tests run the script
    on a fixture overlay and a sandbox home, never the real skills tree. Unused-skill detection is report-only
    and exists for the project tier only (longterm-mem's skillstale check); a global skill is retired by hand.
    No test observes a runtime loading a skill. Codex reading a project skill at .agents/skills was observed
    once live (2026-09-28, Codex 0.148.0) and is recorded in odd/tasks/phase5-closeout.md, not tested. A live
    OpenCode 1.18.31 session discovered project skills placed under .agents/skills and .claude/skills (2026-09-30;
    recorded in the Phase 8 ledger, odd/tasks/skill-lifecycle.md, not tested); its global tier was not observed live,
    because the skill names also exist in several global directories OpenCode reads. A bound Claude Code
    session's projected context lists every capability that is not supported, so it now lists skills=partial
    with the others.

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

gentle-ai-overlay projection hook --event UserPromptSubmit|PreToolUse|SessionStart|PostToolUse [--source claude|codex]
    --source (default: claude) names the runtime that sent the hook; codex maps the tool names Codex sends
    for longterm-mem reads (mcp__longterm_mem__query, mcp__longterm_mem__get) to the ones the gate knows, before
    the gate or the evidence sees the call. Nothing else changes; another value is a usage error (exit 1).
    Internal hook command (Phase 7): Claude Code is meant to run it before each prompt (UserPromptSubmit),
    before each tool call (PreToolUse), when a session starts (SessionStart) and after a memory read has run (PostToolUse); a person does
    not. There is no `overlay projection` wrapper, so it runs on the installed engine binary,
    ~/.claude/bin/gentle-ai-overlay. `install-hooks` installs it into Claude Code settings.json
    (one UserPromptSubmit entry and five PreToolUse entries: the file-edit tools, the longterm-mem query tool, the
    longterm-mem get tool, the Engram reads mem_search, mem_context, mem_get_observation and mem_timeline, and
    the Skill and Read tools; plus one SessionStart entry, and four PostToolUse entries with the same memory-read
    matchers: the longterm-mem query, the longterm-mem get, the Engram reads, and Skill and Read); removing the hooks removes all of them;
    on an existing install, re-run `install-hooks` and restart Claude Code to load it, because Claude Code reads
    hooks only at start and `status-hooks` reports degraded until the entries are in place. Its tests feed it hook JSON.
    UserPromptSubmit, PreToolUse, SessionStart and PostToolUse are the events supported.
    Codex uses this same SessionStart command and renderer, with cwd from its hook stdin;
    no bound workflow or unusable input prints nothing and exits 0. Its two tool entries pass
    --source codex and match only MCP memory reads, not procedural skills, Bash or apply_patch.
    Each new handler must first be trusted by the human in the Codex TUI (/hooks, key t).
    Codex fails open on hook failure; reads by id have no project validation and agent_id is ignored.
    Its PostToolUse receives successful MCP results only: the shared sidecar stores their SHA-256 and
    byte length, never response content. Failed MCP calls have no PostToolUse record. Availability stays
    partial; the gate and evidence are proven by isolated engine processes, not a live Codex session.
    See docs/architecture/adr-c23-m12b-codex-memory-gate.md.
    PostToolUse (C23 M7, docs/architecture/adr-c23-m7-read-evidence.md): after a memory read the PreToolUse
    gate checks has run (an Engram mem_search, mem_context, mem_get_observation or mem_timeline; a longterm-mem
    query or get; the Skill tool for a procedural skill of the lock, or a Read of its SKILL.md), while an open
    workflow is bound, append one line to `<state home>/labdrian/workflows/<project>/<workflow>.memory.jsonl`
    (0600, its own lock; the workflow log is never touched): the tool, the source, the project and goal or
    the id or skill the call named, the call and session ids, and the SHA-256 and length of tool_response,
    never the response itself. Its input may be up to 64 MiB. Writes and promote are not recorded. Silent,
    or, when the line cannot be written, one
    {"hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"..."}} telling the agent the
    evidence of that read was not recorded and why.
    SessionStart (C23 M6): when a session starts, resumes, is cleared or is compacted, put the memory
    instructions of the workflow the repository is bound to into its context, once, as
    {"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"..."}}: the block
    `labdrian memory plan --instructions` prints, plus (C23 M8) one line, "Do not read <source>: it is
    unavailable in this session (<reason>).", for each source of the plan that the hook found it cannot read:
    it runs `engram stats` and the longterm-mem binary and looks for a longterm-mem MCP entry in any runtime
    and for the project's procedural skills lock, each with a three-second deadline, all at once. The line
    only informs; the gate is unchanged. Silent when no workflow is bound, and for a binding or
    workflow it cannot follow, a closed one, a store it cannot read, or input it cannot use (the next
    prompt warns about those); read-only, it never removes a binding. Each UserPromptSubmit keeps its
    one-line memory plan, so a workflow bound or narrowed in the middle of a session is still seen.
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
    plan: scope, sources, project_id, goal_id, write=none; the profile ceiling narrowed by the
    directives the workflow log stored, exactly as `memory plan --project --workflow` prints it), the dependencies the last event recorded
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
      - Memory gate, for a created, running, or paused workflow, against the workflow's memory plan (the
        plan the context states: the profile ceiling narrowed by the directives the log stored). It checks
        the memory reads: the longterm-mem `query` and `get` tools, and the Engram `mem_search`,
        `mem_context`, `mem_get_observation` and `mem_timeline` tools, each by its MCP name, bare
        (mcp__longterm-mem__query, mcp__engram__mem_search) or plugin-prefixed
        (mcp__<plugin>_longterm-mem__query, mcp__plugin_engram_engram__mem_search); a lookalike server
        name (mcp__engramx__mem_search) is not one. A plan with no project (scope none, the
        standalone-minimal profile) permits no read at all, and a read of a source the plan does not
        include is denied (the sdd profile's plan holds engram alone, so a longterm-mem query or get is
        denied; the reason names the source and the profile and says to unbind). Then a `query` is
        denied when its `project` argument is missing, not a string, empty, or different from the plan's
        project (the reason names both projects and says to query the plan's project or unbind), or,
        under scope goal, when its `goal` argument is missing, not a string, empty, or different from
        the plan's goal (the reason names the plan's goal and says to query the plan's goal or unbind), and a
        `mem_search` or `mem_context` is denied when its `project` is missing, not a string, empty, or
        other than the name Engram gives the workflow's identity (its last path segment:
        github.com/acme/proj-1 is proj-1), or when it sets `all_projects` to true. A read by id (`get`,
        `mem_get_observation`, `mem_timeline`) passes once its source is in the plan. A workflow that
        recorded no project identity (created before workflows recorded one) has its project checks
        skipped with one warning. The project's procedural skills are of source `procedural-skills`: the
        Skill tool for a skill the project's procedural skills lock records
        (`.labdrian/procedural-skills.lock.json` at the worktree root), and a Read whose absolute path,
        cleaned, is one of that skill's SKILL.md files (the lock's targets under the worktree root), are
        denied when the plan does not include that source (sdd), naming the skill. A skill the lock does
        not record, a skill the lock records as a registry install, and a global skill pass. A Read of any
        other file is not looked at: the hook decides that from the path before it reads the binding,
        the workflow, or the lock. No lock means no procedural skill. A lock that cannot be read allows,
        with one warning, when the plan would have denied a procedural skill.
        It never denies `promote`, an Engram write, or any other write.
      - Everything else is allowed: no binding, a binding or workflow it cannot follow (foreign, malformed,
        drifted, unavailable, gone), a closed workflow, an unrecognized status, a plan that cannot be
        computed, or input it cannot use. It never blocks on an unknown state.
    Output: a denial is exactly one JSON object,
    {"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"..."}},
    with exit 0 (the JSON decides; exit 2 is never used). An allow prints nothing, or only a
    {"systemMessage":"..."} warning when the gate had something to check and could not (a memory plan it
    cannot compute, a query or Engram read whose input is not an object, a procedural skills lock it cannot read, a recovered internal error, which also goes to
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
    Input: the hook JSON on stdin, at most 8 MiB (input over that is denied, unjudged: the agent controls the length of its own command). Only tool_name and the command,
    file_path, and notebook_path fields of tool_input are read.
    Output: nothing, or exactly one JSON object,
    {"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"..."}},
    with exit 0 (the JSON decides; exit 2 is never used). An allow prints nothing and never permissionDecision "allow".
    It never blocks on an error: input it cannot decode, a failing stdin or stdout, and a recovered internal error all
    allow the call (unlike the shaper clearance guard, which fails closed for its narrower markers, because this hook runs
    on every Bash and file-edit call). A recovered internal error also shows one short sanitized `systemMessage` naming
    the approve guard and saying the tool call was not checked and was not denied, so a guard that stopped guarding does
    not look like one that allowed; the full error goes to stderr. It is never a permission decision. With the engine binary missing the hook does nothing, and `status-hooks` reports it.
    Exit 0 always, except 1 on a command line it does not understand.

overlay --help
    Show this help.
```
