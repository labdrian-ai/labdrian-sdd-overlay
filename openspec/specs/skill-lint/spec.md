# Skill Lint Specification

## Purpose

Provide one authoritative, pure, stdlib-only rule set that decides whether a
SKILL.md is well-formed enough to be registered or promoted. `LintSkill` is
the single source of truth consumed by project-tier registration, global
promotion (`AddCore`), and the `engine skills lint` CLI. It never touches the
filesystem or the network itself; only its CLI wrapper reads a file from
disk.

## Requirements

### Requirement: LintSkill Is a Pure, Stdlib-Only Function

The system MUST expose `LintSkill(frontmatter, body) (hard []error, warnings
[]Warning)` in `engine/skills` as a pure function: given the same
frontmatter and body input, it MUST return the same hard errors and warnings
every time, MUST perform no filesystem I/O, no network I/O, and no
`os/exec` call, and MUST import stdlib packages only (ADR-15,
`TestZeroFetchImportAllowlist`).

#### Scenario: LintSkill performs no I/O

- GIVEN a call to `LintSkill` with arbitrary frontmatter and body strings
- WHEN the call executes
- THEN no file is opened, no network socket is used, and no subprocess is
  started
- AND the only imports reachable from the function are stdlib packages

#### Scenario: LintSkill is deterministic

- GIVEN the same frontmatter and body strings passed to `LintSkill` twice
- WHEN both calls execute
- THEN both calls return byte-identical hard-error and warning slices

### Requirement: Hard Rules Block on Structural and Required-Field Defects

`LintSkill` operates on already-split frontmatter and body text: the
frontmatter fence itself is handled earlier, by `SplitSkillFile` inside
`LintSkillFile` (see the `LintSkillFile Composes SplitSkillFile and
LintSkill` requirement below), and `LintSkill` never sees fence markers or
evaluates fence well-formedness. Given that already-split input, `LintSkill`
MUST return at least one hard error, and MUST NOT return zero hard errors,
when any of the following holds: any of `name`, `description`, `license`,
`metadata.author`, or `metadata.version` is absent or empty; `description`
spans more than one line; `description` exceeds the documented hard length
bound; or the body exceeds the documented hard body-size budget (a
documented deterministic proxy, since `engine/` carries no tokenizer).

#### Scenario: Missing required field is a hard error

- GIVEN frontmatter that omits `metadata.author`
- WHEN `LintSkill` evaluates it
- THEN `hard` contains at least one error naming `metadata.author`

#### Scenario: Multi-line description is a hard error

- GIVEN a `description` value containing an embedded newline
- WHEN `LintSkill` evaluates it
- THEN `hard` contains at least one error about the description spanning
  multiple lines

#### Scenario: Description over the hard bound is a hard error

- GIVEN a `description` value one character longer than the documented hard
  length bound
- WHEN `LintSkill` evaluates it
- THEN `hard` contains at least one error about the description length
- AND a `description` value exactly at the hard bound produces no such error

#### Scenario: Body over the hard budget is a hard error

- GIVEN a body whose size under the documented deterministic proxy exceeds
  the hard body budget by one unit
- WHEN `LintSkill` evaluates it
- THEN `hard` contains at least one error about the body budget
- AND a body exactly at the budget boundary produces no such error

#### Scenario: A well-formed SKILL.md produces zero hard errors

- GIVEN frontmatter with all required fields present, a single-line
  description within the hard bound, and a body within the hard budget
- WHEN `LintSkill` evaluates it
- THEN `hard` is empty

### Requirement: LintSkillFile Composes SplitSkillFile and LintSkill

`LintSkillFile(data []byte)` MUST first call `SplitSkillFile`, which is
where the frontmatter-fence check is evaluated, and MUST only call
`LintSkill(frontmatter, body)` on the successfully split result. When the
frontmatter fence is missing or malformed, `SplitSkillFile` MUST fail before
`LintSkill` ever runs, and `LintSkillFile` MUST surface that failure as a
hard error in its own returned result.

#### Scenario: Missing frontmatter fence is a hard error from LintSkillFile

- GIVEN a SKILL.md file with no `---` frontmatter fence
- WHEN `LintSkillFile` evaluates it
- THEN `SplitSkillFile` fails before `LintSkill` is called
- AND `LintSkillFile`'s returned `hard` contains at least one error naming
  the missing fence

### Requirement: Advisory Warnings Never Block

`LintSkill` MUST report the following as warnings, never as hard errors:
`description` exceeding the SHOULD bound (but not the hard bound); section
order deviating from the documented convention; an incident-log shape (a
`Summary` that reads as a dated event log rather than a lesson); a banned
shell utility named inside a backtick-fenced command; and an absolute
home-path leak (for example a literal `/home/<user>` or `/Users/<user>`
path). Warnings MUST NOT cause `LintSkill` to add to `hard`, and a caller
MUST be able to register or promote a skill that has warnings but no hard
errors.

#### Scenario: Description over SHOULD bound but under hard bound is a warning

- GIVEN a `description` longer than the SHOULD bound but within the hard
  bound
- WHEN `LintSkill` evaluates it
- THEN `warnings` contains an entry about the description length
- AND `hard` is unaffected by that condition

#### Scenario: Banned shell utility in a backtick command is a warning

- GIVEN a body containing a backtick-fenced command invoking a banned shell
  utility (for example `cat` or `grep` where `bat`/`rg` are required)
- WHEN `LintSkill` evaluates it
- THEN `warnings` contains an entry naming the banned utility
- AND `hard` is unaffected by that condition

#### Scenario: Absolute home-path leak is a warning

- GIVEN a body containing a literal absolute home-directory path
- WHEN `LintSkill` evaluates it
- THEN `warnings` contains an entry about the leaked path
- AND `hard` is unaffected by that condition

### Requirement: One Source of Truth for Lint Rules

The system MUST maintain the `LintSkill` rule table in
`engine/skills` as the single authoritative source for every hard rule and
advisory warning it enforces. The machine-checkable section of
`skills/skill-creator/references/skill-style-guide.md` MUST be generated
from that rule table, and a test MUST fail if the generated section drifts
from the current rule table. Human-judgment guidance that is not
machine-checkable MAY remain hand-written prose in the same file. No other
file MUST be treated as a second normative source for machine-checkable
rules (including the currently dangling `docs/skill-style-guide.md`
reference, which this change removes).

#### Scenario: Generated style-guide section matches the rule table

- GIVEN the current `LintSkill` rule table
- WHEN the style-guide generator runs
- THEN the machine-checkable section of
  `skills/skill-creator/references/skill-style-guide.md` matches the
  generator's output exactly

#### Scenario: Drift between rule table and generated doc fails a test

- GIVEN the machine-checkable section of the style guide is hand-edited to
  diverge from the current rule table
- WHEN the drift-detection test runs
- THEN the test fails

#### Scenario: LintSkill table tests cover every rule at its boundary

- GIVEN the complete `LintSkill` rule table (every hard rule and every
  warning)
- WHEN the table-driven test suite runs
- THEN every hard rule has at least one test case at its boundary (just
  inside and just outside the limit, where the rule has a numeric bound)
- AND every warning has at least one test case exercising it

### Requirement: engine skills lint CLI Exit Contract

The system MUST provide `engine skills lint <path>`, a thin CLI wrapper that
reads the SKILL.md at `<path>`, calls `LintSkill`, and prints hard errors and
warnings to a stream a human or agent can read. The CLI MUST exit non-zero
when `LintSkill` returns any hard error, and MUST exit zero when `LintSkill`
returns zero hard errors, regardless of warning count.

#### Scenario: CLI exits non-zero on a hard-error SKILL.md

- GIVEN a SKILL.md file missing a required frontmatter field
- WHEN `engine skills lint <path>` runs against it
- THEN the process exits non-zero
- AND the missing field is named in the printed output

#### Scenario: CLI exits zero with only warnings

- GIVEN a SKILL.md file with no hard errors but at least one warning
  condition
- WHEN `engine skills lint <path>` runs against it
- THEN the process exits zero
- AND the warning is printed
