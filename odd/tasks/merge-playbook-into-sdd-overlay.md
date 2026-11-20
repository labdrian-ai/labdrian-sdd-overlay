# merge-playbook-into-sdd-overlay

Objective: merge the playbook wiki and agents (playbook v3.6.0, MIT) into labdrian-sdd-overlay as one tool, layered (option B: wiki = project definitions, ODD docs = per-feature execution, linked by decision ids), learning loop first. Requirements: memorycard #6142 (R-001..R-015); roadmap item 36, appendix-8 #6150.
Branch: `feat/playbook-merge` (from origin/main ce22eb3f) in the clone `~/labdrian-sdd-overlay-playbook-merge`; delivery strategy `ask-on-risk` (forecast above 400 authored lines, chain strategy still to be asked); test runner `go test ./...` in `engine/` (CI "Engine Tests"), `go run ./cmd/ gadu-generate --check`, `bash -n` and `shellcheck` for shell.

## Specs

- **S1** — One tool inside the overlay (R-010, R-008 scope). The owner said: "vamos a hacerle ingenieria inversa a las ultimas versiones de labdrian-sdd-overlay y playbook y ver como los MERGEAMOS en una unica herramienta", "lo que quiero es un merge en labdrian-overlay antes del lanzamiento de trysm", and "pero esque quiero que sea wiki y agents de playboook + labdrian-sdd-overlay". Scope: the merged tool carries the playbook wiki and the playbook agents together with the overlay's roles, inside this repository. Acceptance: no new standalone repository; every playbook capability that is imported is listed in `overlay.manifest` (manifest rule 8). No date applies: "mandale ahora pero no tenemos fecha".
- **S2** — The learning loop comes first. The owner said: "autorizo, el bucle de aprendizaje es prioridad". Scope: the loop of playbook-agents. Repeated review findings are stored as rows of `docs/wiki/evidence.md`. A kind seen in enough distinct reviews is consolidated one level up, and general kinds are proposed for the shipped `evidence-rules.md`. The rules of level 2 and up reach the write and review subagents when they start. Acceptance: S3 to S8 hold, and their tasks come before every other task of this document. No R-item of brief #6142 covers this spec (see L6).
- **S3** — Findings feed (design, assumption: the overlay review store is the source of findings). The loop is fed by the overlay review lifecycle. The source is the stored results `odd/review-results/<feature>/<head12>-<n>.json` written by `labdrian review record`, plus their `<head12>-<n>.verifier.json` written by `labdrian review verify`. A result yields the findings that count as evidence, each with a review identifier that stays the same on every run (assumption: `<feature>/<head12>-<n>`). Findings that are not evidence are left out. This applies the classify rule "A finding the review marks as not real, dismissed, or already fixed before the review is not evidence": a severe finding whose verdict is `not-reproduced`, and a finding whose `causal_disposition` is `pre-existing` or `base-only`. Acceptance: one deterministic listing per stored result. It names every finding it kept and every finding it left out, with the reason. A result that cannot be read is refused, never listed as empty.
- **S4** — Evidence store with a minimal wiki surface. The page is `<repo>/docs/wiki/evidence.md`, in the format the header of playbook `evidence.sh` defines (title `# Evidence`, tables `## Kinds` and `## Evidence`). The general rules live in `evidence-rules.md`, shipped next to the script. The store needs no other wiki file: playbook `format.md` says "`evidence.md` is optional: `evidence.sh` of `playbook-agents` checks and reads it ... and it does not change the format of the wiki", and `evidence.sh check` reads only the page and `evidence-rules.md`. Acceptance: `check`, `count`, `index` and `rule <kind>` behave as the header of `evidence.sh` states, in a repository with no other wiki file. Rows hold no secret and no customer data. Rows are only appended, except by consolidation (S8).
- **S5** — Classifier carrier. One agent writes the evidence page from the S3 feed, following playbook `classify.md`: its rules, its central test ("would the rule of this kind have prevented this finding?"), its steps and its format. It never commits, pushes or changes git state, and it changes only the page. Constraint: the overlay `reviewer` role cannot carry it. Its body says "Never edit, run, or delegate", and its tools are `Read, Grep, Glob` (`engine/agentgen/roles.go`).
  - The owner answered OD-10: "necesitamos un rol nuevo" (L9). Classify becomes a new overlay role.
  - Assumptions pending the owner's confirmation:
    - its name is `classifier`;
    - write maps to builder and review maps to reviewer, so there are no duplicate roles;
    - explore is unmapped (OD-10b).
  - Acceptance: the agent file is generated from `roles/<role>/body.md`, and `gadu-generate --check` passes (R-009).
- **S6** — Lifecycle trigger. A stored result whose S3 feed keeps at least one finding is classified once. It counts as classified when the page holds an Evidence row whose Review is that result's identifier. This mirrors playbook `review-ran.sh`, which blocks once a review "that ended with findings and started no" classify. Acceptance: the parent of the role chain launches the classifier after `record` / `verify`. A branch commit whose result is unclassified gets asked for its classification once per session and commit by the existing `labdrian review stop-hook`. A result with no kept finding is never asked about. The hook keeps exiting 0 on anything it cannot tell.
- **S7** — Rule injection. The index of level-2+ kinds (`evidence.sh index`: "- <kind> (level N): <applies when>"), and the command that prints one rule, reach the builder and the reviewer when they are launched, as playbook `rules-for-subagent.sh` does for write and review. Nothing reaches the other roles. Acceptance: with no level-2+ kind there is no text and no change to the prompt. The size the injection adds to a prompt is measured and reported, and the cap is OD-11. Injection never blocks or fails a launch. Injection by itself does not change the reviewer's JSON reply contract; the one change to that contract is the rule outcomes of S8 (b).
- **S8** — Consolidation, promotion, level history and rule outcomes. The owner approved this scope: asked "¿Actualizo T6 del documento con esos dos puntos?", the owner answered "deacuerdo" (L7). Points (a) and (b) are adopted from arXiv 2611.18856 "Self-Improvement Memory" (SIM) and arXiv 2611.23476 "GUARD"; the shaper has not read either paper. The requirement is the owner's.
  - Base. This follows playbook `prepare-project/references/evidence.md`:
    - For each "ready to consolidate" group (threshold 3 distinct reviews in `evidence.sh`), a level k+1 rule is proposed to the owner.
    - The owner's yes sets the level.
    - A general kind at level 2+ is proposed for the overlay's shipped `evidence-rules.md`.
    - A declined proposal is recorded and not made again.
    - Classify only appends, and consolidation is only ever proposed to the owner; both stay as in playbook.
  - (a) History of failed attempts per kind. Today step 5 overwrites the kind's row at level k+1 and deletes the level-k Evidence rows (git keeps them). Whoever drafts level k+2 then cannot see which earlier wordings already failed.
    - Required: per kind, an ordered record of each level it held, with the previous rule text, the period, and the distinct reviews that recurred despite it.
    - The record is written before any level-k row is deleted.
    - The drafter of the next level reads it before writing.
    - "Never weaken a rule into something vague" becomes checkable against it: the proposal cites every earlier wording of the kind and says what the new rule keeps and adds. The owner judges vagueness against that record (assumption: no automated vagueness check).
  - (b) Measure with a denominator. Today classify records only findings, so a kind that stops appearing cannot be told apart from a situation that did not arise.
    - Required: the review output carries one line for each injected rule (level 2+ kind) that matched the task, with the kind and an outcome: `followed`, `violated` or `not-applicable`.
    - Classify uses these lines: `violated` keeps counting as a finding of that kind, and `followed` is recorded as exposure. Assumption: `not-applicable` is not recorded.
    - This lets a later change tell "newly broken" from "never solved".
  - Exclusions, out of scope now:
    - replay or backtest of a rule on past reviews;
    - rule retirement;
    - rule pinning;
    - scoring or priority scoring;
    - parallel candidate generation;
    - any number from the papers as an acceptance criterion (the papers' gains come from automatic benchmark optimisation; this loop is human-approved).
  - Acceptance:
    - Nothing is consolidated or promoted without the owner's answer, or a delegation the owner gave.
    - After a consolidation, the history of the kind holds the replaced rule, its period and its review identifiers.
    - A proposal for level k+2 cites the history.
    - A review with an injected matching rule yields one outcome line per such rule.
    - A `violated` outcome is counted as a finding of its kind, and a `followed` outcome is counted as exposure per kind and level, separately from the finding count.
- **S9** — One memory store (R-001, with R-013 and R-015). Every playbook reference to Engram as a write target in imported content is replaced by memorycard. Imported text holds no persona or conversational voice.
  - The owner answered OD-4: "desde aca tomamos el playbook no hacemos upstream" (L8).
  - The playbook is absorbed once, as a snapshot: andresfrei/playbook commit `6de8566bf7f1150f49a75c1162c0f4cf87bf57c5`, tag v3.6.0.
  - Absorbed code is the overlay's own and may be modified. There is no upstream tracking and no vendor branch for it, unlike the gentle-ai upstream/main model.
  - Playbook's MIT LICENSE notice stays with the absorbed code (R-015).
  - Each absorbed file, or the directory that holds it, carries a provenance note naming the snapshot commit and the license.
  - Acceptance: no merged artifact names Engram as a write target. The license text and the provenance note are present from the first commit that absorbs playbook content.
- **S10** — One review gate (R-002). Exactly one gate holds a commit until review, combining playbook's review-before-commit hold with the overlay per-commit lifecycle. Acceptance: one commit attempt triggers one review decision, and the Risk trailer is still honored. Which gate wins on conflict is OD-6.
- **S11** — Decision-id links (R-003). Each ODD feature document links to the wiki decision ids it executes, and the builder guard still parses header, Specs, Tasks and Log. Acceptance: a feature document that carries decision ids passes the builder guard, and a wiki page resolves those ids.
- **S12** — Single install and wiki deploy (R-004, R-005). One path (`labdrian apply` / install-hooks) installs both overlay and wiki capabilities, replacing the plugin-marketplace install. It deploys `docs/wiki` with `.format` 5, the commit check hook, the merge step hook, and the skills prepare-project, write-docs, init and doctor. Acceptance: one command yields both on a target runtime, and after deploy a project can run init/doctor and write wiki pages in format 5.
- **S13** — Hooks and session text (R-006, R-007). There is one deduplicated set of Bash PreToolUse hooks (commit check, review hold, ask-before-delivery), as owned families in `~/.claude/settings.json`. One session-start text stays within playbook's word budgets (contract 270, rules 400, loaded at start 370/620) unless OD-8 relaxes them. Acceptance: no command triggers two hooks enforcing the same rule, hook families stay removable by owner, and a `verify.sh`-equivalent check passes on the merged text.
- **S14** — One agent set (R-008, R-009). Playbook's explore, write, review and classify agents and `models.conf` are reconciled with the overlay's role agents. Every playbook agent is mapped to a role or explicitly dropped. Acceptance: a mapping table exists with no unmapped agent, and `gadu-generate --check` is green in CI.
- **S15** — Cross-cutting, every task. Base is origin/main in a separate clone, and the dev checkout `~/labdrian-sdd-overlay` is never written (R-011). Commits use Conventional Commits with no AI attribution or Co-Authored-By (R-012). Push, PR and merge happen only on the owner's explicit decision (R-014). Persisted artifacts hold no persona text (R-013).

## Tasks

Order is execution order: the learning loop (T1-T6, T16, T17) first, per S2, then the roadmap slices re-ordered behind it. Each task is shaped as one reviewable work unit of about 400 authored changed lines. Line forecasts are pending the estimator. S1 and S15 are constraints every task is held to.

- [ ] T1 — S2-S3 — inline — findings feed: an engine function and a read-only verb (assumption: `labdrian review findings --feature <f> --stored <head12>-<n>.json`) that reads a stored result and its verdict through the existing store readers. It prints the kept findings with the review identifier and the left-out findings with their reason. Tests cover each skip rule and an unreadable result. Help text and `odd/review-results/README.md` are updated. Depends on no open decision. Risk expectation: none (adds a command; reuses the result/verdict readers `check` uses, so it must not change them) — shaped: L6 — commit: -
- [ ] T2 — S2, S4, S9 — inline — evidence store: absorb playbook `evidence.sh` and `evidence-rules.md` from snapshot `6de8566` (v3.6.0) as overlay-owned code.
  - They are the starting point and may be modified later (T6, T17 extend them). They stay the one shared parser, with no second Go parse of the format.
  - The MIT LICENSE notice and a provenance note name the snapshot commit.
  - Track them in `overlay.manifest` as overlay-owned, not as an upstream or vendor entry (assumption: management type `custom`).
  - Add tests for `check|count|index|rule` against a repository holding only `docs/wiki/evidence.md`, and lint with `bash -n`/`shellcheck` (`sh` and `awk` become runtime needs).
  - No open decision blocks it.
  - Risk expectation: item 5 (a deployed file and a new runtime dependency on `sh`/`awk`).
  - shaped: L8 — commit: -
- [ ] T3 — S5 — inline — add a new role that carries classify. Name proposed as an assumption: `classifier`.
  - Source: `roles/classifier/body.md`, absorbed and adapted from playbook `classify.md` of snapshot `6de8566`, with `${CLAUDE_PLUGIN_ROOT}` replaced by the overlay path of T2 and a provenance note.
  - Its row in `engine/agentgen/roles.go`.
  - The generated `agents/classifier.md`, `pi/agents/classifier.md` and `codex/agents/classifier.toml`.
  - `gadu-generate --check` green in CI.
  - Model and tools (proposal): sonnet, effort `medium`, as playbook `classify.md` declares. Tools: Read, Edit, Write, Grep, Glob, Bash, and no memorycard tool, so `role.body` drops the memory line (`roles.go` lines 226-237).
  - Body: it writes only `docs/wiki/evidence.md` and runs only `evidence.sh`, and it never commits, pushes or changes git state.
  - Enforcement: these limits are prompt-level only. The builder guard fails closed for the builder alone (`engine/cmd/roles_builder_guard.go`). `gate-task` appends the memory instructions block to every sub-agent, this one included, and T5's rule index does not reach it.
  - Position (CONFIRMED by the owner, L10: "va fuera"): outside the role chain, like the verifier, which `agentgen` generates but `engine/roles` does not list.
    - The parent launches it after `review record` / `verify` (T4).
    - `engine/roles` `Vocabulary`, `Catalog`, `optionalRole` and the chain transitions stay unchanged.
    - The in-chain alternative is dropped.
  - Still assumptions: the name `classifier` (working name), and write -> builder / review -> reviewer.
  - Differs from the reviewer: the reviewer is read-only (Read, Grep, Glob; "Never edit, run, or delegate") and judges a diff. The classifier writes one page and judges kinds, not code.
  - No open decision of its own.
  - Risk expectation: item 5 (a new agent generated for three runtimes and checked by CI's `gadu-generate --check`); own commit.
  - shaped: L10 — commit: -
- [ ] T4 — S6 — inline — lifecycle trigger: a step in `skills/role-chain/SKILL.md`. After `record` / `verify`, when T1 keeps findings, the parent launches the classifier with T1's output and the repository root. `labdrian review stop-hook` asks once per session and commit for an unclassified result with kept findings, in its existing reason format. Tests cover: asked, not asked when no finding is kept, not asked when an Evidence row carries the identifier, and exit 0 on unreadable input. Depends on T1-T3. Risk expectation: item 3 (stop-hook behavior and the role-chain skill are contracts others consume); own commit — shaped: L6 — commit: -
- [ ] T5 — S7 — inline — rule injection through `gate-task`: when the call's `subagent_type` is the builder or the reviewer, append the T2 `index` text and the `rule <kind>` command (absolute paths) to the prompt, in the same single rewrite as the memory instructions block. Nothing is appended when the index is empty. A reviewer body line says injected rules are review criteria inside its lens rows, with the reply shape unchanged. Measure and report the added size. Extend the gate-task golden tests. Blocked for its cap by OD-11, and for runtimes other than Claude Code by OD-7. Risk expectation: item 3 (gate-task contract pinned by `testdata/contract-golden`, reviewer prompt contract); own commit — shaped: L6 — commit: -
- [ ] T6 — S8 (base, a) — inline — consolidation at close with level history: a close step in the role-chain skill, run by the parent and not by the delivery sub-agent, which cannot ask the owner.
  - It runs `evidence.sh count`, proposes level k+1 rules group by group, applies the owner's yes, records declined proposals, and proposes general kinds for the shipped `evidence-rules.md`.
  - Before any level-k row is deleted, it appends the kind's history row. Design: a `## History` table in `docs/wiki/evidence.md`, `| Kind | Level | Rule | From | To | Reviews |`.
  - `evidence.sh check` is extended to validate that table. Today an unknown `##` section resets the section and its rows are skipped (`evidence.sh` lines 90-97).
  - A new `evidence.sh history <kind>` mode prints the ordered levels; the drafter reads it, and the proposal cites every earlier wording.
  - Tests cover: history written before deletion, check failing on a malformed history row, and the proposal citing history.
  - Depends on T2, T4. No replay, retirement, pinning or scoring.
  - Risk expectation: item 3 (the evidence page format read by `evidence.sh` and by the close step) and item 1 (the step deletes stored rows, and the history row must be written first); own commit.
  - shaped: L7 — commit: -
- [ ] T16 — S8 (b) — inline — rule outcomes in the review output contract:
  - An optional member of the reviewer reply carries one entry per injected matching rule, `{"kind","outcome": followed|violated|not-applicable}` (member name is an assumption).
  - Changes: the `engine/reviewresult` schema and rules (strict about unknown members), `roles/reviewer/body.md` and its generated files, `odd/review-results/README.md`, and the T1 feed extended to list outcomes.
  - Stored results written before the member existed still read and still clear `review check`.
  - Tests cover each outcome value, an unknown outcome refused, and an old result read.
  - Emitter (proposal, pending the owner's confirmation of review -> reviewer): the reviewer emits the lines. The classifier (T3) consumes them in T17.
  - Depends on T5. Blocked by OD-11 (the injected budget now also asks for outcomes).
  - Risk expectation: item 3 (the reviewer reply contract consumed by `record`, `verify`, `check`, `roles append` and CI anchor) and item 1 (the persisted format of stored results); own commit.
  - shaped: L9 — commit: -
- [ ] T17 — S8 (b) — inline — classify uses outcomes:
  - The classifier body counts `violated` as a finding row of the kind at its current level, and records `followed` as exposure. Design: a `## Exposure` table, `| Kind | Level | Review | Date | Outcome |`. `not-applicable` is not recorded (assumption).
  - A new `evidence.sh exposure` mode, next to the unchanged `count`, prints per kind and level the distinct reviews followed and violated.
  - `check` validates the table.
  - Tests cover each outcome and the separation of the two counts.
  - Consumer (proposal): the new classifier role of T3 reads the reviewer's outcome lines through the T1 feed.
  - Depends on T3, T16; no open decision of its own.
  - Risk expectation: item 3 (the evidence page format and the classifier contract); own commit.
  - shaped: L9 — commit: -
- [ ] T7 — S9 — inline — memory (roadmap slice S1): replace every Engram write target in absorbed playbook content with memorycard (in the snapshot only `plugins/playbook-agents/rules.md` names Engram). Check absorbed text for persona voice, and confirm the MIT notice and the provenance note cover every absorbed file. No open decision blocks it; it applies to what T2, T3 and T10 absorb, so it runs once content is absorbed. Risk expectation: none expected; item 3 if an imported skill already consumed by a runtime changes its memory calls — shaped: L8 — commit: -
- [ ] T8 — S10 — inline — one review gate (roadmap slice S2): combine playbook `review-before-commit.sh` with the per-commit lifecycle so that one commit attempt yields one review decision, Risk trailer honored. Blocked by OD-6. Risk expectation: items 2 and 3 (a guard and the review gate others consume); own commit — shaped: L6 — commit: -
- [ ] T9 — S11 — inline — decision-id links (roadmap slice S3): the feature-document place for wiki decision ids, builder guard parsing kept, and a resolver check against wiki pages. Blocked by OD-5. Risk expectation: item 3 (feature-document grammar read by the builder guard) — shaped: L6 — commit: -
- [ ] T10 — S12 — inline — wiki capabilities absorbed (roadmap slice S4, part a): the playbook-wiki skills prepare-project, write-docs, init and doctor, `.format` 5, and the commit check and merge step hooks, taken from snapshot `6de8566` as overlay-owned code with the MIT notice and a provenance note. They are tracked in `overlay.manifest` as overlay-owned, and Engram references are handled per T7. Blocked by OD-7. Risk expectation: item 5 (deployed content) — shaped: L8 — commit: -
- [ ] T11 — S12 — inline — single install path (roadmap slice S4, part b): `labdrian apply` / install-hooks deploys T10 and the T2/T3 loop pieces to the targets OD-7 names, replacing the marketplace install. Blocked by OD-7. Risk expectation: item 5 (installer and deploy); own commit — shaped: L6 — commit: -
- [ ] T12 — S13 — inline — deduplicated hook families (roadmap slice S5, part a): one set of Bash PreToolUse hooks (commit check, review hold, ask-before-delivery) as owned families in `engine/settings`, removable by owner. Blocked by OD-6, OD-7. Risk expectation: items 3 and 5 (settings.json format and hooks others consume); own commit — shaped: L6 — commit: -
- [ ] T13 — S13 — inline — session-start text (roadmap slice S5, part b): one merged text and a `verify.sh`-equivalent word check. Blocked by OD-8. Risk expectation: item 3 (session text every session loads) — shaped: L6 — commit: -
- [ ] T14 — S14 — inline — agent mapping table (roadmap slice S6, part a):
  - classify maps to the new role, as decided in L9 (the name `classifier` is an assumption).
  - write maps to builder and review maps to reviewer, as assumptions pending confirmation.
  - explore is pending OD-10b. The overlay has no read-only mapper role; the built-in Explore or the prototyper may cover it.
  - `models.conf` is reconciled with the model tiers of `engine/agentgen/roles.go`.
  - Blocked by OD-10b and by the owner's confirmation of the assumptions.
  - Risk expectation: none (documentation).
  - shaped: L9 — commit: -
- [ ] T15 — S14 — inline — generate the mapped agents (roadmap slice S6, part b): role bodies and `roles.go` rows for every agent T14 maps to a role not yet generated. The classifier is T3's. Under the assumptions only an explore role could remain, if OD-10b asks for one. `gadu-generate --check` green. Blocked by OD-10b, OD-7. Risk expectation: item 5 (agents deployed to runtime targets) — shaped: L9 — commit: -

## Log

- **L1** (2026-11-20, owner): "vamos a hacerle ingenieria inversa a las ultimas versiones de labdrian-sdd-overlay y playbook y ver como los MERGEAMOS en una unica herramienta"
- **L2** (2026-11-20, owner): "lo que quiero es un merge en labdrian-overlay antes del lanzamiento de trysm"
- **L3** (2026-11-20, owner): "mandale ahora pero no tenemos fecha"
- **L4** (2026-11-20, owner): "pero esque quiero que sea wiki y agents de playboook + labdrian-sdd-overlay"
- **L5** (2026-11-20, owner): "autorizo, el bucle de aprendizaje es prioridad"
- **L6** (2026-11-20, shaper): Design of T1-T15.

  **Sources read:**
  - memorycard #6142 (brief), #6150 (appendix-8), #6146 (pipeline state), #6155 (owner decisions), #868 (architecture), #867 (manifest rules).
  - playbook `plugins/playbook-agents/{agents/classify.md, agents/review.md, evidence.sh, evidence-rules.md, hooks/hooks.json, hooks/review-ran.sh, hooks/rules-for-subagent.sh, skills/review-change, skills/seed-evidence}`, `plugins/playbook-wiki/format.md`, `prepare-project/references/evidence.md`, `scripts/verify.sh` (budget lines).
  - overlay `roles/reviewer/body.md`, `engine/agentgen/roles.go`, `engine/cmd/gate_task.go`, `engine/cmd/binding_wiring.go`, `odd/review-results/README.md`.
  - Path note: the prompt's `docs/wiki/evidence-rules.md` does not exist in playbook. The general rules are `plugins/playbook-agents/evidence-rules.md` (19 kinds, all level 2 or 3). `docs/wiki/evidence.md` is playbook's own project page.

  **Steelman.** Playbook's loop is small, proven and already general:
  - One POSIX script that never writes counts and indexes an append-only page.
  - An agent applies one central test.
  - A hook gives subagents only an index, so rules cost context only when they match.
  - The overlay is a better host than playbook itself. Its findings are already structured JSON (severity, `causal_disposition`, verifier verdict), so "not evidence" becomes a deterministic filter (S3) instead of a model judgment over prose.

  **Choices, each with its tradeoff:**
  - (a) Feed from stored results, not from the reviewer. The reviewer cannot write or delegate, unlike playbook's review agent, which launches classify itself.
  - (b) Import `evidence.sh` verbatim as the one parser instead of a Go port. This follows the shipped rule `format-parsed-differently-by-two-scripts`. Tradeoff accepted: `sh`/`awk` become runtime needs, and it bends manifest rule 6 (engine is the control plane).
  - (c) The minimal wiki surface is `docs/wiki/evidence.md` alone, verified in `format.md` and in `evidence.sh` (`check` reads only the page and `evidence-rules.md`). So the loop does not wait for the full wiki or for OD-5.
  - (d) Inject rules through `gate-task`, not a new SubagentStart hook. gate-task already rewrites every Agent call once and matches `subagent_type` (`hook_translate.go:145`), it is engine-owned, and it adds no hook family (R-006). Tradeoff: it touches a contract pinned by golden tests, and the rules go into the prompt text instead of side context.
  - (e) Trigger through the role-chain step plus the existing stop-hook. A result counts as classified when an Evidence row carries its identifier, so no new file kind is added to `odd/review-results/`.
  - (f) Recommended carrier: a new role, since the reviewer is read-only. This is pending OD-10.

  **Strongest counter-argument.** Writing `docs/wiki/evidence.md` makes every evidence commit non-passive under the stop-hook's passive rule (paths only under `odd/`). The loop's own commits would each need a Risk trailer, sweep and review, and their review findings feed the loop again. This design keeps the passive rule unchanged (cost accepted). Widening it is OD-12.

  **Context budget:**
  - playbook `verify.sh` caps contract 270, rules 400, skill body 700, start load 370/620 words.
  - It only checks that `rules-for-subagent.sh` is wired (lines 161-180). No budget bounds the injected index (unverified beyond those lines).
  - With today's `evidence-rules.md`, the index holds 19 lines before any project kind.
  - Pi's bridge hung on a ~7 KB system prompt (`roles.go:40-44`). That applies only if injection ever reaches Pi.

  **Open decisions** (not answered here; each task names what it is blocked by):
  - OD-1/OD-2: what trysm is and its date. Blocks no task; scheduling stays pending.
  - OD-3: what "mandale ahora" sends. Blocks no task.
  - OD-4: upstream andresfrei/playbook stays the source of truth, or is forked or absorbed. Blocks T2, T3, T7, T10.
  - OD-5: wiki format 5 extended for decision ids. Blocks T9.
  - OD-6: which review gate wins. Blocks T8, T12.
  - OD-7: deploy targets for wiki and loop. Blocks T11, T12, T15, and T5 beyond Claude Code.
  - OD-8: whether playbook word budgets bind the session text. Blocks T13.
  - New, not in appendix-8: OD-10, which agent maps to which role. Blocks T3, T14, T15.
  - New: OD-11, the word or byte cap of the injected index. Blocks T5's cap.
  - New: OD-12, whether evidence-page-only commits become passive. Blocks nothing; the default keeps them reviewed.
  - OD-9 is settled: the fetch was done, and origin/main = ce22eb3f (#6155).

  **Learning-loop blockers.** OD-4 and OD-10 block it at T2/T3. The smallest safe slice that depends on no open decision is T1.

  **Brief gap.** #6142 has no R-item for the learning loop. Text to add, not added here: "R-016 scope: new-capability. Requirement: The merged tool SHALL learn from repeated review findings: findings of stored review results are classified into kinds on `docs/wiki/evidence.md`, kinds repeated across distinct reviews are consolidated one level up with the owner's approval, general kinds are proposed for the shipped `evidence-rules.md`, and rules of level 2 and up reach the builder and reviewer agents when they start. This item is delivered before every other item ("autorizo, el bucle de aprendizaje es prioridad"). Acceptance: a stored result with kept findings yields Evidence rows once; a level-2 kind appears in the builder's and reviewer's prompt; no consolidation happens without the owner's answer."
- **L7** (2026-11-20, shaper): Owner decision, verbatim: asked "¿Actualizo T6 del documento con esos dos puntos?", the owner answered "deacuerdo".

  **The two points.** Adopted from arXiv 2611.18856 "Self-Improvement Memory" (SIM) and arXiv 2611.23476 "GUARD", both relayed by the coordinator and not read by the shaper:
  - (a) Keep the history of failed attempts per kind.
  - (b) Measure with a denominator: followed / violated / not-applicable outcomes per injected rule.

  **Exclusions.** Replay or backtest, retirement, pinning, scoring, parallel candidate generation, and paper numbers as acceptance criteria.

  **Changed.** S8 was rewritten. S7's last sentence was rewritten, because S7 had said the reviewer reply contract is unchanged, and (b) changes it. T6 was reopened and re-shaped for (a). Point (b) went to new tasks T16 (the reply contract and stored format) and T17 (classify and exposure counting). One unit would exceed about 400 changed lines, and each has its own high-risk item. Every other task is unchanged.

  **Design.** The history and exposure tables are added to `docs/wiki/evidence.md` as `## History` and `## Exposure`. `evidence.sh` today skips the rows of an unknown `##` section (lines 90-97), so `check` must be extended to validate both tables.

  **Risks, flagged and not decided:**
  - (a) T16 changes the reviewer's output contract, which `engine/reviewresult` holds strictly ("no member the contract does not name", `odd/review-results/README.md` rule 3). That is high-risk item 3. It is also item 1, because stored results gain a member and old results must keep reading.
  - (b) The injected index budget (OD-11) now also carries the request for outcome lines.
  - (c) Which role emits the outcome lines depends on OD-10.
  - (d) `evidence.sh count` counts distinct reviews of findings only (Evidence rows grouped by kind and level). It stays as it is. A separate `exposure` mode next to it must count distinct reviews with `followed` and with `violated` per kind and level, so the threshold for consolidation is not changed by exposure.

  **Proposed addition to brief #6142** (not edited; R-016 is in L6): "R-017 scope: feature. Requirement: The learning loop SHALL keep, per kind, an ordered history of each level it held (the replaced rule text, its period, and the distinct reviews that recurred despite it), written before consolidation deletes level rows and read by the drafter of the next level; and the review output SHALL carry one outcome line (followed, violated, not-applicable) for each injected rule that matched the task, with violated counted as a finding and followed recorded as exposure, separately from the finding count. Out of scope: replay or backtest, rule retirement, pinning, scoring, parallel candidate generation, and paper figures as acceptance criteria. Source: arXiv 2611.18856 (SIM) and 2611.23476 (GUARD), adopted by the owner ("deacuerdo", 2026-11-20). Acceptance: after a consolidation the kind's history holds the replaced rule, period and review identifiers; a review with a matching injected rule yields one outcome line per rule; exposure and finding counts are reported apart."
- **L8** (2026-11-20, shaper): Owner decision, verbatim, answering OD-4: "desde aca tomamos el playbook no hacemos upstream".

  **OD-4 is ANSWERED.** The playbook is absorbed once as a snapshot, andresfrei/playbook commit `6de8566bf7f1150f49a75c1162c0f4cf87bf57c5`, tag v3.6.0 (2026-11-20, merge of PR #45), with all three plugin manifests at 3.6.0. These facts were relayed by the coordinator and not checked by the shaper.
  - Absorbed code is the overlay's own and may be modified.
  - There is no upstream tracking and no vendor branch, unlike the gentle-ai upstream/main model. So manifest rules 3 and 4 (upstream separation, no vendor mutation) do not apply to it.
  - The MIT LICENSE notice and a provenance note naming the snapshot stay with it (R-015).

  **Changed.** S9 was rewritten. T2, T3, T7 and T10 were re-shaped: the OD-4 blocker was removed, and "verbatim import" became "absorb and own, with provenance".
  - L6 choice (b) still holds as one shared parser, now modifiable.
  - In the snapshot, only `plugins/playbook-agents/rules.md` names Engram. That file belongs to the session text (T13), so T7 reaches it when it is absorbed.

  **Startable now, with no open-decision blocker:**
  - T1: no dependency.
  - T2: no dependency.
  - T7, once content is absorbed.
  - T4 and T6 have no decision blocker, but they wait on T3, which OD-10 blocks.

  **Still open:**
  - OD-1/OD-2 and OD-3 block no task.
  - OD-5 blocks T9.
  - OD-6 blocks T8 and T12.
  - OD-7 blocks T10, T11, T12, T15, and T5 beyond Claude Code.
  - OD-8 blocks T13.
  - OD-10 blocks T3, T14, T15, T16 and T17.
  - OD-11 blocks T5's cap and T16.
  - OD-12 blocks nothing.
- **L9** (2026-11-20, shaper): Owner decision, verbatim, answering OD-10. The owner was asked: classify has no equivalent in the 8 overlay roles; proposal: add it as a new role, with write -> builder and review -> reviewer. The owner answered "necesitamos un rol nuevo".

  **Settled.** Playbook `classify` becomes a new overlay role.

  **Not settled.** These stay assumptions pending the owner's confirmation:
  - (1) write -> builder and review -> reviewer.
  - (2) explore is unmapped: OD-10b, new and open, which blocks T14 and T15.
  - (3) the name `classifier`. Overlay roles are nouns (builder, shaper, reviewer, sweeper), while playbook's `classify` is a verb.

  **Changed.** S5 was rewritten. T3, T14, T15, T16 and T17 were re-shaped as L9. T4 and T5 are unchanged: T4 already says "the classifier", and T5 targets builder and reviewer as assumption (1) says. No task was split. T3 stays one unit under the off-chain proposal; the in-chain alternative would be split into its own task.

  **Design proposals, from the code read:**
  - `engine/roles/roles.go` `Vocabulary` holds prototyper, shaper, estimator, builder, sweeper, polisher, reviewer and delivery. Only sweeper and polisher are `Optional`.
  - `engine/roles/chain.go` requires seq 1 from prototyper or shaper and `from_role` equal to the previous `to_role`. It allows the single backward step reviewer -> builder.
  - The verifier is generated by `engine/agentgen/roles.go` but is not in `Vocabulary`, so an off-chain agent launched by the parent already exists in this repository.
  - Proposal: the classifier is off-chain like the verifier, launched by the parent after `review record` / `verify`. Valid chains and `review record`'s `to_role` stay unchanged. Classification is evidenced by the Evidence rows carrying the review identifier (S6), not by a handoff record.

  **Strongest counter-argument.** Off-chain, no chain record proves the classifier ran. Only the stop-hook question of T4 enforces it. An optional chain role would be checked by `roles append`, at the cost of changing a vocabulary that chains, validators and CI golden files consume. That choice is left to the owner.

  **Risks, not decided:**
  - Adding a role changes generated agents for three runtimes, which CI checks: item 5. It also changes `engine/roles` `Catalog`, which other commands consume, if the in-chain alternative is chosen: item 3. T3 gets its own commit.
  - The classifier's write limits are prompt-level only: no guard enforces "only `docs/wiki/evidence.md`, never commit".

  **Startable now, with no open-decision blocker:**
  - T1, T2 and T3. T3's name and position are assumptions the owner should confirm before its commit.
  - T7, once content is absorbed.
  - T4 after T1-T3, T6 after T2 and T4, and T17 after T3 and T16.

  **Still open:**
  - OD-10b blocks T14 and T15.
  - OD-5 blocks T9.
  - OD-6 blocks T8 and T12.
  - OD-7 blocks T10, T11, T12, T15, and T5 beyond Claude Code.
  - OD-8 blocks T13.
  - OD-11 blocks T5's cap and T16, and through T16 it also holds T17.
  - OD-12 blocks nothing.
  - OD-1/OD-2/OD-3 block nothing.
- **L10** (2026-11-20, shaper): Owner decision, verbatim, answering whether the classifier goes outside the chain (like the verifier) or inside, between reviewer and delivery: "va fuera".

  **Settled.** The new role stays outside the role chain. `engine/roles` `Vocabulary`, `Catalog`, `optionalRole` and the chain transitions stay unchanged, and the in-chain alternative of L9 is dropped. So T3's Risk is item 5 only, and T3 stays one unit.

  **Still assumptions.** The name `classifier` (working name, neither rejected nor explicitly confirmed), and write -> builder / review -> reviewer. OD-10b stays open.

  **Changed.** Only T3 was rewritten and re-shaped as L10. No other task or spec text named the in-chain option; the L9 sentences about it stay as history in the append-only Log.

  **Startable now, with no open decision of their own:**
  - T1, T2 and T3.
  - Then, by dependency only: T7 once content is absorbed, T4 after T1-T3, T6 after T2 and T4, and T17 after T3 and T16.
