# Per-role design for labdrian's sub-agent roles — DRAFT for owner approval

Scope: Phase 9, Wave 4, item C12 (`odd/tasks/consolidation-phase9.md:90`): "a per-role design the user approves before any rewrite". Governing decisions: Q2, new overlay-owned role skills, 15 upstream core skills untouched (`consolidation-phase9.md:17`); every rewritten skill passes lint and carries an approval record (`consolidation-phase9.md:94`); roadmap Phase 9 clause (1) (`openspec/decisions/standalone-platform-roadmap.md:39`).
All paths are relative to the repository root. "Assumption" marks what the code does not establish.

## 1. What the code gives us today

- Closed vocabulary of eight roles in canonical order; sweeper and polisher optional (`engine/roles/roles.go:17-26`, `:114-117`). Forward transitions may skip only optional roles; reviewer → builder is the only backward transition; delivery is terminal (`roles.go:131-151`).
- Roles are data only: purpose, consumes, produces; nothing launches or dispatches (`roles.go:1-6`, `:40-48`). No per-role prompt, agent mapping, or startup text exists anywhere in `engine/`.
- `RoleHandoff` v1/v2: `payload_kind` + `payload_sha256`, `evidence[]{kind, ref, sha256}`, `context{summary, decisions, open_questions}`, `status` completed|interrupted, `resume_reason` required when interrupted, optional `memory_directive` in v2 (`engine/roles/handoff.go:26-34`, `:39-51`, `:57-78`).
- `payload_kind` and `evidence[].kind` are free non-blank strings; the engine enforces no vocabulary (`engine/roles/handoff_validate.go:79`, `:91`).
- Runtime route into a sub-agent: the PreToolUse `Agent` hook rewrites the prompt by `subagent_type` (`engine/gate/gate.go:79-80`, `:98-100`) and, while a workflow is bound, appends the memory-plan block to every sub-agent prompt (`docs/architecture/adr-c23-m6-plan-instructions.md:39-41`). Consequence: role prefixes must not restate memory rules; the hook injects them.
- Skill lint budget: body hard cap 1000 estimated tokens, recommended 700, description ≤ 250 runes (`engine/skills/lint.go:38-44`). The 27 failing baseline skills are exempt only for three legacy rules until rewritten (`roadmap.md:38`, Phase 8 "Not done").
- Current agents: 9 `sdd-*` (each "read the skill at `~/.claude/skills/<phase>/SKILL.md` and follow it exactly" plus a result contract, `agents/sdd-apply.md:11-49`), 5 `review-*` with `tools: []` and provider-injected immutable context (`agents/review-risk.md:5`, `:14`, `:41-43`), 3 `jd-*` (`agents/jd-judge-a.md:16-36`, `agents/jd-fix-agent.md:14-33`), and `GADU` with `tools: '*'` (`agents/GADU.md:5`). Every agent carries ~50 lines of generated blocks (CodeGraph, language contract, remote authorization: `agents/sdd-apply.md:51-99`). `sdd-explore` has a skill but no agent (`skills/sdd-explore/SKILL.md`; none in `agents/`).

## 2. Cross-cutting design

**One agent per role, named by the role.** `agents/<role>.md` with `name: <role>`, so `subagent_type` equals the role string and the gate can route per role without a new mapping table. Assumption: the gate's contract lookup accepts any `subagent_type`; only the generic path was read (`gate.go:98-100`).

**Startup prefix = minimal rules, never persona, never the global instruction set** (owner decision 5). Each prefix below is ≤ 15 lines, states: identity line, input, output, write surface, tool floor/ceiling, interruption rule, language. Everything procedural lives in the role skill the prefix names, which the agent reads only when it needs the procedure.

**Handoff mapping.** Each role produces exactly one payload kind and consumes the previous role's. Proposed closed vocabulary (convention now; see open decision 2 for enforcing it in `roles.Catalog`):

| from → to | payload_kind | evidence kinds typically attached |
|---|---|---|
| (chain start) → prototyper | `goal` (the Goal document) | `file` |
| prototyper → shaper | `exploration-notes` | `file`, `spike-worktree`, `engram-observation` |
| shaper → estimator | `shaper-handoff` (v3/v4, `engine/shaper/handoff_v4.go:36`) | `file`, `engram-observation` |
| estimator → builder | `estimate-report` | `file` |
| builder → sweeper/polisher/reviewer | `work-unit` | `commit`, `test-run`, `lint`, `file` |
| sweeper → polisher/reviewer | `sweep-report` | `commit`, `test-run` |
| polisher → reviewer | `polish-report` | `commit`, `test-run`, `judge-result` |
| reviewer → delivery (or → builder) | `review-verdict` | `review-receipt`, `judge-result` |
| delivery (terminal) | `delivery-record` | `commit`, `pr`, `archive-report` |

`context.summary/decisions/open_questions` is the narrative every role must fill (`handoff.go:47-51`); `evidence[].sha256` binds each ref to bytes, so a role never cites what it did not hash.

**Interruption, uniformly.** A role that cannot finish stops, returns `status: interrupted` and a non-blank `resume_reason`; the next record repeats the same from/to pair (`handoff.go:30-34`, `:73`). A role never pretends completion and never widens its surface to get unstuck. Rework is only reviewer → builder (`roles.go:138`).

**Result envelope** (all roles, returned as the final message): `status` completed|interrupted, `payload_kind`, `payload_ref`, `summary`, `decisions[]`, `open_questions[]`, `evidence[]`, `resume_reason` when interrupted, `skill_resolution`. This is the current `sdd-*` result contract (`agents/sdd-apply.md:41-49`) reshaped onto `RoleHandoff` fields so the parent can append a record without translation.

**Write surface rule.** The parent names the allowed surfaces in the prompt; the role refuses writes outside them and returns `interrupted` with the path it needed (mirrors the orchestrator's "Allowed edit surfaces" rule, enforced today only for writers).

## 3. Per role

### 3.1 prototyper
- Purpose: throwaway spikes to de-risk unknowns before shaping (`roles.go:54-56`).
- Consumes `goal`; produces `exploration-notes`: what was tried, what broke, the narrowed problem statement, and the unknowns that remain.
- May: read the repository, CodeGraph, Engram/longterm-mem within the injected plan; run spikes in a scratch worktree or `mktemp -d` outside the repository; Bash for builds/tests of the spike.
- May not: write tracked files, commit, open PRs, touch `openspec/`, decide scope.
- Replaces: `skills/sdd-explore` (today no agent); the research half of `prespec-malandra`'s output is an input, not replaced (assumption).
- Interruption: notes so far + `resume_reason` naming the open unknown.
- Prefix draft:
  ```
  You are the prototyper of a labdrian role chain. Do this role's work yourself; never launch sub-agents.
  Input: the Goal at the path this prompt names. Read it first.
  Output: exploration-notes at the path this prompt names, then the result envelope.
  Spikes are throwaway: build them only under the scratch directory this prompt names, never in tracked files.
  You may read anything; you may write only the notes and the scratch directory.
  Do not decide scope or write a plan; end with a narrowed problem statement and the unknowns left.
  If you cannot finish, stop with status interrupted and a resume_reason that names what is still unknown.
  Cite path:line for every claim about existing code. Never invent evidence.
  Artifacts are in English. Procedure: skills/role-prototyper/SKILL.md.
  ```

### 3.2 shaper
- Purpose: bounded, appetite-limited plan with scope and out-of-scope limits (`roles.go:60-62`); the plan is the Shaper handoff v3/v4 (architecture, stages, roles, tests, acceptance, risks, estimates, memory scope, delivery limit; `roadmap.md:33`, `handoff_v4.go:36`).
- Consumes `exploration-notes` (optional, `roles.go:61`); produces `shaper-handoff`.
- May: read; write the plan artifact(s) only: the Shaper handoff JSON and, in SDD mode, `openspec/changes/<name>/{proposal,specs,design,tasks}.md` (assumption: the SDD artifact set maps onto the handoff's sections, see section 4).
- May not: write source or tests, run the implementation, estimate authoritatively (estimator does), choose for the user on open product decisions — it returns them in `open_questions` (today `agents/sdd-propose.md:15`).
- Replaces: `sdd-propose`, `sdd-spec`, `sdd-design`, `sdd-tasks` (agents and skills), and GADU's judgment rules (section 5).
- Interruption: partial plan persisted; `resume_reason` names the missing section.
- Prefix draft:
  ```
  You are the shaper of a labdrian role chain. Do this role's work yourself; never launch sub-agents.
  Input: the Goal and, when present, exploration-notes at the paths this prompt names. Read both first.
  Output: a shaper-handoff (scope, out-of-scope, architecture, stages, roles, tests, acceptance, risks) at the path this prompt names, then the result envelope.
  Write only the plan artifacts this prompt names. Never write source, tests, or commits.
  Quote the user's own words for scope and acceptance; never add requirements nobody asked for.
  State the strongest counter-argument to the chosen approach and why it still wins.
  An open product decision is returned in open_questions, never decided here.
  If you cannot finish, stop with status interrupted and a resume_reason naming the missing section.
  Artifacts are in English. Procedure: skills/role-shaper/SKILL.md.
  ```

### 3.3 estimator
- Purpose: effort ranges and confidence per stage before commitment (`roles.go:66-68`).
- Consumes `shaper-handoff`; produces `estimate-report` (per-stage ranges, confidence, buffer, checkpoint count — the shape of `skills/sdd-time-estimation`).
- May: read the plan, the repository, past actuals in Engram (within the injected plan).
- May not: edit the plan's scope, write source, run anything that mutates.
- Replaces: `sdd-time-estimation` (skill; no agent today).
- Tension to resolve: the Shaper handoff v3 already requires per-stage estimates and rejects a missing one (`roadmap.md:33`). See open decision 5.
- Interruption: ranges for the stages done; `resume_reason` names the unestimated stage.
- Prefix draft:
  ```
  You are the estimator of a labdrian role chain. Do this role's work yourself; never launch sub-agents.
  Input: the shaper-handoff at the path this prompt names. Read it first.
  Output: an estimate-report (per-stage effort range, confidence, buffer) at the path this prompt names, then the result envelope.
  You write only that report. Never change scope, stages, or acceptance; disagreements go in open_questions.
  Ground every range in evidence you cite: file counts, similar past units, test runner cost.
  If you cannot finish, stop with status interrupted and a resume_reason naming the stage left.
  Artifacts are in English. Procedure: skills/role-estimator/SKILL.md.
  ```

### 3.4 builder
- Purpose: code, tests, docs for one bounded work unit (`roles.go:72-74`).
- Consumes `estimate-report` (and the plan it embeds); produces `work-unit`: commit id(s), checks run with observed results, RED→GREEN evidence when the test-first policy applies.
- May: read; edit only the allowed surfaces named in the prompt; Bash for the named runner and git on the feature branch; `mem_save` of discoveries.
- May not: launch sub-agents, widen surfaces, approve anything, push, open PRs, delete scratch it did not create (reports its paths instead).
- Replaces: `sdd-apply` (agent and skill).
- Interruption: commits what is green, records progress in the envelope (today's `apply-progress`, `agents/sdd-apply.md:23`), `resume_reason` names the next task.
- Prefix draft:
  ```
  You are the builder of a labdrian role chain. Do this role's work yourself; never launch sub-agents.
  Input: the plan and estimate at the paths this prompt names, plus the task ids and S# you own. Read them first.
  Output: committed changes on the named branch and a work-unit record (commits, checks with observed results), then the result envelope.
  Edit only the surfaces listed under "Allowed edit surfaces"; anything else is a reason to stop, not to improvise.
  When a runnable deterministic test exists, observe RED before the fix, then GREEN; never invent evidence.
  Run the verification commands this prompt names and report each as "<command>: <observed result>".
  Keep scratch outside the worktree; report its paths and do not delete them.
  If you cannot finish, commit what is green, stop with status interrupted and a resume_reason naming the next task.
  Artifacts are in English. Procedure: skills/role-builder/SKILL.md.
  ```

### 3.5 sweeper (optional)
- Purpose: loose ends — dead code, stale comments, leftover scaffolding (`roles.go:79-81`).
- Consumes `work-unit`; produces `sweep-report` plus a cleanup commit, behaviour unchanged.
- May: read the diff of the work unit; edit only files already in that diff; run the suite once.
- May not: touch files outside the unit's diff, change behaviour, change tests' assertions.
- Replaces: nothing by name today (the orchestrator's `simplify` pass is the nearest, assumption); it is the home for the "follow-ups"/"suggestions" carried from review rounds.
- Prefix draft:
  ```
  You are the sweeper of a labdrian role chain. Do this role's work yourself; never launch sub-agents.
  Input: the work-unit record and its diff range this prompt names. Read them first.
  Output: one cleanup commit (or none) and a sweep-report listing what was removed and why, then the result envelope.
  Edit only files already in that diff. Behaviour, public names, and test assertions stay identical.
  Run the suite once after your change and report "<command>: <observed result>".
  If the suite goes red, revert your change and report it as a finding instead.
  If you cannot finish, stop with status interrupted and a resume_reason naming the file left.
  Artifacts are in English. Procedure: skills/role-sweeper/SKILL.md.
  ```

### 3.6 polisher (optional) — judgment-day lives here (owner decision 2)
- Purpose in the catalog: presentation without changing behaviour (`roles.go:86`). Owner decision: `jd-judge-a/b` and `jd-fix-agent` map here, which means the polisher pass is the judgment-day protocol: blind dual judging, verdict synthesis, surgical fixes of confirmed findings, at most two rounds (`skills/judgment-day/SKILL.md:3`). Confirmed fixes can change behaviour; the catalog text must be widened (open decision 1).
- Consumes `work-unit` or `sweep-report`; produces `polish-report`: the judge ledger, the confirmed findings, each fix as `file:line — what`, focused test per fix (`agents/jd-fix-agent.md:19-22`).
- Two sub-modes inside one role, selected by the prompt: **judge** (read-only, JSON findings only, `agents/jd-judge-a.md:17`, `:30-36`) and **fix** (edit only the confirmed ledger entries, never add rows, `agents/jd-fix-agent.md:16-18`, `:31`). The parent runs judge A and B in parallel, synthesizes, then one fix call; a role sub-agent never launches the judges itself.
- May not: refactor beyond a confirmed finding, add ledger rows, re-judge its own fixes.
- Replaces: `jd-judge-a`, `jd-judge-b`, `jd-fix-agent`, `skills/judgment-day`.
- Interruption: fixes applied so far with ledger status, `resume_reason` names the next ledger id.
- Prefix draft (judge mode; fix mode swaps lines 3-5):
  ```
  You are the polisher of a labdrian role chain, in judge mode. Do this role's work yourself; never launch sub-agents.
  Input: the immutable target this prompt names (diff range or files). Read only that.
  Output: one JSON object of findings, no prose: location, severity, claim, evidence_class, causal_disposition, proof_refs.
  You modify nothing. Every BLOCKER or CRITICAL cites causal proof; style is not a finding.
  Return {"findings":[],"evidence":[...]} when clean.
  (fix mode) Edit only the confirmed ledger ids this prompt lists; never add rows; one focused test per fix; report "file:line — what".
  If you cannot finish, stop with status interrupted and a resume_reason naming the ledger id left.
  Artifacts are in English. Procedure: skills/role-polisher/SKILL.md.
  ```

### 3.7 reviewer — gentle-ai lenses live here (owner decision 3)
- Purpose: verdict — approved for delivery or back to the builder with findings (`roles.go:92-94`).
- Consumes `work-unit` / `polish-report` plus acceptance criteria; produces `review-verdict`. Five sub-modes by prompt: `risk`, `resilience`, `readability`, `reliability`, `refuter`, each returning the native JSON shape bound to `subject_hash` (`agents/review-risk.md:41-49`).
- Hard floor from today's agents: `tools: []`; the sole context is `GENTLE_AI_REVIEW_BINDING` + `GENTLE_AI_REVIEW_CONTEXT`; never the live worktree, index, or HEAD (`review-risk.md:5`, `:14`). This must survive the merge into one agent, so the reviewer agent is **tool-less**; a spec-conformance review that needs to run tests is not this role's job (see `sdd-verify` row in section 4 and open decision 3).
- May not: edit, delegate, read outside the injected context, compute `subject_hash`.
- Replaces: `review-risk`, `review-resilience`, `review-readability`, `review-reliability`, `review-refuter`; `sdd-verify` (agent and skill) folds in as the acceptance-criteria check carried in the verdict.
- Interruption: an incomplete inspection is returned as `inspection.status` incomplete with the reason (`review-risk.md:18`, `:47`); at chain level it is `interrupted` with that reason.
- Prefix draft:
  ```
  You are the reviewer of a labdrian role chain, lens <risk|resilience|readability|reliability|refuter>. Review once, return one result, stop.
  Input: GENTLE_AI_REVIEW_BINDING and the GENTLE_AI_REVIEW_CONTEXT block that follows it. Nothing else is context.
  You have no tools: never read the worktree, run commands, or substitute live files.
  Before inspecting, check binding.subject_hash equals artifact_subject.subject_hash and path evidence covers every manifest path once, in order; otherwise return incomplete.
  Output: one JSON object, no prose: subject_hash (copied), inspection, findings, evidence.
  Only candidate-caused, user-impacting defects; BLOCKER/CRITICAL need changed-hunk or differential proof.
  Return "findings":[] with one evidence entry when clean.
  Artifacts are in English. Lens scope: skills/role-reviewer/SKILL.md section <lens>.
  ```

### 3.8 delivery
- Purpose: close the loop; terminal (`roles.go:98-100`, `:135`).
- Consumes `review-verdict` (approved); produces `delivery-record`: branch, commits, PR url when authorized, archive report with observation ids (today `agents/sdd-archive.md:30`).
- May: git state and `gh` reads; `cp -R`/`mv`/`git mv` for archive moves, never Read-then-Write of artifacts (`sdd-archive.md:35`); merge delta specs into main specs in openspec/hybrid mode (`sdd-archive.md:28-29`); push / PR creation only when the prompt carries the user's explicit authorization.
- May not: merge, force-push, delete branches, rewrite checkboxes or historical reports (`sdd-archive.md:20`), edit source.
- Replaces: `sdd-archive` (agent and skill). `branch-pr`, `chained-pr`, `work-unit-commits` stay as procedure the delivery skill names (whether they are among the 15 upstream core skills is not verified).
- Interruption: nothing is half-moved — archive is one mechanical step; if it fails, report the exact failing path with `interrupted`.
- Prefix draft:
  ```
  You are the delivery role of a labdrian role chain. Do this role's work yourself; never launch sub-agents.
  Input: the approved review-verdict and the change locator this prompt names. Read them first.
  Output: a delivery-record (branch, commits, PR url if any, archive report) at the path this prompt names, then the result envelope.
  Move or copy artifacts only with cp -R, mv, or git mv; never read a file and write it back.
  Never rewrite task checkboxes or historical reports; record unfinished work as unfinished.
  Push, PR creation, and merge happen only when this prompt quotes the user's authorization; otherwise report "awaiting authorization".
  If a step fails, stop with status interrupted and a resume_reason naming the failing path.
  Artifacts are in English. Procedure: skills/role-delivery/SKILL.md.
  ```

## 4. Phase → role mapping (resolves gap 6)

| Current phase agent / skill | Role | Payload it becomes | Notes |
|---|---|---|---|
| `sdd-init` | none (orchestrator-run skill) | — | Project bootstrap, not a goal-chain step: stack detection + `.atl/skill-registry.md` (`agents/sdd-init.md:20-23`). Keep as a skill without an agent, like `sdd-explore` today. |
| `sdd-onboard` | none (orchestrator drives the whole chain) | — | It walks all phases (`agents/sdd-onboard.md:21`); as a sub-agent it would have to launch sub-agents, which no role may do. Retire the agent; keep a short teaching skill the parent follows. |
| `sdd-explore` (skill only) | prototyper | `exploration-notes` | Direct match (`roles.go:54-56`). |
| `sdd-propose` | shaper | `shaper-handoff` (intent, scope, out-of-scope) | `agents/sdd-propose.md:22-24` ↔ `roles.go:60-62`. |
| `sdd-spec` | shaper | `shaper-handoff` (acceptance per criterion) | v3 carries acceptance with per-criterion verification (`roadmap.md:33`). |
| `sdd-design` | shaper | `shaper-handoff` (architecture, ADR decisions) | v3 carries architecture (`roadmap.md:33`). |
| `sdd-tasks` | shaper (stages) | `shaper-handoff` (stages) | Parallel/sequential marking (`agents/sdd-tasks.md:23`) is a stage attribute. |
| `sdd-time-estimation` (skill) | estimator | `estimate-report` | See open decision 5. |
| `sdd-apply` | builder | `work-unit` | `apply-progress` becomes the interrupted envelope. |
| `sdd-verify` | reviewer (acceptance check) | part of `review-verdict` | Semantic change: today verify "never gates archive" (`agents/sdd-verify.md:42`); a reviewer verdict does gate (`roles.go:94`). Open decision 3. |
| `judgment-day`, `jd-*` | polisher | `polish-report` | Owner decision 2. |
| `review-*` | reviewer | `review-verdict` | Owner decision 3. |
| `sdd-archive` | delivery | `delivery-record` | Terminal. |
| `GADU`, `gadu-operator`, `gadu-orchestrate` | shaper (rules only) | — | Section 5. |

Chain shape for an SDD change: prototyper (optional in practice when exploration already exists — but the engine makes it mandatory, `roles.go:14-16`; the parent supplies a trivial `exploration-notes` payload then) → shaper (one agent, four artifacts) → estimator → builder (one call per task group) → sweeper? → polisher? → reviewer (one call per lens, parent synthesizes) → delivery. The ODD path uses the same chain with the feature document as the shaper payload.

## 5. GADU analysis (owner decision 4)

Evidence. Six traits: judgment, red-team the user's logic (steelman then strongest counter-argument), no sycophancy, recommend the highest-odds path, autonomy and sub-agent orchestration, source-grounded (`agents/GADU.md:20-30`). Voice section is persona (`GADU.md:32-36`). Two "signature capabilities": adversarial review via judgment-day, and parallel fan-out via native dispatch (`GADU.md:42-44`). `tools: '*'` (`GADU.md:5`). Its own text says a sub-agent "generally cannot spawn its own sub-agents" and must then work single-threaded (`GADU.md:28`). The body is generated from `engine/gadu/persona/body.md` (`GADU.md:8`).

Fit per candidate role:
- **Prototyper**: GADU argues; it does not spike. Trait 6 (verify against sources) overlaps, but the role's output is throwaway code and notes, not a position. Weak fit.
- **Builder**: the builder must follow the plan and its surfaces; GADU's defining move is to re-open the premise before building (`GADU.md:22`). That is exactly what a builder must not do mid-unit. Poor fit.
- **Sweeper**: nothing in GADU is about cleanup. No fit.
- **Shaper**: "steelman, hunt for flaws, surface the strongest counter-argument, recommend one option and name the tradeoff, ask the one question that resolves a real fork" (`GADU.md:22`, `:26`, `:34`) is the shaper's job description: a bounded plan with explicit out-of-scope and the open product decision returned, not taken (`roles.go:60-62`). Strong fit for traits 1, 2, 4, 6. Trait 5 (lead sub-agents) belongs to the parent orchestrator, which already owns delegation; trait 3 and the Voice section are persona and are excluded by the minimal-rules constraint.

Recommendation: **shaper, as rules, not persona.** Retire `agents/GADU.md` and the `gadu-operator` skill as sub-agent material; fold the three testable rules (steelman + strongest counter-argument before choosing; recommend one option and name its tradeoff; return a real fork as exactly one question) into the shaper skill and the one prefix line "State the strongest counter-argument…". Fold `gadu-orchestrate`'s fan-out/dialectic modes (`skills/gadu-orchestrate/SKILL.md:26-57`) into the parent's own procedure, because only the parent may fan out. The adversarial-review capability is already the polisher.

Strongest counter-argument: GADU is a top-level operator invoked by name, not a lifecycle role; its value is the persona and the authority to orchestrate, and the experiment says persona-only prompts score 3/10. Mapping it anywhere keeps the name and discards what made it GADU, so the honest move is to retire it outright rather than call the shaper "GADU". If the owner wants GADU to survive, the only coherent home is the top-level session's output style (where persona is allowed), never a sub-agent role.

## 6. Open decisions for the owner (max 6)

1. **Polisher purpose text.** The catalog says "without changing behavior" (`roles.go:86`) but judgment-day fixes change behaviour. Recommendation: amend the `Catalog` string to "apply confirmed, ledger-bound findings and presentation fixes; never widen scope" in one small engine PR with its test, before C15; do not route jd fixes through reviewer → builder, because that would make every polish round a formal rework.
2. **Enforce the payload_kind vocabulary in the engine or keep it a skill convention?** Today any non-blank string passes (`handoff_validate.go:79`). Recommendation: add `PayloadKind` to `RoleInfo`/`Catalog` and validate `to_role` against it in `AdmitRecord` (hexagonal: the rule lives in the domain), with a v2-compatible transition (existing chains unaffected). One PR before the role skills land.
3. **sdd-verify semantics.** Fold into the reviewer (verdict gates delivery) or keep it a non-gating builder self-check? Recommendation: reviewer. The chain already has the only backward edge for exactly this (`roles.go:138`); a non-gating verify has no place in the typed chain. The runtime half (running the suite) stays with the builder's self-verification; the reviewer judges from the injected evidence only.
4. **GADU.** Shaper-as-rules and retire the agent (recommended, section 5), or retire outright with no rule transfer. Recommendation: shaper-as-rules; keep `gadu-orchestrate` modes as parent procedure.
5. **Estimator versus the shaper's mandatory estimates.** Shaper v3 rejects a handoff without estimates (`roadmap.md:33`), yet estimator comes after shaper in the chain (`roles.go:29-38`). Recommendation: the shaper writes provisional ranges marked `confidence: draft`; the estimator replaces them and the handoff reaches `ready` only after the estimator record; `roles` chain order unchanged, no engine change (readiness is a human clearance in the Pi TUI anyway, `roadmap.md:33`).
6. **The ~50 generated lines per agent** (CodeGraph, language, remote authorization; `agents/sdd-apply.md:51-99`) contradict "minimal rules". Recommendation: keep the one-line language rule and the remote-authorization block in the prefix (safety floor), and move the CodeGraph guidance into the prototyper/builder skills only, if the generator that stamps these blocks is ours; if it is gentle-ai's, accept them and measure the cost in the telemetry baseline before deciding.

## 7. Not verified (assumptions I made)

- The experiment figures (2026-10-22, n=10/arm, 9/10 vs 7/10 vs 3/10, $0.059 vs $0.26) are taken from the brief; nothing in the repository records them (`rg` found no "minimal-rules"/"persona-only"), and the date is after today (2026-10-21).
- Which 15 skills are the "upstream core" set (Q2) was not read; `branch-pr`, `chained-pr`, `work-unit-commits`, `go-testing`, `skill-*` may be among them and are therefore not named for rewrite here.
- Whether the gate-task hook can key a contract by an arbitrary `subagent_type` such as `builder` (only `gate.go:79-100` was read, not the contract catalog).
- Whether the generated blocks in `agents/*.md` are stamped by a tool we own (`gentle-ai-overlay gadu-generate` exists for GADU only, `engine/cmd/usage.go:43`) or by gentle-ai sync.
- Whether `agents/*.md` are subject to the skills lint budget; the lint applies to `SKILL.md` files (`engine/skills/lint.go`), so the ≤ 15-line prefix limit here is a design constraint, not a lint rule.
- The evidence-kind vocabulary (`commit`, `test-run`, `file`, …) is my proposal; the engine accepts any non-blank kind (`handoff_validate.go:91`).
- The vault pages in `~/labdrian-brain` (C12) were not read; this draft is from code and ledgers only.
- Whether the SDD artifact set (proposal/spec/design/tasks) can be embedded one-to-one in the Shaper handoff v4 without a format change; only the field list at `engine/shaper/handoff_v4.go:36` and the v3 `Role{Role, Responsibility}` shape (`engine/shaper/handoff_v3.go:20-23`) were read.
