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
`skills/anti-generic-design/references/` MUST contain a palette and typography reference file that is distilled (not copied verbatim) from 4 named real sites: depoluxe.xyz, bymonolog.com, collection.industries, useportal.net.

Each palette/typography entry in the reference file MUST cite which of the 4 sites informed it.

#### Scenario: Reference file traceability
- **GIVEN** a reader opens the references file under `skills/anti-generic-design/references/`
- **WHEN** they inspect any palette or typography entry
- **THEN** it is attributed to at least one of the 4 named source sites
- **AND** all 4 sites are cited somewhere in the file

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

### Requirement: Skill is indexed via skill-registry refresh, not hand-edited YAML
After `skills/anti-generic-design/SKILL.md` exists, `anti-generic-design` MUST be indexed by running `gentle-ai skill-registry refresh --force` (or the `/skill-registry` skill), which scans `*/SKILL.md` under the repo's `skills/` directory and regenerates `.atl/skill-registry.md`. `skills.registry.yaml` MUST NOT be hand-edited as part of this change — the registry command is the sole mechanism for indexing.

#### Scenario: Skill appears in the regenerated registry
- **GIVEN** `skills/anti-generic-design/SKILL.md` exists
- **WHEN** `gentle-ai skill-registry refresh --force` is run
- **THEN** `.atl/skill-registry.md` contains a row for `anti-generic-design` with its trigger/description and path
- **AND** no manual edits were made to `skills.registry.yaml` to achieve this

## OUT OF SCOPE (unchanged from proposal)

- Automated lint/script validator for the heuristic (deferred to v2).
- Building the `labdrian-brain` design vault.
- Duplicating, merging, or replacing `frontend-design`, `design-critique`, or `design-system`.
