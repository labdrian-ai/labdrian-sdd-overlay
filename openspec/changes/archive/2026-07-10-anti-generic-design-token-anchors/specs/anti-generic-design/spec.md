# Delta for anti-generic-design

## MODIFIED Requirements

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

## ADDED Requirements

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

## Out of Scope (this delta)

- Vendoring `awesome-design-md` or any third-party DESIGN.md corpus.
- Expanding into general design-system / token-library tooling (elevation systems, breakpoints, full type hierarchies).
- Automating the anti-mimicry checklist — stays v1 manual, per the skill's existing deferral to a future v2.
