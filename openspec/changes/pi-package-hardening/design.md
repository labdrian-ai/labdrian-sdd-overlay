# Design: Pi Package Hardening and GADU as a Real Pi Subagent

## Technical Approach

Extend existing seams only: `engine/pipkg` + `engine/skills/parse.go` (R-001..R-004), `pipkg.Check` comparison basis + additive `package.json` field (R-005..R-007), a new `engine/reviewreceipt` package wired as a fail-closed `PreToolUse` Bash hook plus `tools/archive-anchor-gate` consumption (R-008..R-011), and `engine/runtime/pi.go` for the Subagents extension, GADU link, status and uninstall (R-012..R-016). `cmd_apply`'s deploy-from-`main` contract is untouched. Spec scenarios expected per slice are listed under Testing Strategy.

## Architecture Decisions

| # | Decision | Choice | Rejected | Rationale |
|---|---|---|---|---|
| D1 | Mode drift (R-001) | `listFiles` returns `{data, perm}`; `Check` diffs `Perm()` and reports `<rel>: mode 0644 -> 0755`. `copyFile` keeps writing 0644 (the registry-recorded mode is the build output). A symlinked `destDir` root is already refused by `listFiles` (WalkDir lstat) — keep, add RED test. | Preserve source exec bits | No package file is executable; preserving bits widens the build contract for no consumer. |
| D2 | Build root (R-002) | `os.Chmod(tmpDir, 0755)` right after `MkdirTemp`. | Custom mkdir + random suffix | stdlib rung. |
| D3 | Path containment (R-003) | `validateEntry`: `path` must be non-empty, relative, contain no `..` component, and `filepath.Clean(path)==path`. `buildInto` re-checks `filepath.Rel(skillsDir, dst)` has no `..` prefix (defense in depth). | Only pipkg check | Registry parse is the earlier, shared boundary; pipkg check guards future callers. |
| D4 | SKILL.md name (R-004) | `pipkg.buildInto` reads `<src>/SKILL.md`, scans the frontmatter block line-by-line for `name:`, requires it to equal `filepath.Base(e.Path)`. Lives in pipkg because `validateEntry` is filesystem-free by design. | YAML dependency; parse in `engine/skills` | Zero-dep invariant (ADR-1); parse.go stays pure. Spec should place R-004's test in `pipkg_test.go`, not `validateEntry`. |
| D5 | Provenance (R-005) | `packageManifest.Labdrian{BuiltFrom string}` = `git -C overlayRoot rev-parse HEAD` (omitted when not a git repo). `resolvePackageVersion(root, rev)` gains a `rev` argument. | Pass ref from shell | Build already shells to git; single source. |
| D6 | Comparison basis (R-006/R-007) | `Check` returns `(CheckReport{Basis, Ref}, error)`. Basis order: (1) `builtFrom` matches `^[0-9a-f]{40}$` and `git cat-file -e <sha>^{commit}` → export `skills/ agents/ skills.registry.yaml` at that sha via `git archive --format=tar` + `archive/tar` into a temp root and `buildInto` from it, `builtFrom` forced to the sha; (2) unresolvable/absent → same export at `main`, `Basis="main"`, message states `compared against main; builtFrom <sha> is not resolvable locally`; (3) overlay root not a git repo → working tree, `Basis="worktree"`. `SyncCheck`/`Status`/`pipkg check`/`SYNC_CHECK:pi:` print the basis. | `git worktree`; `git show` per file | archive+tar is stdlib and atomic; worktree mutates `.git`. |
| D7 | Receipt capture (R-008) | New `engine/reviewreceipt`: `Capture(repo, change)` validates `schema=="gentle-ai.review-receipt/v2"` and `terminal_state=="approved"`, atomically writes byte-identical `openspec/changes/<change>/review-receipts/<lineage_id>.json`. Enforced by a **fail-closed** `PreToolUse` Bash hook (`gentle-ai-overlay review-receipt hook`) matching `gentle-ai review acknowledge-approved`: captures every surviving approved receipt when exactly one active change (non-archive dir under `openspec/changes/` with `state.yaml`) exists; passes through when the repo has no `openspec/changes/`; **denies** (exit 2, message naming `review-receipt capture --change <name>`) when several are active. Hook registered by `install-hooks` under a third settings identity. | Edit `sdd-orchestrator-workflow.md` | That file is `managed` (upstream) in `overlay.manifest`; a hook is overlay-owned and mechanical. |
| D8 | Receipt consumption (R-009/R-010) | `archive-anchor-gate` reads `review-receipts/*.json` in the archived folder; a recorded `approved_tree` must equal a receipt's `final_candidate_tree` (prefix rule) or the report is a Finding. closure-feedback (custom `inception-pipeline/SKILL.md`) reads `approved_tree` and `review_lens_count` from that file; the `.git/...` path is removed from its Plan. | Keep prose-only anchor | Prose is what produced three self-asserted archives. |
| D9 | Missing receipt (R-011) | New `ReceiptConventionDate = "2026-09-12"`. Reports on/after it with no receipt file → Finding `no verified receipt` unless `review-receipts/override.json` (`{"schema":"labdrian.review-receipt-override/v1","owner","reason","recorded_at"}`) exists AND the Cycle Timestamps section contains `override`; outcome then `self-asserted`. `--change <name>` flag runs the same check pre-archive on the active folder (exit 1 = block); inception-pipeline Gate Compliance lists it before `sdd-archive`. | CLI-only flag | A flag leaves no record; a file travels with the archive. |
| D10 | GADU placement (R-013) | Symlink `~/.pi/agent/agents/GADU.md -> <destDir>/agents/GADU.md`. `destDir` path is stable across `swap` (only the inode changes). Ownership = `Readlink` equals that target; no state file. | Copy + fingerprint record | Copy needs a record to tell stale from conflict; the symlink target is the record. |
| D11 | Extension probe/install (R-012) | `settings.json` `packages[]` entry with prefix `npm:pi-subagents-j0k3r` or `npm:pi-subagents` (optional `@ver`) = installed. Else `runPiCommand(bin,"install","npm:pi-subagents-j0k3r")` after printing `installing third-party Pi extension pi-subagents-j0k3r (npm) required for GADU dispatch`; `LABDRIAN_PI_SKIP_SUBAGENTS=1` skips and status says so. Runs inside `apply --target pi` (user-initiated). | Separate command; interactive prompt | apply is the consent surface; no package-choice UI (out of scope). |
| D12 | Frontmatter (R-014) | Template unchanged. Verified in `pi-subagents-j0k3r@1.5.15`: inline `tools: '*'` → `["*"]` wildcard; `model: opus` → `parseModel` returns `undefined` (no `/`) so the parent model is inherited; name lowercased to `gadu`. Add a `gadu_test` guard: `tools` is a single inline scalar (never list+inline, which blocks loading). | Provider-qualified model | Would break Claude Code's reading of the same file. |
| D13 | Status/uninstall (R-015/R-016) | `gaduLinkState(home,destDir,expected) ∈ {missing,current,stale,conflict}` (stale = our link but target missing or bytes ≠ generated; conflict = entry not our symlink). Status message carries `subagents_extension=<installed|not-installed>` and `gadu_link=<state>` as two reasons. `Uninstall` removes the link only when it is ours, then `pi remove`; never touches the extension. | Boolean "gadu ok" | Collapses failure modes (R-015 intent). |

## Data Flow

    apply --target pi (on main)                     sync-check --target pi (any branch)
      Build ──rev-parse HEAD──> package.json          read builtFrom ──resolvable?──> git archive <sha>
        └─ pi install <dest>                            │ no: git archive main + disclosure
        └─ probe settings.json ─> pi install npm:...    └─ buildInto(export) ─> diff bytes+perm
        └─ symlink ~/.pi/agent/agents/GADU.md

    review approved ─> [PreToolUse hook] capture .git/gentle-ai/.../review-receipt.json
                        └─> openspec/changes/<c>/review-receipts/<lineage>.json ─> acknowledge burns
    archive ─> folder moves ─> archive-anchor-gate: approved_tree == receipt.final_candidate_tree | override

## File Changes

| File | Action | Slice |
|---|---|---|
| `engine/pipkg/pipkg.go`, `pipkg_test.go` | Modify: D1–D4 | 1 |
| `engine/skills/parse.go`, `parse_test.go` | Modify: D3 | 1 |
| `engine/pipkg/{pipkg.go,export.go}`, `engine/runtime/pi.go`, `engine/cmd/main.go`, `bin/labdrian-overlay` (`pipkg_sync_check_and_report`) | Modify/Create: D5–D6 | 2 |
| `engine/reviewreceipt/{receipt.go,hook.go}` + tests, `engine/cmd/main.go`, `engine/settings` identity, `bin/labdrian-overlay` install-hooks | Create/Modify: D7 | 3a |
| `tools/archive-anchor-gate/{gate.go,receipt.go,main.go}` + tests, `skills/inception-pipeline/SKILL.md` | Modify/Create: D8–D9 | 3b |
| `engine/runtime/pi.go`, `pi_test.go`, `engine/gadu/gadu_test.go`, `bin/labdrian-overlay`, `README.md` | Modify: D10–D13 | 4 |

## Interfaces / Contracts

```go
type CheckReport struct{ Basis string /* "ref"|"main"|"worktree" */; Ref string }
func Check(overlayRoot, registryPath, destDir string) (CheckReport, error)
type packageManifest struct{ ...; Labdrian *labdrianField `json:"labdrian,omitempty"` }
func Capture(repoRoot, change string) ([]Captured, error)   // engine/reviewreceipt
func gaduLinkState(home, destDir, expected []byte) linkState  // engine/runtime
```

## Slice Table (stacked-to-main, 400 authored lines each)

| Order | Slice | Reqs | Est. lines | Note |
|---|---|---|---|---|
| 1 | pipkg-integrity | R-001..R-004 | 230–320 | |
| 2 | sync-check-provenance | R-005..R-007 | 280–380 | needs git in tests; skip in `-short` |
| 3a | review-receipt-capture | R-008 | 300–400 | **re-sliced**: entry's slice 3 (R-008..R-011) cannot fit 400 with hook + gate + TDD |
| 3b | receipt-anchor-gate | R-009..R-011 | 250–350 | depends on 3a |
| 4 | gadu-pi-subagent | R-012..R-016 | 320–400 | Medium overrun risk; R-014 test is tiny |

## Testing Strategy

| Layer | Scenarios (expected spec names) |
|---|---|
| Unit (pipkg/skills) | mode-only drift; symlinked dest root refused; `../` path rejected at parse and build; name≠dir rejected; name==dir passes; live registry passes |
| Integration (pipkg, git in `t.TempDir`) | builtFrom == HEAD; feature-branch unrelated diff → clean; main diverged → drift; unresolvable sha → `main` disclosure; non-git root → worktree basis; non-hex builtFrom never reaches git |
| Unit (reviewreceipt) | hook captures before acknowledge (file exists, bytes identical); no openspec → pass-through; two active changes → deny; non-approved receipt refused |
| Unit (anchor-gate) | approved_tree == receipt tree → verified; missing receipt post-convention → block; override file + prose → self-asserted, recorded; `--change` pre-archive block/pass |
| Integration (runtime, scratch HOME + fake `LABDRIAN_PI_BIN` script recording argv) | install when absent; no-op when `pi-subagents` present; skip env honoured; link state 4×2 matrix; gentle-pi-style overwrite of its own files leaves link intact; uninstall removes only our symlink |

## Threat Matrix

| Boundary | Applicability | Design response | RED tests |
|---|---|---|---|
| Documentation-like paths | N/A — no executable classification | — | — |
| Git repository selection | Applicable — `git -C overlayRoot` archive/rev-parse; `builtFrom` read from an installed file | Always `-C <abs overlayRoot>`; `builtFrom` validated `^[0-9a-f]{40}$` before any git argv; refs passed as `<sha>^{commit}` | non-hex/`--option` builtFrom never spawns git; relative overlay root resolved absolute |
| Commit state | N/A — no commits made | — | — |
| Push state | N/A | — | — |
| PR commands | N/A | — | — |
| Subprocess (pi, hook) | Applicable | Fixed argv `pi install npm:pi-subagents-j0k3r`; hook only string-matches the command, never executes it; hook denies (exit 2) rather than guessing a change | fake pi records argv exactly; hook with a look-alike command (`echo acknowledge-approved`) passes through |

## Migration / Rollout

`labdrian.builtFrom` is additive; a legacy package without it takes the `main` basis with disclosure. Receipt convention applies from 2026-09-12; earlier archives stay in `known-gaps.txt`. GADU link and extension removed by the selective uninstall; `pi remove npm:pi-subagents-j0k3r` is the user's own step.

## Open Questions

- [ ] Pre-archive enforcement is tool + documented gate (D9); a mechanical `PreToolUse` Agent deny for `sdd-archive` launches is deferred (follow-up).
- [ ] Hook coverage is Claude Code only; Codex/Pi sessions must run `review-receipt capture` manually before acknowledging.
