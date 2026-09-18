# Delta for Skill Lifecycle

## ADDED Requirements

### Requirement: AddCore Enforces a LintSkill Hard Gate

`AddCore` MUST run `LintSkill` (from the `skill-lint` capability) against the
candidate skill's frontmatter and body before performing any manifest or
registry write, in addition to its existing SKILL.md-existence precondition.
When `LintSkill` returns any hard error, `AddCore` MUST refuse the add: exit
non-zero, leave the registry and manifest byte-for-byte unchanged, and
report the hard lint error(s) to the caller. When `LintSkill` returns zero
hard errors, `AddCore` MUST proceed exactly as before this change, regardless
of any warnings `LintSkill` reports.

#### Scenario: AddCore refuses a hard-lint-failing SKILL.md before any write

- GIVEN a `t.TempDir()` with a minimal registry, manifest, and
  `skills/foo/SKILL.md` whose frontmatter is missing `metadata.version`
- WHEN `AddCore(["foo"], ...)` is called
- THEN exit is non-zero
- AND the hard lint error naming `metadata.version` is reported
- AND registry bytes are unchanged
- AND manifest bytes are unchanged

#### Scenario: AddCore proceeds when LintSkill reports only warnings

- GIVEN a `t.TempDir()` with a minimal registry, manifest, and
  `skills/foo/SKILL.md` that has zero hard lint errors and at least one
  warning
- WHEN `AddCore(["foo"], ...)` is called
- THEN exit is 0
- AND the registry entry for `foo` is written exactly as it would be
  without this change

#### Scenario: AddCore behavior for a lint-clean SKILL.md is unchanged

- GIVEN the existing `AddCore` test suite (SC-65 through SC-68 and prior
  cases) exercised against SKILL.md fixtures with zero hard lint errors
- WHEN this capability's `LintSkill` gate is added
- THEN every existing `AddCore` test still passes unmodified

#### Scenario: The lint gate runs before any manifest or registry write

- GIVEN a SKILL.md with a hard lint error and a registry/manifest pair that
  would otherwise validate
- WHEN `AddCore` processes the add
- THEN the lint check happens before the manifest is appended and before
  the registry is serialized
- AND no partial write (manifest-only or registry-only) occurs as a result
  of the refusal
