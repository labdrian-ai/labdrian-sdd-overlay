# Proposal: prespec-malandra

## Why

The overlay has a complete pipeline for the case where requirements ALREADY exist as
language: `requirements-from-transcripts` converts meeting transcripts, customer stories,
and stakeholder notes into traceable EARS requirements, which then feed manifest →
architecture → roadmap → SDD. It also has `sdd-explore`, which accepts a topic seed and
investigates an idea against the codebase.

There is a gap NEITHER tool fills: the **zero-input prespec** case. A maintainer arrives
with only a vague itch — "I want to improve onboarding", "something is slow", "we need a
dashboard" — and there is NO transcript to transcribe, NO feature description to explore,
and NO requirement to derive. `requirements-from-transcripts` needs raw conversation as
input; it cannot manufacture intent from nothing. `sdd-explore` needs a topic seed; the
audit that drove the design confirmed it already accepts one, so "we fill explore's
zero-input" was a hollow claim. The REAL unmet need is **manufacturing that seed** —
eliciting WHAT to build when the only starting material is a vague idea.

`spec-kit` (evaluated in `sdd/prespec/spec-kit-evaluation`) scored 3/5 for this gap: its
`/clarify` taxonomy is useful but REQUIRES a `spec.md` to already exist, so it cannot run on
a blank slate. No toolkit performs product discovery / stakeholder elicitation from zero.

This change introduces **prespec-malandra**: a delegate-only skill that runs an adaptive
Socratic interview to turn a vague idea into a structured prespec brief. The brief is a
synthetic transcript persisted under `project/{project}/prespec/{discovery-id}`, which then
feeds `requirements-from-transcripts` exactly like a real transcript would. The novelty is
the ASSEMBLY: cold-start from zero, an adaptive question-sequencing state machine, a
sufficiency/convergence criterion, and an honest refusal floor — not any single borrowed
piece.

### Honest scope note (probabilistic elicitation, hard contract boundary)

The interview quality is instruction-driven and therefore probabilistic — a sub-agent under
load may ask a weaker question than the design intends. What is NOT probabilistic, and what
this proposal encodes as hard constraints, is the CONTRACT BOUNDARY: prespec-malandra never
derives a change-name, never invents a full strawman, never self-grades, never forces a
metric, and never squats the `sdd/` namespace. Those are red lines, not best-effort nudges.

## What Changes

This change delivers ONE new delegate-only skill and its supporting reference material. It
adds NO code to the SDD engine and modifies NO existing skill's behavior. The brief it emits
is consumed manually (copied into `requirements-from-transcripts`) in this slice.

1. **Add** `skills/prespec-malandra/SKILL.md` — the delegate-only Socratic prespec engine.
   The MVP runs in HARDCODED `standard` mode (question budget 5) and implements:
   - **Stage 0 — cold-start seed**: a Mom-Test past-behavior probe ("Tell me about the last
     time X frustrated you") as the primary opener; a 5-archetype pain MCQ as fallback when
     the user cannot answer freely; and an honest **refusal floor** — if no goal can be
     elicited, return `needs-more-input` and NEVER fabricate a goal.
   - **Stage 1 — job statement**: elicit a `[verb] + [object] + [context]` job statement,
     apply an **anti-solution bounce** (redirect premature solution-talk back to the
     problem), and produce a **bounded readback-as-strawman** that reframes the JOB ONLY —
     never an invented data model, scenario, or schema.
   - **Stage 3 — budgeted adaptive loop**: initialize the **10-cell coverage grid** (Stage 2,
     folded into init), rank uncovered cells by **Impact × Uncertainty**, and ask ONE
     non-leading question per iteration (MCQ or ≤5-word answers), in emergent order, until the
     budget (5) or the stop test fires.
   - **No-leading lint**: a rejection checklist applied to every candidate question before it
     is asked — reject questions that smuggle the answer, presuppose a solution, or bundle
     multiple concerns.
   - **Stage 5 — convergence + readiness gate**: a stop test (coverage reached / explicit user
     signal / budget exhausted / diminishing returns) and a **readiness score**; `Partial`
     never counts as `Clear`, and a score `< 0.6` returns `needs-more-input` instead of a
     brief.
   - **Artifact emission**: on success, persist the prespec brief (sections 1–6 + the full
     synthetic transcript) to `project/{project}/prespec/{discovery-id}`, where
     `discovery-id` is a **ULID** (NOT kebab-case) with `capture_prompt: false`.

2. **Add** `skills/prespec-malandra/references/coverage-taxonomy.md` — the 10-cell coverage
   grid definition (cells adapted from spec-kit's `/clarify` taxonomy, re-weighted to
   front-load pre-goal discovery: JTBD job, current-state gap, why-now) plus the
   Impact × Uncertainty ranking rubric and the no-leading lint checklist.

### Capabilities

#### New

- **`prespec-engine`** — a delegate-only Socratic interview state machine that elicits WHAT
  to build from a zero/vague input and emits a structured prespec brief. Owns Stage 0
  (cold-start + refusal floor), Stage 1 (job statement + anti-solution bounce + bounded
  readback), Stage 3 (budgeted adaptive question loop over the coverage grid), Stage 5
  (convergence + readiness gate), and brief emission. Hardcoded `standard` mode (budget 5)
  for this slice.

- **`prespec-coverage-grid`** — the 10-cell coverage model with Impact × Uncertainty cell
  ranking and the no-leading question lint. Drives question selection in the Stage 3 loop and
  the coverage component of the Stage 5 stop test.

- **`prespec-brief-artifact`** — the persisted output contract: a synthetic-transcript brief
  (sections 1–6 + transcript) stored at `project/{project}/prespec/{discovery-id}` with a
  ULID identifier and `capture_prompt: false`. Designed to be consumed by
  `requirements-from-transcripts` as if it were a real transcript.

#### Modified

- None. No existing skill, engine phase, or topic-key owner is modified in this slice. The
  brief is consumed manually (copied into `requirements-from-transcripts`).

### Requirements (EARS)

- **R-001**: WHEN `prespec-malandra` is invoked with a vague or empty idea, the engine SHALL
  run Stage 0 and emit a Mom-Test past-behavior probe before any solution-shaped question.
- **R-002**: IF Stage 0 cannot elicit any goal after the probe and the 5-archetype MCQ
  fallback, THEN the engine SHALL return `needs-more-input` and SHALL NOT fabricate a goal.
- **R-003**: The engine SHALL produce a `[verb] + [object] + [context]` job statement in
  Stage 1 and SHALL redirect any premature solution input back to the problem (anti-solution
  bounce).
- **R-004**: The Stage 1 readback SHALL reframe the job statement ONLY and SHALL NOT invent a
  data model, scenario, schema, or other strawman artifact.
- **R-005**: The engine SHALL initialize the 10-cell coverage grid and SHALL rank uncovered
  cells by Impact × Uncertainty before selecting each Stage 3 question.
- **R-006**: The engine SHALL ask at most ONE question per Stage 3 iteration and SHALL stop
  the loop at the question budget of 5 in `standard` mode.
- **R-007**: Before asking any question, the engine SHALL apply the no-leading lint and SHALL
  reject any question that smuggles the answer, presupposes a solution, or bundles concerns.
- **R-008**: WHERE a coverage cell remains unresolved at the budget, the engine MAY emit a
  bounded informed guess marked `[ASSUMPTION]`, capped at 3 assumptions, each justified
  against the job statement.
- **R-009**: The engine SHALL run a Stage 5 stop test (coverage / user signal / budget /
  diminishing returns) and compute a readiness score; a `Partial` cell SHALL NOT be counted
  as `Clear`.
- **R-010**: IF the readiness score is `< 0.6`, THEN the engine SHALL return
  `needs-more-input` and SHALL NOT emit a brief.
- **R-011**: WHEN the readiness gate passes, the engine SHALL persist the brief (sections 1–6
  + transcript) to `project/{project}/prespec/{discovery-id}` using a ULID identifier and
  `capture_prompt: false`.
- **R-012**: The engine SHALL NOT derive a change-name; change-name derivation remains the
  exclusive responsibility of `requirements-from-transcripts`.
- **R-013**: WHERE a metric or success measure cannot be elicited, the engine SHALL offer a
  `no-metric-yet` escape and SHALL NOT force the user to invent a metric.

## Impact

- **New files**: `skills/prespec-malandra/SKILL.md`,
  `skills/prespec-malandra/references/coverage-taxonomy.md`.
- **New persisted artifact namespace**: `project/{project}/prespec/{discovery-id}` (ULID),
  distinct from and never overlapping the `sdd/` engine namespace.
- **Affected actors**: the maintainer running prespec-malandra (interview UX);
  `requirements-from-transcripts` (gains a new, synthetic-transcript input shape it can
  ingest unchanged). No engine phase, no orchestrator runtime, no existing skill behavior is
  modified.
- **Open-source posture**: the skill is authored to be extractable into a standalone
  `prespec-malandra` repo later; this slice does NOT package or publish it.

## Scope

### In scope (first slice / MVP)

- One delegate-only skill `prespec-malandra` in HARDCODED `standard` mode (budget 5).
- Stage 0 (cold-start seed + refusal floor) + Stage 1 (job statement + anti-solution bounce +
  bounded readback) + Stage 3 (budgeted adaptive loop).
- The 10-cell coverage grid with Impact × Uncertainty ranking.
- The no-leading lint as a question-rejection checklist.
- Stage 4 bounded informed-guess with `[ASSUMPTION]` cap 3.
- Stage 5 stop test + readiness gate at 0.6.
- Brief artifact (sections 1–6 + synthetic transcript) persisted to
  `project/{project}/prespec/{discovery-id}` (ULID, `capture_prompt: false`).
- Standalone manual invocation; brief output copied by hand into
  `requirements-from-transcripts`.

### Out of scope (explicit non-goals)

- NO tier-based mode selection (`quick=3` / `deep=7`) — mode is hardcoded `standard` (5).
- NO security → `High` auto-promotion of coverage cells.
- NO enforced `sdd-propose` crosswalk (the 7-of-10-dimension pre-fill is a v2 goal).
- NO `sdd-explore` auto-read of the brief — consumption is manual in this slice.
- NO public OSS repo packaging or publication.
- NO modification of `requirements-from-transcripts`, `sdd-explore`, or any engine phase.

### Red lines (hard constraints, non-negotiable)

- **Never derive a change-name in prespec.** Change-name authority stays with
  `requirements-from-transcripts` (see `_shared/pre-sdd-contracts.md`). The ULID
  `discovery-id` is chosen specifically so a prespec id can NEVER be mistaken for a
  kebab-case change-name.
- **Never invent a full strawman** (data models, scenarios, schemas). The Stage 1 readback
  reframes the JOB only. (This is the "Lens B trap" the design rejected.)
- **Never self-grade the coverage grid against an invented draft.** Readiness is measured
  against elicited coverage, not against a fabricated spec.
- **Never force a metric.** Offer a `no-metric-yet` escape.
- **Never squat the `sdd/` namespace.** The brief lives under
  `project/{project}/prespec/`, never under `sdd/`.

## Acceptance Criteria

- **AC-1**: Invoking prespec-malandra with a vague idea produces EITHER a brief at
  `project/{project}/prespec/{ULID}` OR a `needs-more-input` outcome — never a fabricated
  goal and never a change-name.
- **AC-2**: The emitted brief contains sections 1–6 plus a full synthetic transcript and is
  ingestible by `requirements-from-transcripts` without manual reshaping of its structure.
- **AC-3**: The Stage 3 loop asks ≤ 5 questions in `standard` mode, one per iteration, each
  passing the no-leading lint.
- **AC-4**: A readiness score `< 0.6` yields `needs-more-input` with NO brief written.
- **AC-5**: At most 3 `[ASSUMPTION]`-marked guesses appear, each tied to the job statement.
- **AC-6**: The persisted `discovery-id` is a ULID; no `sdd/`-namespaced or kebab-case
  identifier is created by this skill.

## Risks and Open Questions

- **Probabilistic interview quality**: question selection and the no-leading lint are
  instruction-driven; a weaker question may slip through under load. Mitigation: the lint is a
  rejection checklist applied per question, and the readiness gate refuses low-coverage briefs
  rather than emitting weak output.
- **Hardcoded budget (5) may under- or over-ask**: a single fixed budget will not fit every
  idea. Accepted for the MVP; tier-based mode selection is the v2 remedy.
- **Manual hand-off seam**: copying the brief into `requirements-from-transcripts` by hand is
  error-prone and unenforced. Accepted for this slice; the enforced crosswalk / auto-read is
  deferred to v2.
- **Readiness threshold (0.6) is unvalidated**: the cutoff is a design assumption with no
  empirical calibration yet. Open question: revisit after the first handful of real runs.
- **Synthetic-transcript ingestion**: `requirements-from-transcripts` was built for real
  conversation; the brief must read like one. Open question: does its contradiction/ambiguity
  pre-analysis behave well on a synthetic transcript, or does it need a brief-aware hint? To
  validate during spec/design.

## Assumptions

- **A-1**: A ULID generator is available to the delegate at runtime; if not, the spec/design
  phase must choose an equivalent non-kebab, collision-resistant id scheme that still makes
  change-name confusion impossible.
- **A-2**: `requirements-from-transcripts` can ingest the brief's synthetic transcript as-is;
  validated by AC-2, flagged as a risk above if it needs a brief-aware adapter.
- **A-3**: `standard` mode (budget 5) is the right default for the MVP, consistent with the
  spec-kit `/clarify` budget noted in the design.
- **A-4**: Brief sections 1–6 are the structure carried from the design's brief template; the
  spec phase will pin the exact section list. This proposal treats "sections 1–6 + transcript"
  as the contract, deferring exact field names to the spec.
- **A-5**: The skill is `strict_tdd: false`-appropriate (Markdown skill + reference doc, no
  testable production code), consistent with how skill-only changes have been treated in this
  repo; the design phase confirms the testing posture.
