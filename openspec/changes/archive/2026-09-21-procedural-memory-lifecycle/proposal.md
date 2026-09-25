# Proposal: Procedural memory lifecycle (draft, register, promote, revise, retire)

## Intent

Item 30 (`procedural-candidate-detection`) gives the overlay a durable promotion-candidate store: an Engram record that knows a pattern has recurred N times and is not already covered by a registered skill. Nothing consumes that record. An emitted candidate today is a dead end: no drafting procedure, no machine-enforced authoring standard, no way to put a skill where the runtimes will load it, no signal that a registered skill is failing to teach, and no retirement path. The procedural-memory loop stops one step after detection.

This change closes the loop for roadmap items 31-35 under the two-tier autonomy premise (Engram #3426, owner-confirmed 2026-09-18):

- **Project tier (agent-autonomous).** Inside the project it is working in, the agent drafts, lints, registers, revises and retires skills without a per-skill human approval. The blast radius is one repository. Human approval is replaced by mandatory machine guardrails.
- **Global tier (human-gated).** Promotion of a project skill into the overlay `skills/` tree, which deploys to every runtime and every project, stays a human decision through the existing `engine skills add` (`AddCore`) path.

The mandatory guardrails from #3426 are the contract of this change, not optional hardening: (1) a `LintSkill` hard gate; (2) declared provenance frontmatter plus ownership-by-hash, so the agent mutates only skills it placed and nobody edited since; (3) one git commit per registration, with `git revert` as rollback; (4) the post-promotion occurrence trigger for revision; (5) a do-not-capture list gating candidate quality.

Success is an emitted candidate that becomes a linted, project-local skill loaded by Claude Code, Pi and (once confirmed) Codex, recorded with provenance and hash, revisable only while the agent still owns it, promotable to the overlay only by a human, and retirable only through a report plus a decision, with every transition appended to the record's `History`.

Supersession: the Hermes synthesis (Engram #3420) section 3.4 assumed human-only registration. At the project tier that assumption is superseded by #3426; its approval procedure survives only for project-to-overlay promotion.

## Scope

### In Scope

- **Item 31 — drafting.** Contract amendment to `skills/_shared/procedural-candidate-detection.md`: do-not-capture list (environment-dependent failures, negative claims about tools, transient errors, one-off narratives, unresolved failures presented as workflow); lesson shape for `Summary` (imperative rule plus one clause of why, no observation ids, dates or PR numbers); `Disposition: new | extend:<skill-id>` (extend-before-create); a draft record at `procedural/drafts/{kind}/{slug}` holding the complete SKILL.md text or a unified diff; an extended `Status` vocabulary; an append-only `History` list on candidate and draft records.
- **`LintSkill`.** A pure, stdlib-only `LintSkill(frontmatter, body) (hard []error, warnings []Warning)` in `engine/skills`, with a thin `engine skills lint <path>` CLI. Hard: frontmatter fence and required fields (`name`, `description`, `license`, `metadata.author`, `metadata.version`), description one line and within the MUST bound, body within the hard budget. Advisory: description SHOULD bound, section order, incident-log shape, banned shell utilities in backticks, absolute home-path leak. One source of truth for these rules (decision (a)).
- **Item 33 — project-tier registration (agent-owned).** A sibling of `engine/skills/install.go` that writes a linted SKILL.md into each project-local runtime target directory, reusing the R-055 traversal guard and validate-before-write discipline; stamps declared provenance (`metadata.provenance: procedural`, `metadata.candidate: <topic key>`); records id, provenance, candidate key and sha256 in a project lock file (decision (b)); ends in one git commit that contains only the registration paths (decision (c)).
- **Item 32 — global promotion (human).** Project-to-overlay promotion as a documented procedure over the existing `engine skills add` / `AddCore`, with `LintSkill` added as a hard gate inside `AddCore`. No new approval machinery, no TTL, silence is not consent.
- **Item 34 — revision.** Ownership-by-hash check (disk hash must equal the recorded hash, otherwise the skill is human-owned and the agent stops); `OccurrencesSincePromotion >= 2` as the revision trigger; revision drafts follow the item-31 draft path and the item-33 lint-register-commit path at the project tier, and the human path at the global tier.
- **Item 35 — retirement.** A report-only detector over the paths and commands a procedural skill names, reusing `longterm-mem/internal/staleness`, plus candidate `LastObserved` age and supersession by a newer skill. Removal is a decision: agent-owned at the project tier (commit, revertable), human `engine skills remove` / `RemoveCore` at the global tier. Consolidation records `AbsorbedInto: <skill-id>`, verified to exist with `MatchCandidate`.
- **Codex discovery check.** An explicit, recorded smoke test that Codex 0.148.0 loads project skills from `.agents/skills/<id>/`, performed before any code relies on it (see Approach, step 1).

### Out of Scope

- **Item 30 work itself** (emission table, `MatchCandidate`, contract section 6). Consumed, not built here.
- **Rejected Hermes machinery** (#3420 recommendation 12): skills hub federation, sync plane, per-skill lock files and batch snapshots, content-addressed blob ledger, regex threat scanner, background curator fork, aux-LLM approval guardian, time-based automatic archiving, the 60-character description cap.
- **Any draft file under `skills/`.** Drafts are Engram records only; a file there would fail the `UNREGISTERED_ON_DISK` ondisk gate.
- **Registry schema change.** `Entry` and the strict `parse.go` key set stay as they are; provenance lives in SKILL.md frontmatter and the project lock file.
- **Go-side writes into Engram**, new hooks, and skill-load telemetry. Record transitions stay agent-driven through MCP tools, as in item 30.
- **Evidence scrubbing (OQ-8).** Drafts inherit the episodic layer's hygiene; the `LintSkill` absolute-path warning is the only scrub, and the contract says so.
- **Consolidation sweep** (#3420 recommendation 10) and opencode targets (opencode is not used).
- **Changes to the ondisk gate.** Project-local directories are outside its scope and stay there.

## Capabilities

### New Capabilities

- `skill-lint`: the `LintSkill` hard/advisory rule contract, its single rule source, and the `engine skills lint` CLI exit contract.
- `procedural-skill-drafting`: do-not-capture list, lesson shape, `Disposition`, draft record topic key and body, `History` append-only rule, extended `Status` vocabulary.
- `procedural-skill-registration`: project-tier multi-target registration, provenance frontmatter, project lock file, hash recording, commit scope; the human project-to-overlay promotion procedure.
- `procedural-skill-maintenance`: ownership-by-hash, revision trigger and path, report-only retirement detection, retirement and `AbsorbedInto` rules.

### Modified Capabilities

- `skill-lifecycle`: `AddCore` gains a `LintSkill` hard gate; a SKILL.md that fails hard lint is refused before any manifest or registry write.
- `procedural-candidate-detection`: status vocabulary, record fields (`Disposition`, `History`, `Registered`/`Promoted` hash line, `OccurrencesSincePromotion`, `AbsorbedInto`) and the do-not-capture list. This capability is not yet archived to `openspec/specs/`; the delta is written against item 30's change spec and must be rebased on it once item 30 archives.

## Approach

1. **Confirm the premise before building on it (first slice).** Run and record a Codex smoke test: place a throwaway skill under a temp repo's `.agents/skills/`, confirm Codex lists or loads it, and record the command and output in the design. If Codex does not discover it, Codex support is recorded as a known gap; the target set is unchanged because Pi reads `.agents/skills/` regardless (verified in installed Pi docs) and Claude reads `.claude/skills/`.
2. **`LintSkill` first**, because drafting, registration, promotion and revision all gate on it. Pure function beside `parse.go`, no filesystem, stdlib only (ADR-15, `TestZeroFetchImportAllowlist`). Body budget uses a documented deterministic proxy, since `engine/` cannot carry a tokenizer.
3. **Contract amendment as prose plus a contract-artifact test**, following the item-30 pattern (`procedural_candidate_contract_test.go`). Record transitions remain agent-driven MCP writes; `History` is appended, never rewritten, to survive Engram's overwrite-on-upsert.
4. **Project-tier registration as a sibling of `install.go`**, not a modification: the existing installer is Claude-only and registry-driven, and its behavior must not change. The sibling takes explicit target roots, reuses the traversal guard, validates before writing, writes the lock file, and prints the exact paths it wrote. The git commit is performed by the agent procedure with an explicit pathspec, because `engine/` bans `os/exec`; the procedure refuses to commit when unrelated changes are staged.
5. **Global promotion reuses `AddCore` unchanged apart from the lint gate.** No new CLI.
6. **Revision and retirement reuse existing seams**: the hash check reuses the lock-file hash; the retirement detector lives in the `longterm-mem` module, because Go forbids importing `longterm-mem/internal/staleness` from the `engine` module; `RemoveCore` and `MatchCandidate` are reused as-is.
7. **Tests use temp directories only.** No test reads or writes the live `.claude/skills/`, `.agents/skills/`, `.pi/`, `skills-lock.json`, `$HOME`, or a live runtime binary (tests-touch-live-Pi precedent). Strict TDD applies to Go code; prose contract sections are gated by contract-artifact tests plus an acceptance checklist run during `sdd-verify`.

### Slice plan (sequential; one slice at a time: apply, review, PR, merge; never parallel)

Each slice targets at most about 400 authored changed lines; estimates are for `sdd-tasks` to refine.

| # | Slice | Content | Estimate | Depends on |
|---|---|---|---|---|
| 0 | item-30 completion | item 30 phases 2-4 merged (owned by `procedural-candidate-detection`, not this change) | n/a | none |
| 1 | `skill-lint` | Codex smoke-test record; `LintSkill` + tests; `engine skills lint`; style-guide source-of-truth wiring | ~350 | 0 |
| 2 | `drafting-contract` | contract sections: do-not-capture, lesson shape, `Disposition`, draft record, `History`, status vocabulary; contract-artifact test | ~250 | 1 |
| 3 | `project-register-core` | pure multi-target register planner, lock-file read/write, provenance stamping, hash; tests over temp dirs | ~350 | 2 |
| 4 | `project-register-cli` | CLI wiring, lint gate, commit procedure in the contract, acceptance checklist | ~250 | 3 |
| 5 | `global-promotion` | `LintSkill` gate in `AddCore`; human promotion procedure | ~200 | 4 |
| 6 | `revision` | ownership-by-hash check, `OccurrencesSincePromotion` trigger, revision path | ~300 | 5 |
| 7 | `retirement` | report-only detector in `longterm-mem`, retirement and `AbsorbedInto` rules | ~350 | 6 |

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `engine/skills/lint.go` (new), `lint_test.go` | New | `LintSkill` pure function and tests |
| `engine/skills/` register sibling of `install.go` (new) | New | Project-tier multi-target registration and lock file |
| `engine/skills/install.go` | Unchanged (reused) | Traversal guard and copy discipline reused, behavior untouched |
| `engine/skills/lifecycle.go` | Modified | `LintSkill` gate inside `AddCore`; `RemoveCore` reused |
| `engine/skills/match.go` | Reused | `MatchCandidate` (item 30) for `AbsorbedInto` checks |
| `engine/cmd/...` | Modified | `skills lint` and project-register subcommands |
| `skills/_shared/procedural-candidate-detection.md` | Modified | Sections after item 30's section 6 |
| `skills/skill-creator/SKILL.md`, `skills/skill-creator/references/skill-style-guide.md`, `docs/skill-style-guide.md` (absent) | Modified | Resolve the dangling normative reference per decision (a) |
| `longterm-mem/` (new subcommand over `internal/staleness`) | New | Report-only retirement detector |
| Consumer repos: `.claude/skills/`, `.agents/skills/`, project lock file | New (runtime writes) | Agent-registered project skills |
| Engram `procedural/drafts/{kind}/{slug}` | New | Draft records |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Item 30 phases 2-4 slip or change the record schema | Med | Hard dependency; slice 1 does not start until item 30 is merged; drafting contract written against the merged text |
| Codex does not load `.agents/skills/` | Med | Slice-1 smoke test; Codex target is droppable without affecting Claude or Pi |
| Creating a project skill dir makes Pi prompt for project trust on the user's next start (`.pi/skills` and `.agents/skills` both require trust) | High | Accepted in revised decision (e): one prompt per project; disclosed in the registration output |
| Autonomous registration commits unrelated staged work in a consumer repo | Med | Explicit pathspec commit; refuse when unrelated changes are staged; acceptance scenario for it |
| Agent overwrites a human-edited skill | Med | Ownership-by-hash hard check; mismatch makes the skill human-owned |
| Low-quality or log-shaped skills accumulate at the project tier without a human in the loop | Med | Do-not-capture list, lesson shape, `LintSkill` incident-log warning, extend-before-create, git revert |
| `OccurrencesSincePromotion` is a proxy, not load telemetry | Med | Accepted and disclosed; no telemetry exists |
| Body token budget approximated without a tokenizer | Low | Documented deterministic proxy, tested at the boundary |
| Tests touch live user directories or runtimes | Low | Temp-dir-only rule stated above and asserted in tests |
| On-disk roadmap mirror (v7, 25 items) does not list items 30-35 | Low | Engram roadmap is canonical; flagged for a roadmap-maker regeneration, not fixed here |
| Hybrid persistence partial (Engram save refused by multiple active sessions) | Med | File is canonical for this phase; orchestrator mirrors to Engram |

## Rollback Plan

- Every slice is its own PR; revert slice PRs in reverse order (7 to 1).
- Go code is additive except the `AddCore` lint gate (slice 5), which reverts with its PR and restores the prior `AddCore` behavior.
- Contract amendments are additive sections; reverting them leaves item 30's sections 1-6 intact.
- In consumer repos, each agent registration, revision or retirement is one commit; `git revert <commit>` restores the previous state. Lock-file entries revert with the same commit.
- Draft and candidate records are inert Engram data under their own topic keys; they can be left in place.

## Dependencies

- **dependsOn: `procedural-candidate-detection` phases 2-4** (emission decision table, `MatchCandidate`, contract section 6) merged to `main`. No apply of this change runs before that.
- Engram MCP tools in each runtime (existing hard dependency).
- Existing seams: `engine/skills/install.go` (R-055 guard), `AddCore`/`RemoveCore`, `ParseRegistry`, `NormalizeSlug`, `longterm-mem/internal/staleness`.
- Codex 0.148.0 project-skill discovery (to be confirmed in slice 1). Pi discovery of project `.agents/skills/` (after project trust) verified in installed Pi docs.
- No new external dependency, no network access, no Go-side Engram write path.

## Decisions (owner-accepted 2026-09-18)

The owner accepted all seven recommendations below as stated on 2026-09-18. The "Recommendation" column is now the decision; `sdd-spec` and `sdd-design` must treat it as binding.

| # | Decision | Recommendation | Tradeoff |
|---|---|---|---|
| (a) | Style-guide source of truth: prose document as source, or Go rules as source with a generated doc | Go rules as source: `LintSkill`'s rule table is authoritative, and the style guide's machine-checkable section is generated from it and asserted by a test; human-judgment guidance stays prose. Make `skills/skill-creator/references/skill-style-guide.md` (exists) the single prose file and drop the dangling `docs/skill-style-guide.md` reference | Removes drift and the dangling reference; costs a small generator/golden and makes lint-rule changes code changes. The alternative (doc as source, Go mirrors it) is cheaper now but has two places to update and nothing to catch drift |
| (b) | Project lock file: reuse `skills-lock.json` or an overlay-owned file | Overlay-owned file (for example `.labdrian/procedural-skills.lock.json`), leaving `skills-lock.json` to the external installer | `skills-lock.json` belongs to another tool (live `archify` entry, `sourceType: github`); writing our entries into it risks being rewritten or rejected by that tool and mixes ownership. Cost: one more file per consumer repo |
| (c) | Registration commit automatic or left staged | Automatic, scoped to the registration paths only, refusing when unrelated changes are staged | #3426 names "every registration is a git commit" as a guardrail; staging only leaves the rollback unit undefined and the loop human-bottlenecked. Cost: the agent writes commits on the user's current branch in consumer repos |
| (d) | Project-tier `Status: registered` distinct from global `promoted` | Yes, distinct: `registered` (project, agent) and `promoted` (overlay, human) | Keeps the two trust tiers visible in every record and in `History`, and lets revision rules differ per tier. Cost: one more state in the transition table |
| (e) | Target directories | **Revised 2026-09-18 (owner-accepted):** write `.claude/skills/` (Claude Code) and `.agents/skills/` (Pi always; Codex too if the slice-1 smoke test confirms it). Never write `.pi/skills/` | The original recommendation (`.pi/skills/` to avoid Pi's trust prompt) rested on a false premise: installed Pi docs (`docs/security.md`, `docs/skills.md`) list both `.pi/skills` and project `.agents/skills` as trust-requiring resources, and project skills load only after trust. One Pi-visible copy avoids Pi's duplicate-name warning; the `.agents/skills/` target is no longer droppable because Pi needs it regardless of the Codex result. Cost: Pi prompts for project trust once per project |
| (f) | Behavior in the overlay repository itself, where "project" and "global" meet | Project-tier registration writes only runtime project dirs (`.claude/skills/`, `.agents/skills/`), never `skills/`; promotion to `skills/` stays human | Consistent with the two tiers; the overlay repo gets agent skills that are not deployed globally. The alternative (disable project tier in the overlay repo) is safer but removes the loop where most procedural knowledge is generated |
| (g) | `metadata.author` on agent-authored skills | A constant (for example `labdrian-overlay procedural`) rather than the OS or GitHub user | Avoids leaking identity and marks machine authorship; a human who takes ownership changes it |

## Success Criteria

- [ ] No slice of this change is applied before item 30 phases 2-4 are merged.
- [ ] The Codex `.agents/skills/` discovery result is recorded with command and output before any registration code targets it.
- [ ] `LintSkill` table tests cover every hard rule and every warning at their boundaries; its rules have one source of truth, and a test fails if the documented rules drift from the code.
- [ ] `AddCore` refuses a SKILL.md with a hard lint error before any manifest or registry write.
- [ ] A registration test over temp dirs writes the skill to every configured target, records provenance, candidate key and sha256 in the project lock file, and rejects traversal attempts.
- [ ] The registration procedure produces exactly one commit containing only registration paths, and refuses when unrelated changes are staged.
- [ ] A revision attempt on a skill whose disk hash differs from the recorded hash is refused and reported as human-owned.
- [ ] The retirement detector is report-only (no file or record mutation) and flags a skill that names a path removed in git history.
- [ ] Candidate and draft records carry a `History` that only grows across transitions.
- [ ] No file is written under `skills/` except through the human `engine skills add` path; no test touches live user directories.
- [ ] `cd engine && go vet ./... && go test ./...` and the `longterm-mem` test suite pass.
