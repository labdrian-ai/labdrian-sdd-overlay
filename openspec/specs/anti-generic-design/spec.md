# Spec: anti-generic-ai-design-skill

## ADDED Requirements

### Requirement: Forbidden-pattern list beyond frontend-design
`skills/anti-generic-design/SKILL.md` MUST list, as explicitly forbidden, the "Claude/SaaS look" patterns that `anthropics/skills/frontend-design` does not cover:
- Inter or equivalent over-used sans-serif fonts
- Violet-to-blue gradient backgrounds/accents
- Generic `box-shadow` card styling
- Flat 3-column grid layouts

The SKILL.md MUST state explicitly that this list is complementary to, not a duplicate or replacement of, frontend-design's own banned-look list (cream+serif+terracotta, near-black+acid-green, broadsheet-hairlines).

#### Scenario: Skill documents the gap
- **GIVEN** a reader opens `skills/anti-generic-design/SKILL.md`
- **WHEN** they look for what this skill forbids
- **THEN** they find all four forbidden patterns (Inter/equivalent, violet-blue gradient, generic shadow card, flat 3-col grid) named explicitly
- **AND** a statement clarifying this skill does not duplicate frontend-design's existing bans

### Requirement: Reference palette/typography grounded in real sites

ID: R-002

WHEN a reader opens `skills/anti-generic-design/references/palette-typography.md`, the system SHALL present a dimension-indexed anchor table — one row per design dimension (canvas polarity, accent scarcity, corner-radius signature, depth mechanism, type philosophy) — with each row citing 2–3 contrasting anchors drawn from a fixed table of 5–6 distinct industry verticals' current market-leader sites.

(Previously: distilled directions from 4 named award-gallery sites, one row per aspect, informed by human recollection with no persisted capture artifact.)

#### Scenario: Reference file presents dimension rows, not per-site aspect rows
- GIVEN a reader opens `palette-typography.md`
- WHEN they inspect the anchor table
- THEN each row corresponds to one design dimension (canvas polarity, accent scarcity, corner-radius signature, depth mechanism, type philosophy)
- AND each row cites 2–3 anchors that visibly disagree with each other

#### Scenario: Old no-capture caveat is retired
- GIVEN the file's attribution note
- WHEN a reader checks it against the new capture
- THEN the "human recollection / no capture artifact" caveat is updated or removed to match the now-real, dated capture

### Requirement: Sector-led capture sourcing method

ID: R-006

The `palette-typography.md` capture table SHALL source each anchor from one of 5–6 fixed, distinct industry verticals' current (not stale or superseded) market-leader site, each entry independently observed by opening the live site and dated with a "last verified: <date>" stamp.

#### Scenario: Vertical table is fixed and distinct
- GIVEN the sector table backing the anchors
- WHEN it is inspected
- THEN it lists 5–6 distinct industry verticals, each represented by its current market leader

#### Scenario: Entry is independently observed and dated
- GIVEN any single anchor entry
- WHEN its provenance is checked
- THEN it carries a "last verified: <date>" stamp and was recorded from direct observation (rendered hex, computed font-family/weight/size, actual border-radius, actual shadow treatment), not recollection

### Requirement: Observational/factual-commentary framing as sole trademark mitigation

ID: R-007

WHILE any anchor cites a named market-leader brand, the system SHALL frame the citation strictly as observational/factual CSS commentary ("this site's rendered CSS uses X"), never as a redistributed brand design system.

#### Scenario: Citation reads as observation, not brand kit
- GIVEN a citation naming a real brand's site
- WHEN its wording is reviewed
- THEN it states what was observed on the live site, with no implication of a redistributed or licensed brand design system

### Requirement: Anti-mimicry checklist line

ID: R-008

The `critique-template.md` checklist and the `SKILL.md` inline checklist SHALL each include the line `[ ] token set is NOT >80% traceable to a single cited anchor → PASS`.

#### Scenario: Line present in both checklists
- GIVEN `critique-template.md` and `SKILL.md`
- WHEN their checklists are compared
- THEN both contain the identical anti-mimicry line, consistent in wording and format with the existing checklist lines

### Requirement: Anti-mimicry gate validated before close

ID: R-009

IF the anti-mimicry checklist line has not been run against at least 2–3 real generated design outputs with the outcome recorded, THEN the change SHALL NOT be considered complete.

#### Scenario: Gate demonstrably flags single-anchor mimicry
- GIVEN 2–3 real generated outputs (the existing before/after case plus 1–2 fresh generation attempts, at least one deliberately mimicking a single anchor)
- WHEN the anti-mimicry line is run against each
- THEN at least one output is correctly flagged as failing, and the outcome is recorded

#### Scenario: Missing validation blocks completion
- GIVEN the checklist line has been added but never run against real outputs
- WHEN completion is assessed
- THEN the change is reported incomplete until the validation run and its recorded outcome exist

### Requirement: v1 validation heuristic (manual, rule-based)
The skill MUST define a self-critique heuristic expressed as a simple, human-runnable checklist of rules/keywords (e.g., flagging "Inter", "gradient", "shadow", "3-column grid" in generated output or design descriptions). This heuristic MUST NOT require any automated script, linter, or tooling to execute — v1 is manual-only. Automated tooling is explicitly deferred to a future v2 and is out of scope for this change.

#### Scenario: Heuristic is runnable by hand
- **GIVEN** a design output to review against this skill
- **WHEN** a person follows the SKILL.md self-critique checklist
- **THEN** they can complete the check using only the checklist text, with no script or tool invocation required

### Requirement: Skill directory structure matches verifiable-skill precedent
The skill MUST be structured as `skills/anti-generic-design/SKILL.md` plus a `references/` subdirectory, modeled on the `anthropics/knowledge-work-plugins` `data/skills/data-context-extractor` precedent (SKILL.md + references, no code).

#### Scenario: Structure check
- **GIVEN** the `skills/anti-generic-design/` directory
- **WHEN** its contents are listed
- **THEN** it contains `SKILL.md` and a `references/` subdirectory
- **AND** it contains no engine/TUI source code

### Requirement: Skill is registered for deployment and indexed for discovery
After `skills/anti-generic-design/SKILL.md` exists, the skill MUST be both **registered for deployment** and **indexed for discovery**. These are two distinct artifacts written by two distinct commands, and satisfying one does not satisfy the other:

| Artifact | Purpose | Sole mechanism |
|---|---|---|
| `skills.registry.yaml` + `overlay.manifest` | Deployment — decides whether `labdrian apply` copies the skill to the runtime targets | `labdrian skills add <id>`, then `labdrian skills sync-manifest` |
| `.atl/skill-registry.md` | Discovery — the delegator index a launching agent reads to select skills | `gentle-ai skill-registry refresh --force`, or the `/skill-registry` skill |

Neither `skills.registry.yaml` nor the `*/SKILL.md` row in `overlay.manifest` may be hand-edited; `labdrian skills add` writes both, and `labdrian skills sync-manifest` places the row in the registry-ordered block. `references/` rows in `overlay.manifest` MUST be added directly, because `sync-manifest` regenerates only `*/SKILL.md` rows by design and never reference files.

> Superseded wording: this requirement previously named `gentle-ai skill-registry refresh --force` as "the sole mechanism for indexing" while also forbidding hand-edits to `skills.registry.yaml`. That route is impossible — `skill-registry refresh` regenerates `.atl/skill-registry.md` and cannot write `skills.registry.yaml` at all — and following it literally left the skill registered nowhere, undeployed to all three runtimes, while `apply`, `sync-check` and `skills validate` all reported green.

#### Scenario: Skill is registered for deployment
- **GIVEN** `skills/anti-generic-design/SKILL.md` exists
- **WHEN** `labdrian skills add anti-generic-design` is run, followed by `labdrian skills sync-manifest`
- **THEN** `skills.registry.yaml` contains an entry for `anti-generic-design` targeting claude, opencode and codex
- **AND** `overlay.manifest` contains an `anti-generic-design/SKILL.md` row inside the registry-ordered skill block
- **AND** every file under `skills/anti-generic-design/`, `references/` included, has a deploying `overlay.manifest` row
- **AND** neither file was hand-edited to produce the `SKILL.md` row

#### Scenario: Skill appears in the regenerated discovery index
- **GIVEN** `skills/anti-generic-design/SKILL.md` exists
- **WHEN** `gentle-ai skill-registry refresh --force` is run
- **THEN** `.atl/skill-registry.md` contains a row for `anti-generic-design` with its trigger/description and path

#### Scenario: An unregistered skill fails the build
- **GIVEN** a skill directory present under `skills/` with no deploying `overlay.manifest` row
- **WHEN** the engine test suite runs
- **THEN** `TestRepositorySkillsAreFullyRegistered` fails and names every unregistered path
- **AND** the failure is reported as `UNREGISTERED_ON_DISK`

## OUT OF SCOPE (unchanged from proposal)

- Automated lint/script validator for the heuristic (deferred to v2).
- Building the `labdrian-brain` design vault.
- Duplicating, merging, or replacing `frontend-design`, `design-critique`, or `design-system`.
