# GADU Canonical Source Specification

## Purpose

Single source of truth for the GADU operator persona. All delivery-shape artifacts
derive from this source. No persona content is authored in the delivery artifacts directly.

## Requirements

### Requirement: Single Canonical Source

ID: R-001

The overlay SHALL maintain exactly ONE canonical GADU persona source file; persona body
SHALL NOT be authored directly in `agents/GADU.md` or `skills/gadu-operator/SKILL.md`.

#### Scenario: Divergent edit to generated artifact is caught

- GIVEN `agents/GADU.md` was edited directly instead of the canonical source
- WHEN the generator test runs
- THEN the test fails, reporting a divergence between the generated artifact and the
  canonical source

#### Scenario: Canonical source is the only authoring point

- GIVEN the canonical source is updated with a new persona trait
- WHEN the generator runs
- THEN both `agents/GADU.md` and `skills/gadu-operator/SKILL.md` reflect the update
  without any manual edit to either file

### Requirement: Valid Claude Code Agent Frontmatter

ID: R-004

The emitted `agents/GADU.md` SHALL be a valid Claude Code agent file with `name`,
`description`, `model: opus`, and `tools` frontmatter and SHALL be invocable as `@GADU`.

#### Scenario: Required frontmatter fields are present

- GIVEN the generator has run successfully
- WHEN `agents/GADU.md` is inspected
- THEN the YAML frontmatter block contains `name: GADU`, a non-empty `description`,
  `model: opus`, and a `tools` field

#### Scenario: Agent frontmatter passes structural parse

- GIVEN `agents/GADU.md` is installed at `~/.claude/agents/GADU.md`
- WHEN the YAML frontmatter of the file is parsed programmatically
- THEN `name` equals `"GADU"` AND `model` equals `"opus"`

<!-- MANUAL acceptance criterion (not automatable in CI): @GADU appears as an
     addressable agent in Claude Code when the file is present in ~/.claude/agents. -->

### Requirement: Generated Files Carry Do-Not-Edit Header

ID: R-012

The generator SHALL prepend each generated file (`agents/GADU.md` and
`skills/gadu-operator/SKILL.md`) with a do-not-edit notice stating that the file is
generated from the canonical source and SHALL NOT be edited directly.

#### Scenario: Generated agent file carries do-not-edit notice

- GIVEN the generator has run
- WHEN `agents/GADU.md` is inspected
- THEN the file contains a comment or frontmatter notice stating it is
  generated from the canonical source and must not be edited directly

#### Scenario: Generated skill file carries do-not-edit notice

- GIVEN the generator has run
- WHEN `skills/gadu-operator/SKILL.md` is inspected
- THEN the file contains a comment or frontmatter notice stating it is
  generated from the canonical source and must not be edited directly

### Requirement: Valid Persona Skill

ID: R-005

The emitted `skills/gadu-operator/SKILL.md` SHALL be a valid overlay persona skill
loadable into a native subagent on demand in opencode and Codex.

#### Scenario: Skill conforms to overlay skill format

- GIVEN the generator has run
- WHEN `skills/gadu-operator/SKILL.md` is inspected
- THEN it contains the GADU persona body and conforms to the overlay skill file format
  used by existing skills in the `skills/` directory

#### Scenario: Skill is loadable in opencode

- GIVEN `skills/gadu-operator/SKILL.md` is installed in the opencode skills directory
- WHEN an opencode native subagent loads the skill
- THEN the GADU persona body is available in the subagent's context

#### Scenario: Skill is loadable in Codex

- GIVEN `skills/gadu-operator/SKILL.md` is installed in the Codex skills directory (`~/.codex/skills`)
- WHEN a Codex native subagent loads the skill
- THEN the GADU persona body is available in the subagent's context

<!-- MANUAL acceptance criteria (not automatable in CI): the "Skill is loadable in
     opencode" and "Skill is loadable in Codex" scenarios above each require a live
     third-party runtime to load the skill file and confirm the persona body is
     available in its context. -->
