# labdrian-sdd-overlay

A vendor+overlay git framework that lets your customized gentle-ai skills survive `gentle-ai sync`/upgrade cycles.

## What this is

`gentle-ai sync` and `upgrade` overwrite `~/.claude/skills/` with pristine vendor files. This repo uses two long-lived git branches to track the vendor baseline and your customizations separately, then merges and deploys with a single command.

- **`upstream` branch**: pristine vendor baseline extracted from gentle-ai upgrade backups
- **`main` branch**: your customized overlay — rebases on top of each new upstream

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

## Normal update cycle

After every `gentle-ai sync` or `gentle-ai upgrade`:

```bash
# 1. Refresh upstream with new vendor files
overlay capture --from-backup ~/.gentle-ai/backups/<new-backup>/snapshot.tar.gz
# (or just: overlay capture   — to pull from ~/.claude/skills/ directly)

# 2. Merge upstream into main and deploy to ~/.claude/skills/
overlay apply
```

That's it. If there are merge conflicts, `overlay apply` exits 1 and tells you exactly which files to resolve.

## One-time bootstrap

```bash
# Clone or copy this repo to ~/labdrian-sdd-overlay/, then:
cd ~/labdrian-sdd-overlay
chmod +x bin/overlay
bin/overlay bootstrap
```

Bootstrap will:
1. Init the git repo
2. Extract managed files from the backup tarball onto `upstream`
3. Create `main` from `upstream`, then layer in your current customized files

## Adding a new tracked skill

1. Add a line to `overlay.manifest`:
   - `<relative-path> managed` — if it has a vendor counterpart
   - `<relative-path> custom` — if it's purely your own addition
2. If managed: run `overlay capture --from-backup <tarball>` so upstream gets the vendor copy
3. Copy your customized version into `skills/<path>` and commit on `main`
4. Run `overlay apply` to deploy

## CLI reference

```
overlay bootstrap          One-time setup: init repo, seed upstream, create main
overlay capture            Refresh upstream from ~/.claude/skills/ (post-sync)
overlay capture --from-backup <tarball>
                           Refresh upstream from a specific backup tarball
overlay apply              Merge upstream into main and deploy to ~/.claude/skills/
overlay status             Show branch, diff stat, and per-file drift detection
```
