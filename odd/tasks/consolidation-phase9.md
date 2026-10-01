# ODD Task — Phase 9: Consolidation and polish

> **Roadmap authority:** [Standalone Platform Master Roadmap](../../openspec/decisions/standalone-platform-roadmap.md), canonical **Phase 9 — Consolidation and polish** (inserted by the user on 2026-10-01): pay down the debt Phases 5–8 deferred — the SDD/ODD skill rewrite for labdrian's sub-agent team and the deferred cleanup backlog — before building Observability (Phase 10) and Release (Phase 11).

## Problem and why

Phases 5–8 shipped with a recorded backlog of deferred items, an approval baseline that exempts 27 skills failing the lint budget, and skills that were written for a single agent rather than a team of role agents. The user decided the cleanup must not be left out, and set a governing principle: the whole system must align with hexagonal architecture and clean code to the maximum, whatever it costs.

## Inventory

Ultracode workflow `wf_afe65bf2-736` (9 read-only agents) collected 124 items from Engram, the Phase 5–8 ledgers, the skills, and repository state, and verified each against `main` `f19a33c` (Engram #3855). Items proven fixed were removed with their proof (install-hooks summary `28d2082`; workflow store flake `a6d10dc`; #309 and #312 code items; Phase 5 residuals C3/C4; SL2 `--pre`).

## Decisions (user, 2026-10-01)

- **Principle:** hexagonal architecture and clean code to the maximum, whatever it costs; every design fork offers and prefers the maximum-decoupling option (Engram #3856).
- **Q1 hook upgrade:** `install-hooks` replaces owned entries in place when their command changes; a test pins that old unquoted entries are recognized as owned.
- **Q2 upstream core skills:** create new overlay-owned role skills for the sub-agent team; leave the 15 upstream core skills untouched; document them in the vault as sources.
- **Q3 untracked tool directories:** `.gitignore` `.agents/`, `.claude/skills/`, `.pi/`, `skills-lock.json`; resolve the tracked `skills/archify` symlink to untracked content.
- **Q4 #293 frozen promoted pages:** doctor-only report with the explicit-promote / pin remedy.
- **Q5 #312 registry/engine coupling:** the skills domain reads its own registry model through a port; the YAML file is an adapter; tolerant reader with must-understand fields; versioned adapters for format changes.
- **Q6 profile retirement:** snapshot the resolved profile into the workflow log at creation; workflows are self-contained; the catalog only serves new workflows.
- **Q7 runtime adapters:** archive `runtime-roster` as superseded; implement R-141 as an `Adapter` port with a self-registering adapter registry; an unregistered target is an error.
- **Q8 project identity:** a `ProjectIdentity` port with ordered adapters (explicit `--project-id`, origin remote read from `.git/config` without running git, directory name); a verb to declare project-scoped skills (R-046/R-047).
- **Q9 open SDD changes:** archive `overlay-versioned-releases`; finish `longterm-mem-knowledge-ingestion` as an adapter behind an ingestion port, before the skills documentation.
- **Q10 orphan branches:** delete `pr137`, `feat/archive-reconcile-guard`, `fix/ci-full-history-for-engine-tests` (superseded on main); keep `wip/audit-remediation-snapshot` (review it) and `sdd/shared-project-vault` (input for the vault work).
- **Q11 additions:** `.gitattributes` `skills/** -text`; a deadline on the Pi subprocess through an injected `CommandRunner` port; a real `memory:procedural-skills` signal through a project-context port; a guarded sandbox test of the Pi global tier. OpenCode `skills` stays `partial`.
- **Q12 goal-v2 open question:** close as superseded.
- **Vault (user):** the skills analysis lives in the project vault `~/labdrian-brain` (registered in `~/.labdrian-overlay/vaults.json`): one page per skill and per role, comparisons, open questions; research and thinking happen there; the new skills are designed from that knowledge. The user's uncommitted `.obsidian/` changes are never touched.

## Tasks

Wave 0 — hygiene (each GitHub or destructive step asks for approval at the time):
- [x] C1 — Close delivered issues #366–#371 and #297 with evidence.
- [x] C2 — Prune worktrees and merged branches; delete the three superseded orphan branches (Q10).
- [x] C3 — `.gitignore` the tool directories; resolve the `skills/archify` symlink; `.gitattributes` `skills/** -text` (Q3, Q11-1). Done in `887a519`.
- [x] C3b — Fix the Codex skill frontmatter outside the repository (needs explicit authorization for `~/.codex`).
- [x] C9 — Stale-text sweep (roadmap and ledgers, canonical spec, superseded notes, Q12); close #309. Text done in `06bbea2`; closing #309 is a GitHub step that still waits for approval.

Wave 1 — architecture:
- [x] C17 — Hexagonal alignment audit (read-only): map every engine and longterm-mem package to domain / port / adapter, list each boundary violation (domain code that knows files, processes, formats, or concrete runtimes), and propose the order of fixes; the user approves before any refactor.

Wave 2 — engine, hooks, ports:
- [ ] C4 — `synctrigger` platform split; `GOOS=windows` build green.
- [ ] C5 — `synctrigger` runner hardening (#301).
- [ ] C6 — Hook recognizer consolidation and identity tightening.
- [ ] C7 — Quote the binary path in every hook family; in-place upgrade of installed entries (Q1).
- [ ] C18 — Runtime `Adapter` port with self-registering registry (Q7); archive `runtime-roster`.
- [ ] C19 — Registry port, YAML adapter, tolerant reader, must-understand fields, versioned adapters (Q5); closes #312.
- [ ] C20 — Profile snapshot in the workflow log (Q6).
- [ ] C21 — `ProjectIdentity` port and adapters; project-scoped declaration verb (Q8).
- [ ] C8 — `skills uninstall`; `project-status` lists install records.
- [ ] C10 — longterm-mem: `doctor` report for frozen pages (Q4); detector status for candidates without an Engram record; the deploy path rebuilds longterm-mem; Pi `CommandRunner` with deadline; procedural-skills presence through a project-context port; guarded Pi global-tier test (Q11).

Wave 3 — specs and ingestion:
- [ ] C11 — Archive and spec-sync the four skill aliases with renumbering; archive `overlay-versioned-releases`.
- [ ] C22 — Finish `longterm-mem-knowledge-ingestion` as an adapter behind an ingestion port (Q9).

Wave 4 — skills for the sub-agent team (last):
- [ ] C12 — Vault documentation in `~/labdrian-brain` (one page per skill and per role, comparisons, open questions), ingested through C22; research and thinking in the vault; a per-role design the user approves before any rewrite.
- [ ] C13 — Rewrite the custom inception family.
- [ ] C14 — Rewrite the remaining custom planning skills.
- [ ] C15 — New overlay-owned role skills derived from the core skills (Q2), one PR per role.
- [ ] C16 — Every new or rewritten skill passes lint, carries an approval record, and leaves the baseline.

## Scope and constraints

- Hexagonal principle governs every task. TDD strict (`go test` from `engine/` and `longterm-mem/`). RDD on. One slice at a time. Candidates under about 150 KB.
- Tests never touch real state. Destructive, GitHub, deploy, vault-write, and `~/.codex` steps each need explicit approval at the time.
- Ultracode: the user opted into multi-agent workflows for this phase.

## Unclear (investigate inside the tasks)

Per-class degradation warnings (sources disagree); the Phase 8 non-blocking findings triage; branch counts; whether the gentle-ai sync overwrites `~/.claude/skills`; `wip/audit-remediation-snapshot` content vs main.

## Progress and evidence

- Inventory (Engram #3855); decisions Q1–Q12 and the principle (this ledger; Engram #3856).
- **C1** (user OK 2026-10-01): closed #366–#371 (delivered by PRs #372–#377, merged 2026-09-21 through tracker #379 into main) and #297 (delivered by `1495b21`, in main), each with an evidence comment.
- **C2** (user OK 2026-10-01): removed 5 worktrees (goal-v2; three `/tmp/labdrian-goal-*`; review-workflow-profile, whose untracked ledger was an older version of the one on main) and pruned 3 missing ones; kept `labdrian-standalone` and the gentle-ai candidate view. Deleted 90 local branches (every commit verified in main with `git cherry`, plus the three Q10 orphans); kept `main`, `chore/phase9-hygiene`, `sdd/shared-project-vault`, `wip/audit-remediation-snapshot`, `wip/standalone-platform`. Deleted 3 remote branches (`fix/pre-sdd-entry-contract-assets` and `union/pr1b-merge`, contained in main; `feat/archive-reconcile-guard`, Q10).
- **Mode and routes.** TDD strict, runner `go test` from `engine/`, resolved from the project configuration (this ledger). C3 and C9 ran as one delegated writer (each touches 2 or more non-trivial files) on branch `chore/phase9-hygiene` in the `~/labdrian-sdd-overlay-shaper` worktree, from `main` `f19a33c`. The commits are unpushed.
- **C3** — `887a519` `chore(repo): ignore tool installs and keep skill bytes exact`.
  - `.gitignore` gains `/.agents/`, `/.claude/skills/`, `/.pi/`, and `/skills-lock.json` (anchored at the repository root), plus `/skills/archify`. `skills/archify`, the only tracked symlink, is untracked with `git rm --cached`. `.gitattributes` is new, with `skills/** -text`.
  - Test `skills:TestSkillFilesKeepTheirExactBytesUnderLineEndingConversion` pins the rule by what git does under `core.autocrlf` in a throwaway repository, with a control file outside `skills/` that proves the conversion is active. RED observed twice (no `.gitattributes`; then a `.gitattributes` without the rule, where git rewrote the CRLF skill to LF), GREEN with the rule.
  - Archify dependency check: `rg -n archify` over tracked and untracked files found only a synthetic tar fixture (`engine/pipkg/extract_tar_internal_test.go`), two example comments (`engine/pipkg/pipkg.go:628`, `engine/skills/project_register_test.go:362`), and archived historical docs. Nothing in `bin/`, `overlay.manifest`, `skills.registry.yaml`, the README, `docs/`, `tools/`, `tui/`, `agents/`, `pi/`, or `opencode/` depends on the link being tracked.
  - Checks: with throwaway installer output in the worktree, `git status --short --ignored` and `git check-ignore -v` showed the four paths and `skills/archify` ignored; `.claude/settings.json` stays tracked; `git ls-files -ci --exclude-standard` prints nothing. `skills validate` exits 0 (37 skills, 77 files, 37 grandfathered) before the change, with the local link resolving, with it dangling, and in a clean clone of the branch where `skills/archify` is absent.
- **C9** — `06bbea2` `docs: sweep stale text after Phases 7 and 8`.
  - Roadmap Phase 7 and Phase 8 rows: the "not deployed" text and the "Next action" cells are corrected; the evidence in the rows is untouched.
  - `odd/tasks/runtime-adapters.md`: the install-hooks summary backlog item is marked done in `28d2082`. The "26 commits" figure was recounted from the ledger's own commit lists (25 commits in slices S1 to S8, plus the RA9 test-comment commit `bf55b52`) and is exact, so it was left.
  - `openspec/specs/procedural-skill-registration/spec.md`: the global-promotion requirement and its heading now say that the global tier has the Phase 8 approval record and the project tier stays autonomous. No test pins this spec text; the section 12 pin on the contract was already current.
  - Superseded notes: the sync-triggers proposal (replaced by its `design.md` and the delivered verb `sync-trigger`; PRs #299, #300, #302; issue #301) and the pi-runtime-target proposal and apply-progress (replaced by design corrections A1 to A5 and five slices, PRs #308, #310, #311, #313, #314; issue #309).
  - `odd/tasks/goal-v2-identity.md`: the next step is closed as superseded (Q12), citing the Shaper handoff contract, Phase 6 git-free provenance, and the Phase 9 `ProjectIdentity` port.
- **Checks** (from `engine/`, after each commit): `gofmt -l .` clean, `go vet ./...` ok, `go test -count=1 ./...` 26 packages ok. The CI archive guards (`tools/archive-anchor-gate`, `tools/archive-reconcile`) run locally against the worktree exit 0.

- **C3b** (user authorization for `~/.codex`, 2026-10-01): `~/.codex/skills/software-architect-consultor-senior-de-arquitectura-y-diseno/SKILL.md` had no front matter at all (it started with a Markdown title), so Codex could not read its name or description. Backed up to `~/.labdrian-phase9-backup-20261001T024838/SKILL.md`; prepended a front matter with `name` (the directory name) and a one-line Spanish `description` matching the skill's own trigger section; the body is byte-identical to the backup (`cmp`). The file is outside the repository and outside labdrian's registry, so the overlay lint's `license`/`metadata` requirements do not apply to it.
- **Wave 0 native review** — lineage `review-f617c59cd19bbe12` over `f19a33c..ad5bb45` (high; the process evidence is a comment in `engine/pipkg/pipkg.go`), granted, four lenses; approved first pass, acknowledged. Folded into the first Wave 2 commit: `R3-env-override` (the `.gitattributes` test appends git environment variables instead of replacing existing ones), `R4-1` (that test runs git with no deadline and no `GIT_TERMINAL_PROMPT=0`). Fixed here: `R2-next-step-stale-wave0` (this section's next step). Cleanup: the remaining suggestions.

- **C17** — read-only hexagonal alignment audit (ultracode `wf_2e59e99b-d33`): 51 packages, 156 claimed violations, each challenged by an adversarial reviewer; the confirmed ones grouped into about 60 work units in phases A–I. Full target architecture, work units, ordering, and the user's decisions D1–D6 in `docs/architecture/hexagonal-target.md` (Engram #3863). Three real bugs found: T1 (the TUI sends `--target all` for its three targets and the backend also acts on Pi), H3 (an unbounded `syscall.Flock` at `engine/cmd/main.go:917`), H23 (`--config-root` ignored for Pi). Decisions: D1 adapters in a subpackage under their domain; D2 a small pure `identity` module shared by engine and longterm-mem; D3 explicit `Register(r *Registry)` per runtime called from cmd, no `init()` globals (the user first answered A and corrected to B when the conflict with the principle was pointed out); D4 accept the three strictness changes; D5 the backend `targets` subcommand is the single source for the TUI; D6 migrate all of B1–B4.
- **Task mapping.** The Wave 2–4 tasks are now executed as the C17 work units: C18 = H23, C19 = H15 + H16, C20 = H32 + H33, C21 = H18 (with D2), C10 Pi runner = H24, C22 = L11; C4–C8 and the C10 doctor/detector/deploy items stay as listed; L3 (vault repository port) precedes C12. Order: T1/T2 and Phase A first, then B, C, D, E, F, G, H, I, then Wave 4.

- **T1/T2** (branch `fix/tui-target-catalog`, delegated writer): `8dc9e94` the backend `targets` subcommand from one `TARGET_CATALOG` array (`<name><TAB><kind>`, kind `copy` or `package`), from which `ALL_TARGETS`, `TARGET_KINDS`, and the `all` expansion derive; `474234f` the `TargetCatalog` port and its adapter; `19e7d46` the TUI holds no target list, sends `--target all` only when the selection is exactly the catalog it showed (re-checked right before the call) and otherwise runs per target, and fails closed when the catalog cannot be read; Pi is now visible, and capture/restore skip it (`Action.CopyTargetsOnly`); `dc951eb` the `BackupQuery` port answered by `restore --list`, so the TUI follows `STATE_DIR`. Native review lineage `review-7d73a5ab97fabf0a` (high): one real blocker (R3/R4: the restore confirm screen queried the backend synchronously inside bubbletea's Update, freezing the UI up to N × 15 s); correction `daf30a8` moves the lookup into a command with a "Consultando respaldos…" screen, stale-result guard, and quit while in flight (RED: the old code blocked until the test's timeout); validator approved; acknowledged. Follow-ups for T3: a backup query error looks like "no backups"; the `all` re-check adds one backend round trip.
- **Phase A** (branch `refactor/phase9-arch-guard`, delegated writer): `8ce63b6` H1 architecture fitness test (ring table per module; 29 engine and 25 longterm-mem known-debt edges, each tagged with the C17 unit that removes it; new violations, unringed packages, and stale debt fail); `f798f40` the checker extracted into the `archguard` module (user decision D7; standard library only; engine and longterm-mem depend on it from their tests through a local `replace`; CI job `test-archguard`); `dc78f9b` H2a `atomicfile` and `d15a1a9` H2b `statestore` (the strictest semantics of the existing copies: fsync of file and directory, exact mode set before writing, atomic backup with the original mode, symlink refusal, bounded reads); `822e8cd` H3a flock as a locker field with `NoWait` and `Perm`; `9d7bb92` H3b the propagate registry lock bounded at 2 s (RED: the old wait hung past the test's 10 s bound); `be77e86` the propagate spec R-005 states the bound. Native review lineage `review-4de8f2fb1e03ca00` (high), approved first pass, acknowledged. Follow-ups for the first Phase B commit: the backup check/read window in `atomicfile` (`Lstat` then `ReadFile`), a dead `Support` row in `archguard`, `AcquireDir` validating `Perm`, an end-to-end test of the bounded propagate lock, `statestore` panicking on empty `parts`.

- **Phase B batch 1** (branch `refactor/phase9-b1`, delegated writer): `d1d3eb7` the five Phase A review follow-ups (the `atomicfile` backup now reads through the same no-follow descriptor it checked; dead `archguard` `Support` row removed; `filelock.AcquireDir` no longer validates `Perm`; end-to-end test of the bounded propagate lock; `statestore` returns a typed error for an empty chain); `108464b` H4 (`shaper` owns `WorktreeProvenance`; `cmd` maps `gitprov.Observation`); `93e9c72` H5 (`EventLog` and `ProfileCatalog` ports injected into the workflow Lifecycle; two real bugs found and fixed RED-first: `Create` read the role chain after the resolver refused the profile, and `RecordStage` ignored the resolver's stage order); `f153604` H11 (`PrespecCore` moved to `cmd/prespec.go`; clock and entropy injected). Architecture debt: 29 → 26 engine lines (H4 and H11 removed theirs; H5 removes none because the workflow edges come from the store files, which move in H6). Parent decisions on the writer's forks: validate `filelock` `Perm` only when creating the file in exclusive mode, and make the `atomicfile` backup fail closed on platforms without a no-follow open — both in batch 2. Native review lineage `review-a20e1b5b95aa8475` (high), approved first pass, acknowledged; its two warnings are the non-unix fallback, already scheduled.

## Next step

Deliver batch 1 (user approved 2026-10-01); then Phase B batch 2: the batch 1 follow-ups, H6 (workflow store to `workflow/filelog`), H7 (roles ChainStore to `roles/filechain`).
