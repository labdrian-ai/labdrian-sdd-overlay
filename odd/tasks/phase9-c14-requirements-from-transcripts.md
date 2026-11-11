# C14: rewrite requirements-from-transcripts within the lint budget and take it out of the lint baseline
Objective: rewrite `skills/requirements-from-transcripts` (entry and its two references) so it meets the C16 gate (lint, an owner approval record, removal from `lintBaselineSkills`). Every contract other skills consume stays the same. Branch: `feat/phase9-c14-requirements`, base `origin/main` 38c11819.
Delivery: one PR; the parent owns commits, the PR and its merge after green CI. Test runner: `go test` from `engine/` (`go -C engine test ...` from the repository root).

## Specs

- **S1 — Boundary.** Owner, 2026-11-11: "BUENO CONTINUEMOS HASTA CERRAR". On the parent's question "¿Rehacemos solo `requirements-from-transcripts`, o también otras skills de planificación? … Mi recomendación es rehacer solo `requirements-from-transcripts`, sacarla del baseline y cerrar C16 con eso." the owner answered "bueno de acuerdo". Ledger (`odd/tasks/consolidation-phase9.md:112,121`): "C14 — Rewrite the remaining custom planning skills." and "C16 — Every new or rewritten skill passes lint, carries an approval record, and leaves the baseline." Scope:
  - `skills/requirements-from-transcripts/SKILL.md` and its existing references `references/output-template.md` and `references/patterns-catalog.md`;
  - their new pin test;
  - the lint-baseline count surfaces listed in L2.
  
  Acceptance: the final diff rewrites no other skill and does not edit `skills/_shared/*`, `skills.registry.yaml`, role bodies or generated agents. It changes no schema or validator. `overlay.manifest` changes only when a reference is added or removed (assumption: none is). The items C13 carried "to C14 or a spec follow-up" (C13 L37-L39: the inception closure wording) are not in this scope.

- **S2 — Contracts other skills consume (invariants).** The rewrite keeps each of these, and the pin test checks each one:
  - (a) The skill derives `change-name` exactly once, as a kebab-case slug. It emits the slug in the brief header and in the Project-Inception Handoff table, and nothing downstream re-derives it. Consumers: `skills/inception-pipeline/SKILL.md:28` ("`change-name` comes once from requirements-from-transcripts; inherit it verbatim"); `skills/_shared/pre-sdd-contracts.md:28-36`; `skills/_shared/entry-contract.schema.json:35`; `skills/clarify-request/SKILL.md:53` ("it owns the change-name and the EARS requirements").
  - (b) It persists the brief under topic key `project/{project}/requirements/{change}` (`pre-sdd-contracts.md:45`). `skills/project-inception/SKILL.md:26` loads that full record when the parent passes `requirements_brief_ref`.
  - (c) The brief has the shape `pre-sdd-contracts.md:74-76` defines: "EARS-formatted requirement items (R-NNN), each carrying a `scope` enum of `new-capability | feature | fix`, plus a project-inception handoff table". IDs use the `R-001` format and stay stable. Each requirement line is one EARS sentence with exactly one SHALL. Closure counts these items: `skills/inception-pipeline/references/closure-feedback.md:11` reads "the count of EARS R-NNN requirements" for `requirement_count`.
  - (d) The Tier 3 lightweight object keeps its fields: `R-NNN`, `scope: fix`, `one_line_impact`, `acceptance_evidence`, `change_name`, plus the EARS sentence. The skill switches to the full pipeline on any ambiguity. Consumer: `skills/inception-pipeline/references/tier-selection.md:54` ("requirements-from-transcripts (lightweight)").
  - (e) The skill accepts a clarify-request brief and a prespec synthetic transcript as input (`clarify-request/SKILL.md:53,58`; `prespec-malandra/SPEC.md:374`).
  - (f) Persisted artifacts are in English and stakeholder quotes stay verbatim (`pre-sdd-contracts.md:20-24`). The entry loads `../_shared/pre-sdd-contracts.md` (`pre-sdd-contracts.md:5`).

- **S3 — The skill's own rules survive.** The entry still carries the rules that decide whether a requirements brief may exist:
  - no invented requirements: an underived requirement is marked as an assumption;
  - the contradiction and ambiguity pre-analysis runs before any requirement. It covers anything that affects permissions, data, workflow, money, compliance or user-visible behavior. It asks about one issue at a time, with options, then stops and waits. It continues only after each issue is resolved or the user approves named assumptions;
  - the EARS rule: one SHALL, banned soft verbs blocking;
  - one fix or feature per requirement;
  - acceptance evidence and keywords on every requirement.
  
  Detail that applies only while drafting may move to the existing references, loaded through an explicit instruction: the full EARS table, the classification and examples, the three downstream input layers, the traceability and quality bar, and the decision-prompt and brief templates. Detail never moves just to escape the token budget. Acceptance: each rule above appears in the entry or in a reference the entry tells the reader to load at the step that needs it, and the test pins where each one lives.

- **S4 — Storage follows the memorycard override.** New writes go to memorycard through capabilities the parent resolves, under the S2(b) topic key, and the writer reads them back. The skill never writes or exports Engram. It never passes a field the store does not have: `capture_prompt` is an Engram `mem_save` argument and does not exist in memorycard `mem_save` (title, body, topic_key, type, scope, project, cwd, directory, supersedes). The precedent is C13 S4 and `skills/project-inception/SKILL.md:16`. Assumption: the shared `pre-sdd-contracts.md:59` line ("use `capture_prompt: false`") stays as it is because S1 keeps `_shared` out of scope; the rewritten skill simply does not repeat it. Acceptance: the entry names the topic key, memorycard and readback, and contains no Engram write or export instruction and no `capture_prompt`.

- **S5 — No routing into retired SDD artifacts.** The current entry tells readers:
  - to produce candidates for "`sdd/<change-name>/{proposal,spec,design,tasks}` artifacts in Engram" (`SKILL.md:39`);
  - "Genesis SDD artifacts are Engram-first" (`SKILL.md:40`);
  - to write for "future SDD verify agents" (`SKILL.md:108`).
  
  Those keys belong to the SDD engine, and the role chain writes none of them (`pre-sdd-contracts.md:57`). Today the route is project-inception or roadmap-maker, then the role chain, as `skills/inception-pipeline/SKILL.md:28-34` describes. The rewritten entry and references name that route. They give no instruction to create `sdd/*` engine artifacts or `openspec/` artifacts, and they keep no "SDD verify agents" or "before PASS/archive" wording. Assumption: the brief's section headings, including `SDD Change Candidates`, `SDD Roadmap Inputs` and `Project-Inception Handoff`, keep their names; no consumer reads them by name (repository grep, L2). Acceptance: the pins reject `sdd/<change-name>/{proposal`, `Engram-first`, `SDD verify agents` and `capture_prompt` in the entry and in both references.

- **S6 — Lint limits (C16, part 1).** Same table as C13 S8:
  - description at most 250 runes;
  - body at most 1000 estimated tokens (ceil of UTF-8 bytes / 4), pinned at 950 by the test so one edit cannot reach the hard budget;
  - no hard lint findings;
  - the shape Activation Contract / Hard Rules / Decision Gates / Execution Steps / Output Contract.
  
  The description keeps its activation triggers, including the Spanish "requerimientos desde meets" and "historias de cliente" (a host picks a skill by its description alone). Acceptance: `go -C engine run ./cmd/ skills lint ../skills/requirements-from-transcripts/SKILL.md` exits 0 with no hard findings, and the pin test passes.

- **S7 — Leaves the baseline (C16, part 2).**
  - Remove `"requirements-from-transcripts"` from `lintBaselineSkills` in `engine/skills/approval_write.go:28` and nothing else.
  - The count goes from 29 to 28 in `engine/skills/approval_gate_test.go:380` (`lintBaselineSize`), in `engine/skills/registryyaml_adapter_test.go:159`, in `README.md` (lines 255 and 465) and in `bin/lib/usage.sh` (lines 183 and 215).
  - Guard logic, flags and messages stay unchanged.
  
  Acceptance: `TestLintBaseline_IsWellFormedAndFixed` fails at 29 before the removal and passes at 28 after it.

- **S8 — Owner approval record (C16, part 3).** Only after the skill tree's final bytes, paths and modes are stable, the owner runs `labdrian skills approve --id requirements-from-transcripts --approver <name> --source-root <worktree>/skills --registry <worktree>/skills.registry.yaml --manifest <worktree>/overlay.manifest` from a terminal they control. The approval digest covers the references too (`engine/skills/approval_digest.go`). No agent edits `.approval.json`, runs approve or names an approver. Any later byte change to the tree makes the approval stale and needs a fresh one. Acceptance: source validation reports the skill as approved; while approval is pending, C16 is not done and no check reports GREEN.

- **S9 — Verification.**
  - Test-first: the pin test fails (RED) on the current skill before any skill edit, and the baseline test fails at 29 before the removal.
  - The pins check obligations and where they live, not preferred wording.
  - Prompt-only behavior gets structural readback. RED is marked n/a there, with no invented model test.
  - Final checks, all run once: `go -C engine test -count=1 ./...`, `go -C engine vet ./...` and `gofmt -l` on the changed Go files; `go -C engine run ./cmd/ skills validate --registry ../skills.registry.yaml --manifest ../overlay.manifest --source-root ../skills`; `OVERLAY_DIR="$PWD" go -C engine run ./cmd/ gadu-generate --check`; `bash -n bin/lib/usage.sh` with `go -C engine test -count=1 ./shelltest`; `git diff --check`.

## Tasks

- [ ] T1 — S1-S6,S9 — inline — New pin test `engine/skills/requirements_from_transcripts_test.go` (RED on the current skill), then rewrite `skills/requirements-from-transcripts/SKILL.md` to GREEN within the lint limits, preserving every S2 contract and the S3 rules, with storage per S4 and routing per S5 — shaped: L2 — commit: -
- [ ] T2 — S2-S5,S9 — inline — Align the two references with the rewritten entry: S5 retired-routing and S4 Engram wording out, the drafting detail the entry delegates in, both reference paths unchanged; extend the pins to the references (RED first) — shaped: L2 — commit: -
- [ ] T3 — S7,S9 — inline — Remove only `requirements-from-transcripts` from `lintBaselineSkills` (RED at 29, GREEN at 28) and update the count surfaces — shaped: L2 — commit: -
- [ ] T4 — S8 — inline — Parent presents the final `skills/requirements-from-transcripts/` tree and the exact approve command; the owner reviews it and runs approve; parent reads back the record and reruns source validation — shaped: L2 — commit: -
- [ ] T5 — S1-S9 — inline — Final validation: every S9 command once on the approved tree, results recorded per `S#` in this Log, with no approval or check claimed that was not observed — shaped: L2 — commit: -

## Log

- **L1** (2026-11-11, owner): 2026-11-11, after C13 closed, the owner asked to continue Phase 9 to its end: "BUENO CONTINUEMOS HASTA CERRAR". The parent asked: "¿Rehacemos solo `requirements-from-transcripts`, o también otras skills de planificación? … Mi recomendación es rehacer solo `requirements-from-transcripts`, sacarla del baseline y cerrar C16 con eso." The owner replied: "bueno de acuerdo".

  Phase ledger wording (`odd/tasks/consolidation-phase9.md`): "C14 — Rewrite the remaining custom planning skills." and "C16 — Every new or rewritten skill passes lint, carries an approval record, and leaves the baseline."

- **L2** (2026-11-11, shaper): Designed S1-S9 and T1-T5. No task has run. This is the only shaper entry and the target of every `shaped: L2` mark. The shaper wrote only this document: no code, test, approval or commit.

  **Recommendation and counterargument.**
  - Steelman: requirements-from-transcripts is the first producer of the inception path. Its brief seeds project-inception (`project-inception/SKILL.md:26`), its slug names the role chain (`inception-pipeline/SKILL.md:28-34`), and closure counts its R-NNN items (`closure-feedback.md:11`). It is the last skill of that path still on the lint baseline (`approval_write.go:28`), and it still sends work to retired `sdd/*` Engram artifacts (`SKILL.md:39-40,108`). A compact entry costs every caller less to load and closes C16 for the path.
  - Strongest counterargument: most of the skill's value is its long procedure: 21 execution steps, the three downstream layers and the quality bar (`SKILL.md:88-157`). These steps shape the quality of the brief a model writes. No deterministic test proves that quality, so a compaction that sounds right can quietly lower it, and pins only show that text is present.
  - Recommendation: keep the entry as the contract (S2 invariants, S3 gating rules, storage, routing). Move only the procedure that applies while drafting into the two references that already exist, loaded at the step that needs them. Accepted tradeoff: structural pins and owner review stand in for proof of model output quality, which is not measured.

  **Sources and findings.**
  - Read: `skills/odd-feature-doc/SKILL.md`; the full skill and both references; `odd/tasks/phase9-c13-inception-family.md` (Specs, Tasks, L2 and L33-L42); `skills/inception-pipeline/SKILL.md`; `skills/_shared/pre-sdd-contracts.md`; `skills/clarify-request/SKILL.md:40-58`; `engine/skills/approval_write.go:1-60`; `engine/skills/inception_family_artifact_test.go:95-168`.
  - Repository-wide grep, which names the consumers in S2: `inception-pipeline/references/tier-selection.md:20,32,54`, `closure-feedback.md:11`, `project-inception/SKILL.md:26`, `entry-contract.schema.json:35`, `overlay.manifest:55,132-133`, `bin/lib/bootstrap.sh:60`, `bin/overlay:58`.
  - `docs/architecture/role-design/per-role-design-v3.md` has no match for requirements, transcript, inception or clarify. The skill belongs to the parent's inception route, not to a role body. Unverified beyond that grep.
  - No living spec under `openspec/specs/` owns this skill's rules. Its `R-NNN` mentions (runtime-lifecycle, longterm-mem-*) concern their own specs, so no Spec here rewords a living rule.
  - No existing engine test pins this skill's text. `engine/skills/clarify_request_test.go:592` pins clarify-request's wording about this skill.
  - The current description and body sizes were not measured by the shaper. T1's RED reports them.

  **C13 pitfalls carried forward.**
  - (1) Read the owning source before changing a rule. C13 L37 recorded that the parent unscoped a rule the living actuals spec owned, and the pin protected the divergence. Here, the owning sources are the S2 consumers, so the pins check those clauses against the consumers' own text.
  - (2) Pin obligations, not wording. C13 L34-L36 went through three wording rounds and spent budget trims to stay under 950.
  - (3) Run approve with `--source-root` and the worktree's registry and manifest. Without them it read the main checkout and reported `unchanged` (C13 L35).
  - (4) Keep the branch on an unmoved base. CI's `Merge Lands A Reviewed Tree` refused a branch that had merged `main` in (C13 L40-L41).

  **Planned edit surfaces.** Future builder surfaces; this shaper wrote only this document. No builder edits `.approval.json`.

  | Task | Paths |
  |---|---|
  | T1 | `skills/requirements-from-transcripts/SKILL.md`; new `engine/skills/requirements_from_transcripts_test.go` |
  | T2 | `skills/requirements-from-transcripts/references/output-template.md`; `skills/requirements-from-transcripts/references/patterns-catalog.md`; `engine/skills/requirements_from_transcripts_test.go` |
  | T3 | `engine/skills/approval_write.go` (one ID); `engine/skills/approval_gate_test.go` (count); `engine/skills/registryyaml_adapter_test.go` (comment count); `README.md` and `bin/lib/usage.sh` (count prose only) |
  | T4 | none for agents; the owner's approve command writes `skills/requirements-from-transcripts/.approval.json` |
  | T5 | this document's Log |

  **Test design.**
  - The new test reuses `SplitSkillFile`, `LintSkillFile`, `EstimateBodyTokens`, `parseFrontmatterFields`, `readRepoFile`, `requireClauses`, `rejectClauses` and `actualsInstrumentationRepoRoot`, as `TestProjectInceptionCompactCoordinator` and `TestInceptionPipelineKeepsActivationAndClosureRules` do.
  - It checks the size and lint limits (S6), the S2 clauses, the S4/S5 rejections and each S3 rule in its stated location.
  - Where an S2 consumer's own text can be read (for example `closure-feedback.md` "EARS R-NNN" and `tier-selection.md` "(lightweight)"), the test reads both sides, so neither can move alone.
  - Focused command: `go -C engine test -count=1 ./skills -run 'TestRequirementsFromTranscripts|TestLintBaseline|TestBaselineLintDecision|TestClarifyRequest'`.
  - Order of T3 before T4: `BaselineLintDecision` refuses any hard finding for a skill that is not on the baseline (`approval_write.go:57-60`), so the owner's approval then enforces the lint itself.

  **Estimate.** Not an estimator run; no actuals (n=0), low confidence.

  | Task | Authored lines (additions plus deletions, tests included) |
  |---:|---|
  | T1 | 180-320 |
  | T2 | 40-140 |
  | T3 | 10-30 |
  | T4 | 0 |
  | T5 | Log only |
  | Total | 230-490 |

  The total may exceed the 400-line heuristic. The owner-stated delivery is one PR, and the parent decides whether to disclose a size exception. Risk: item 3 (contracts other skills consume) for T1-T2; item 2 (approval lint gate) for T3.
