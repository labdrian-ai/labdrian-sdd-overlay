# clarify-request: rewrite prespec-malandra as a grounded, short clarification loop

Branch `feat/phase9-clarify-request` (base `f13b2c7f`). Delivery: single PR. Test runner: `go test ./skills/` in `engine` (prose pins, strict TDD).
Owner decisions D1-D5 (2026-10-23, Engram #4426). Diagnosis workflow `wf_b1a93981-f03`, root causes RC1-RC7.

## Specs

- **S1 (rename, D5 b).** The skill is renamed now, as `clarify-request` (owner: "a"). `skills/prespec-malandra/` is removed and `skills/clarify-request/SKILL.md` replaces it, with `name: clarify-request`. `skills.registry.yaml` (id and path), `overlay.manifest` (its rows) and the approval baseline (`engine/skills/approval_baseline.go`: the `prespec-malandra` entry is dropped and its count pin is updated) follow the rename. Help and comment text that says the `prespec` verbs serve the `prespec-malandra` skill (`bin/lib/usage.sh`, `bin/lib/engine_verbs.sh`) states that no skill calls them since `clarify-request`. Out of scope: the top-level `prespec-malandra/` package (README, SPEC, LICENSE), inline test fixtures that use the name only as sample data, and history under `openspec/`, `odd/` and `docs/`.
- **S2 (inline, D4 a).** The skill runs inline in the parent conversation, not as a sub-agent. The line "You are a sub-agent. Do not delegate further." and any sub-agent framing are gone.
- **S3 (the loop).** The owner's complaint (verbatim): "hace preguntas fuera de contexto una repetidera  falta fe entendimiento parece un psicologo barato". The loop is:
  1. Ground: before asking anything, read the request, `mem_search` for the project and topic, the project manifest when one exists, and the files the user named.
  2. Restate: 3-6 lines that say what was understood, each tagged `said` (the user's words) or `inferred` (from the repository or memory, naming the source).
  3. Ask only the gaps that block a decision. Each question quotes the user's words or a repository artifact it is about. One question per message.
  4. Update the restatement with each answer. Never ask again about something already answered or already found in the grounding.
  5. Hard stop at 3 questions in total. After that, record any open gap as an assumption and stop asking.
- **S4 (checklist, D2 b).** The 10-cell grid is replaced by a hidden checklist of 6 items: goal, current gap, constraints, done-when, alternatives, risks. Each may be `n/a`. The items are not ranked and are never shown to the user as a grid or a score. `references/coverage-taxonomy.md` is deleted.
- **S5 (Mom-Test, D1 b).** Mom-Test framing (past behaviour, not hypotheticals) is used only when the user describes a third party's problem (a client, a user, a stakeholder). It is never used on the user's own request.
- **S6 (what goes).** The skill calls no `labdrian prespec` verb (`rank`, `lint`, `readiness`, `brief`). It keeps no anti-solution bounce, no business-archetype menu, no question lint that bans the repository's own vocabulary (API, module, plugin), and no readiness gate or score.
- **S7 (output).** The result is a brief: the final restatement, the checklist (with `n/a` items), the questions asked with their answers, and the assumptions. When the user confirms it, the brief is saved with `mem_save` under topic `project/{project}/clarify/{YYYYMMDD-HHMM}`, `type: architecture`, `capture_prompt: false`. The topic never starts with `sdd/`. The skill never derives a change-name. The hand-off to `requirements-from-transcripts` is unchanged (it owns the change-name and the EARS requirements).
- **S8 (approval).** `clarify-request` has no baseline entry, so it needs an owner `labdrian skills approve --id clarify-request --approver <name>`. The builder never runs it.

## Tasks

| ID | Specs | Route | Status | Commit / evidence |
|---|---|---|---|---|
| T1 | S1-S7 | delegated builder (owner L1 standing: implementations by Sonnet at max effort) + independent verifier (item 3: a mirrored skill prompt consumed on four runtimes is renamed and its contract changes) | open | |
| T2 | S8 | owner | open | |
| F1 | D3 a | separate slice, after this one merges: retire `labdrian prespec rank/lint/readiness` (and `brief`, now unused) after checking other consumers | open | |

## Log

- **L1** (2026-10-24, owner): "si arranca por ahi" (start the clarify-request rewrite). Standing: "ve en ultracode con opus high para las verify y sonnet max para implementaciones".
- **L2** (2026-10-23, owner): "la prespec-malandra hay que revisarla porque funciona como la mierda tambien"; "hace preguntas fuera de contexto una repetidera  falta fe entendimiento parece un psicologo barato". Decisions D1 b, D2 b, D3 a, D4 a, D5 b, name "a" (`clarify-request`).
- **L3** (2026-10-24, parent): consumers mapped on `f13b2c7f`. No other skill reads a `prespec` topic or names the skill; the live references are `skills.registry.yaml:352`, `overlay.manifest:66,119`, `engine/skills/approval_baseline.go:35`, `bin/lib/usage.sh:113` and `bin/lib/engine_verbs.sh:13`. `labdrian prespec brief` refuses any brief whose grid readiness is below 0.6 (`engine/prespec/brief.go:110`), so the new skill cannot use it; it saves the brief with `mem_save` itself. Assumption (S7): the topic `project/{project}/clarify/{YYYYMMDD-HHMM}` replaces `project/{project}/prespec/{ULID}`; nothing reads either.
