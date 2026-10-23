# Role agents — v3 slice 5: retire the superseded agents and GADU (Phase 9, PR 5 of `docs/architecture/role-design/per-role-design-v3.md` §6)

Objective: delete the agents and skills the eight role agents replace, together with their manifest rows, and every consumer that names them. Branch `feat/phase9-retire-agents` from `main` (`fd41c0c0`). Delivery: `ask-on-risk`, one PR, work-unit commits per task. Runners: `cd engine && go test ./...`, CI-form `skills validate`, `gadu-generate --check`, and a differential of `labdrian apply --target <t>` against a base copy in an isolated HOME.

## Specs

Read v3 in full (D5, D10, §2, §4, §5, §6, §9), v2 §1-§2 and §5, and `odd/tasks/role-agents-slice4.md` before any edit.

- **S1** — v3 §6 row 5, verbatim: "Deletion: `agents/sdd-*.md`, repo copies of `review-*`/`jd-*` (byte-identical to gentle's; rows + files together, R §7), `agents/GADU.md`, `opencode/agents/GADU.md`, `pi/agents/GADU.md`, `skills/gadu-operator`, `gadu-orchestrate` (or keep as orchestrator-only reference, v2 OD4), GADU engine persona and README mentions; `sdd-phase-common.md` shrunk to §B-D". Risk named there, verbatim: "item 3 (manifest rows and names others consume), item 5, item 1 (uninstall of the Pi link)". §6 trailer, verbatim: "differential check of `labdrian apply --target <t>` output against the base copy for PR 4-5".
- **S2** — OD4 is resolved by the owner (L3): `skills/gadu-orchestrate` is retired, deleted like `gadu-operator`.
- **S3** — Consumers (memory `consumer-impact-rule`):
  - Enumerate every reader of each deleted name and path: skills, the registry, the manifest, engine code and tests, routing pins, capability declarations, the Pi package build, CI, README and docs, and the orchestrator and skill text that launches `sdd-*` agents.
  - Each reader is updated to the role agents or the deletion, or is listed with a reason if it must stay.
  - The list goes in the Log with path:line evidence.
- **S4** — Deployed copies and the Pi link (v3 item 1):
  - State what `labdrian apply` does with a deployed file whose manifest row is gone, for each target: `~/.claude/agents/sdd-*.md`, `GADU.md`, the OpenCode GADU, and the Pi package plus the `~/.pi/agent/agents/GADU.md` link.
  - After `apply --target pi`, no overlay-owned link may point to a file the package no longer ships. The ownership proof is the existing Readlink equality.
  - A file the overlay does not own is never removed or rewritten. That covers gentle-ai's own `review-*`, `jd-*` and `gentle-ai-*` agents.
  - Any removal of deployed files is shown in the differential and stated.
- **S5** — Engine persona: GADU's projection leaves `engine/agentgen` (the persona, its constants, its artifacts and its pins). The eight roles' outputs stay byte-identical. The CLI command name `gadu-generate` is not renamed in this slice: no spec asks for it. Report it as a follow-up.
- **S6** — Global skills:
  - Deleted skills leave the registry and the manifest through the repository's own retire or removal path. Find that path; do not hand-edit around it.
  - Every remaining global skill whose bytes change needs the owner's `labdrian skills approve`, which an agent cannot run. List those ids as `needs_owner`.
- **S7** — Safety:
  - Tests never touch a live Pi, Codex or Claude install (memory `tests-touch-live-pi`). They use a temp HOME, STATE_DIR and GOCACHE, with `LABDRIAN_PI_BIN` set.
  - The live deploy of this PR is a separate owner-authorized step after merge.
- **S8** — Scope fence:
  - No CI ratchets (PR 6).
  - No change to the eight role bodies or their generated outputs.
  - No change to the gentle review lifecycle.
  - No new agents.

## Tasks

| ID | Specs | Route | Status | Commit / evidence |
|---|---|---|---|---|
| T1 | S1, S3, S4, S5, S7, S8 | delegated workflow (owner, L1: "sonnet max para implementaciones"); agents and GADU: files, rows, engine persona, the Pi link, consumers | done, verified (L6), reviewed (L7) | `b434cb45`, `715cbc92`, `71aec8a5` |
| T2 | S1, S2, S3, S6, S8 | same workflow, after T1; skills: `gadu-operator`, `gadu-orchestrate`, `sdd-phase-common.md` shrunk to §B-D, registry, consumers | done, verified (L6), reviewed (L7); approved by the owner (L8) | `b02603f9`, approvals in L8 |
| T3 | S4 (live) | owner-authorized after merge: `labdrian apply` on the real targets | pending | — |
| T4 | slice 4 carry | record slice 4's T3 live check in `odd/tasks/role-agents-slice4.md` | done | first commit of the branch |

## Log

- **L1** (2026-10-21, owner): "bueno cotinua en ese orden ve en ultracode con opus high para las verify y sonnet max para implementaciones".
- **L2** (2026-10-23, owner): after the slice 4 live check I asked "¿Borro el worktree `role-agents-s4` y arranco el PR 5, el de las eliminaciones?" Owner: "si porfa".
- **L3** (2026-10-23, owner): I asked "En el PR 5, ¿qué hacemos con `skills/gadu-orchestrate` (v2 OD4)?" Owner: "Retirarlo (Recomendado)".
- **L4** (2026-10-23, fix round for verifier blockers B1 and B2): S3 consumer map. Method: rg over the worktree (HEAD `922f50a6` plus the slice diff), history docs excluded. Every path:line below was re-read in this round.
  - Names deleted: `agents/sdd-{apply,archive,design,init,onboard,propose,spec,tasks,verify}.md` (9; `agents/sdd-explore.md` never existed), `agents/review-{readability,refuter,reliability,resilience,risk}.md`, `agents/jd-{judge-a,judge-b,fix-agent}.md`, `agents/GADU.md`, `opencode/agents/GADU.md`, `pi/agents/GADU.md`, `skills/gadu-operator`, `skills/gadu-orchestrate`, the `engine/agentgen` GADU projection, `engine/runtime/pi_gadu_frontmatter_test.go`.
  - UPDATED (the diff is the evidence; paths in T1/T2):
    - `overlay.manifest` rows, `skills.registry.yaml` and the approval baseline (`engine/skills/approval_*.go`).
    - The Pi package build (`engine/pipkg/{build,comparison}.go`) and runtime (`engine/runtime/pi*.go`).
    - The CLI (`engine/cmd/{agentgen,pipkg,runtime_registry,usage}.go`) and launcher text (`bin/lib/{bootstrap,usage,pi_package}.sh`).
    - `README.md`.
    - The section A pointer of eight sdd-* skills (for example `skills/sdd-apply/SKILL.md:68`), `skills/_shared/sdd-phase-common.md` (shrunk to sections B-D) and `skills/_shared/sdd-orchestrator-workflow.md:296`.
    - Nothing in `engine/capability` names these agents.
    - Routing pins read no real row: `engine/installer/routing_pins_capture_test.go:92` writes a synthetic `pinManifest`.
  - STAYS (1), the launch text of the sdd-* phases:
    - Readers:
      - `skills/sdd-{apply,archive,design,explore,init,propose,spec,tasks}/SKILL.md:15,17,18`. The text is "the dedicated `sdd-<phase>` sub-agent"; `:18` says "delegate to the dedicated `sdd-<phase>` sub-agent using your platform's delegation primitive".
      - `skills/sdd-verify/SKILL.md:15` and `skills/sdd-onboard/SKILL.md:13-15,19`.
      - `skills/_shared/sdd-orchestrator-workflow.md:93-334`: phase launch rules at `:95`, `:105-109`, `:138-146`, `:249-262`, `:313-334`.
      - `skills/inception-pipeline/SKILL.md:121,127-130`. `:132` says it "never routes the phases itself".
    - Reasons:
      - (a) The text names the phase and its skill id (`engine/sddregistry/sddregistry.go:43-53`), not an agent file. The orchestrator launches by passing exact `SKILL.md` paths (`sdd-orchestrator-workflow.md:292-294`), and no file under `skills/` names an `agents/<name>.md` path. `sdd-explore` never had an agent file at HEAD (9 sdd-* files, no explore) and `skills/sdd-explore/SKILL.md:15,18` carries the identical text. So the deletion changes nothing this text relies on.
      - (b) The sdd-* skills, `sdd-orchestrator-workflow.md` and `sdd-phase-common.md` are `managed` upstream rows (`overlay.manifest:62-64,79-80,95-104,115-116`). Owner decision Q2 is "leave the 15 upstream core skills untouched" (`odd/tasks/consolidation-phase9.md:17`). The slice edits them only where S1 forces it.
      - (c) Rewording would add `labdrian skills approve` for `sdd-init`, `sdd-onboard` and `inception-pipeline` to the eight ids already needed (S6).
      - (d) No spec re-points these launches. `sdd-init` and `sdd-onboard` map to no role (`docs/architecture/role-design/per-role-design-v2.md:43`). `sdd-verify` to reviewer is a semantic change (`per-role-design-draft.md:213`). `skills/role-chain/SKILL.md` carries no SDD phase mapping.
    - Not a reason, and not used: "gentle-ai deploys the sdd-* agents itself". gentle-ai 4.2.0 retires them (Engram #4406; binary symbols `internal/cli.retiredSDDAgentsStep` and `legacyassets.OwnsRetiredSDDAgent`; 0 hits for the executor body text).
  - STAYS (2), `skills/judgment-day/SKILL.md:20` ("Never launch `review-refuter`"):
    - It is a prohibition, not a launch target.
    - The agent still exists at runtime: gentle-ai 4.2.0 embeds `claude/agents/review-refuter.md` among its eight Claude agents (review-* x5, jd-* x3).
    - The judges and the fix actor are already `polisher` and `builder` (`:16-18,21`).
  - STAYS (3), gate scope:
    - `skills/_shared/oo-quality-contract.md:2` (`[sdd-design, sdd-tasks, sdd-apply]`), `skills/_shared/anti-generic-design.md:2` and `engine/assets/anti-generic-design.md:2` (`[sdd-tasks, sdd-apply]`).
    - The gate injects a contract only when the launch's `subagent_type` is in that list (`engine/gate/gate.go:127-135`).
    - Slice 1 gave `minimalism-contract.md:2` `shaper, builder` (`per-role-design-v3.md:89`) and nothing equivalent exists for these two. After this slice no launch carries a type that triggers them.
    - Left unchanged: which roles receive them is a scope decision no spec makes (both are `custom` rows, `overlay.manifest:126-127`). Open for the owner.
  - STAYS (4), `engine/pipkg/labdrian-gate.ts:28-30` (`PHASE_NAMES = ["sdd-tasks", "sdd-apply"]`):
    - By its own comment this is the exhaustive set of agents the Pi gate touches, and every other name is a no-op.
    - The Pi package never shipped an sdd-* agent (`pi/agents/` at HEAD held GADU.md and the eight roles), so the list is inert after this slice. It also never listed `shaper` or `builder`.
    - Decide together with (3).
  - STAYS (5), tests that must keep naming the deleted agents:
    - S4 foreign-file proofs (gentle-ai's `review-*`, `jd-*` and `gentle-ai-*` that apply must never remove): `engine/runtime/pi_agents_test.go`, `engine/cmd/pipkg_test.go`, `engine/shelltest/overlay_pi_package_build_test.go`.
    - Retirement pins: `engine/skills/role_agents_artifact_test.go:575-576` and `engine/skills/gadu_skills_retired_test.go`.
    - Synthetic fixtures that write their own `overlay.manifest` into a temp overlay and use no repo-root helper: `engine/skills/ondisk_test.go:14-27`, `engine/installer/{routing_pins*,manifest_routes,overlay_sandbox,tar_listing,tar_paths}_test.go` with `testdata/routing_pins.golden`, `engine/skills/{routes,sync}_test.go`, `engine/skills/app/routes_test.go`, `engine/cmd/manifest_test.go`, `engine/pipkg/{pipkg_test.go,gitsource/gitsource_test.go}`. The suite is green after the deletion.
  - STAYS (6), the gentle review lifecycle (S8): `engine/reviewreceipt/{parse_test.go,receipttest/receipttest.go}` and `tools/archive-anchor-gate/**` use `review-risk` and the other lenses as receipt ids.
  - STAYS (7), the CLI name `gadu-generate` (S5; rename is a follow-up): `bin/labdrian-overlay:104`, `bin/lib/engine_verbs.sh:88-92`, `bin/lib/usage.sh:270-273`, `.github/workflows/ci.yml:138`, `engine/cmd/agentgen.go:30`, `README.md:296,419`.
  - History, not rewritten: `docs/e1-durability-probe.md`, `docs/architecture/role-design/*`, `odd/tasks/*`, `odd/review-receipts/*`, `openspec/changes/archive/*`, `CHANGELOG.md:19`.
  - Updated by the parent outside the writer's surfaces: `.github/workflows/ci.yml:132` (comment only) and `CHANGELOG.md` (Unreleased entries). Follow-up, cosmetic: the `tui/` sample reports that use `agents/GADU.md` as their example path (`tui/internal/domain/syncreport.go:5`, `tui/internal/ui/model_test.go:1024,1032-1033`, `tui/internal/app/actionrunner_test.go:424,450`, `tui/reportmerge_test.go:77,106`, `tui/pin_test.go:236`, three `tui/testdata/**/*sync_check_dashboard.golden`). Cosmetic. `docs/architecture/hexagonal-target.md:41` is stale since slice 4.
- **L5** (2026-10-23, verifier blocker B1 on S4): the Pi link of a dropped agent.
  - `apply --target pi` rebuilds the package first (`bin/lib/pi_package.sh:29`, `engine pipkg build`, which swaps the package wholesale) and runs `pi install` only when `pi` is on PATH (`:44`).
  - The first version retired `~/.pi/agent/agents/GADU.md` only after `pi install`. With `pi` absent or `pi install` failing, the link stayed and pointed at nothing (reproduced in an isolated HOME with a stub `pi`).
  - Fix: `RetireDroppedPiAgentLinks` (`engine/runtime/pi_agents.go`) runs right after the package build in the Pi adapter's Install, Update and Rollback (`engine/runtime/pi.go`, `buildPackage`) and in `engine pipkg build` (`engine/cmd/pipkg.go`).
  - Ownership is unchanged: Readlink equality with `<package>/agents/<file>`. A file, or a link to any other path (gentle-ai's `review-*`, `jd-*`, `gentle-ai-*`), is never removed or rewritten. `status --target pi` now names an owned link the package no longer ships.
  - Evidence:
    - Five runtime tests, one cmd test and one shelltest, each seen red before the code. Three more cmd tests pin the previous behaviour.
    - Four mutants (adapter retire, `pipkg build` retire, the two Status branches) killed.
    - In the isolated-HOME differential, base against fixed differs only in the removed `GADU.md` link, package file and two gadu skills, and in the words "required for Pi subagent dispatch". `status` and the pi stub log are identical.
    - The upgrade run prints `pipkg build: GADU.md link removed (the package no longer ships it).` and leaves the three foreign files unchanged.
- **L6** (2026-10-23): workflow `wf_bf77b523-436` ran a Sonnet max writer and two Opus high verifiers (spec and adversarial), with one correction and a recheck.
  - **Blockers:** B1 (S4, a dangling Pi link when `pi` is absent or `pi install` fails) was fixed in the correction (L5); recheck pass. B1/B2 (S3, the sdd-* launch text was not listed) were answered by listing it with reasons (L4, STAYS 1); recheck pass on content. The parent wrote the Log entries.
  - **Checks:** `go test ./...` passes 69 packages. The only failures are the 3 known environmental tests and the 2 `engine/skills` tests that wait for the approvals. `gadu-generate --check` OK. CI-form `skills validate` exits 1 with exactly 8 `APPROVAL_MISSING`.
  - **needs_owner:** the owner's approval record for sdd-apply, sdd-archive, sdd-design, sdd-explore, sdd-propose, sdd-spec, sdd-tasks and sdd-verify. Their bytes changed when section A left phase-common.
  - **Open for the owner, not blocking:**
    1. The sdd-* launch text: keep the listing (done) or reword it to roles.
    2. The gate scope of `oo-quality-contract` and `anti-generic-design` (and `labdrian-gate.ts:30`). No launch triggers them after this slice.
    3. The link retire lives inside `pipkg build` rather than in a separate verb.
  - **Follow-ups:**
    - Rename `gadu-generate` (S5).
- **L7** (2026-10-23): native review.
  - **Split:** T1 as one commit was refused with `lens_context_budget_exceeded`, so it was rebuilt as three commits with the same final tree (checked with `git diff` against the old tip: empty).
    - `b434cb45`: the repository copies.
    - `715cbc92`: the GADU persona and its Claude and OpenCode files.
    - `71aec8a5`: the Pi GADU and the link retire.
  - **Reviews.** All four were approved and acknowledged, with receipts in `odd/review-receipts/role-agents-slice5/`. None had a blocker.
    - `review-14c06b24df495831` covers `b434cb45` (risk, reliability).
    - `review-c66bc0978f2f2235` covers `715cbc92` (risk, readability, reliability).
    - `review-c5f49d0cd8156bab` covers `71aec8a5` (all four lenses).
    - `review-4252a2828a1da5aa` covers T2, `b02603f9` (risk, readability, reliability).
  - **Dispositions:**
    - **A R3-001 and B R3-001** (GADU, then `pi/agents`, not covered by the retirement test): these are artifacts of the split. `71aec8a5` restores both, and the final test scans `agents/`, `pi/agents/` and `opencode/agents/` for GADU too.
    - **A R3-002** (deployed retired agents stay in `~/.claude/agents` and the OpenCode agents dir, because apply never removes a file whose row is gone): pinned by `engine/installer/agent_deploy_test.go` as the current behaviour (S4). Whether to clean them is open for the owner.
    - **B R2** (the `piSonnetModel` comment framed the bridge's prompt limit as GADU-only): reworded, with the measured role file sizes (largest about 2 KB). **B R2** (the verb name): follow-up (S5). **B R3-002:** informational.
    - **C R2** (two note formats): the listing-error note keeps slice 4's `agents link: ...` form, the one Uninstall uses. No change.
    - **C R3** (`pipkg build` reads `$HOME`): `engine/cmd/usage.go:51-53` documents it, and every runtime verb takes the home from the environment. No change.
    - **T2 R2-001 and R3** (sections E and F removed): v3 row 5 asks for "§B-D". The Review Workload Guard lives in `skills/_shared/sdd-orchestrator-workflow.md:247`, and no skill names phase-common's E or F.
    - **T2 R2-002** (the registry check matched lines): moved to `TestTheGaduSkillsAreGoneFromTheRegistry` in the external test package, which reads the registry with the real reader. Red against the pre-T2 registry, green now.
    - **T2 R3** (validate red until the approvals): needs_owner (L6).
    - The `tui/` sample paths.
    - `docs/architecture/hexagonal-target.md:41`.
- **L8** (2026-10-23): the owner ran `~/rc.sh` and approved sdd-apply, sdd-archive, sdd-design, sdd-explore, sdd-propose, sdd-spec, sdd-tasks and sdd-verify (8 `.approval.json` records, committed). `labdrian skills validate`: registry and manifest aligned (38 skills), skills/ matches the manifest (77 files), approvals verified (17 approved, 21 grandfathered). TestApprovalBaseline_PinnedToTheRepositoryRegistry and TestRegistryLockE2E_ValidateLeavesTheRealTreeUntouched now pass. T2 R3 resolved. Next: the owner decisions, then push and PR.
