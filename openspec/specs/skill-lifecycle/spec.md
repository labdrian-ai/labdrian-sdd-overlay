# Skill Lifecycle Specification

## Purpose

Define the behavior of the skill lifecycle CLI surface (`skills add` /
`AddCore`), including optional provenance flags that let an operator register
an already-vendored external skill directory without ever triggering a
fetch, clone, or other network/subprocess operation.

## Requirements

### Requirement: parseFlags Accepts --repo and --ref

ID: R-126

When `parseFlags` processes `--repo <url>` in the argument list, it MUST
return the url in a new `repo` output parameter. When `--ref <sha>` is
present, it MUST return the sha in a new `ref` output parameter. Both flags
are optional and position-independent with respect to existing flags and the
id argument.

#### Scenario: --repo and --ref are parsed independently of position

- GIVEN a command-line argument list containing `--repo <url>` and/or `--ref <sha>` in any position relative to other flags and the id argument
- WHEN `parseFlags` processes the arguments
- THEN the returned `repo`/`ref` values match the supplied flags

### Requirement: AddEntry Provenance Behavior

ID: R-127, R-128

When `AddEntry` (or its replacement `AddEntryWithProvenance`) is called with a
non-empty `repo` string, the returned entry MUST have `source.type ==
"external"` and `Source.Repo` set to the supplied url; when `ref` is also
supplied (non-empty), `Source.Ref` MUST be set to the supplied sha. When
`AddEntry` is called without a `repo` argument (empty string), the returned
entry MUST have `source.type == "custom"` and `Source.Repo == ""`, preserving
existing behavior with no manifest tag or downstream behavior change.

#### Scenario: skills add --repo creates an external entry (SC-65)

- GIVEN a `t.TempDir()` with a minimal registry, manifest, and `skills/foo/SKILL.md`
- WHEN `AddCore(["foo", "--repo", "https://github.com/example/skills"], ...)` is called
- THEN exit is 0
- AND the registry entry for "foo" has `Source.Type == "external"` and `Source.Repo == "https://github.com/example/skills"`
- AND the manifest has "foo/SKILL.md custom"
- AND `Validate(reg, manifestPath)` returns nil error

#### Scenario: skills add --repo --ref creates an external entry with ref (SC-66)

- GIVEN a `t.TempDir()` with a minimal registry, manifest, and `skills/bar/SKILL.md`
- WHEN `AddCore(["bar", "--repo", "https://example.com/repo", "--ref", "deadbeef"], ...)` is called
- THEN `entry.Source.Ref == "deadbeef"`
- AND round-trip `parse(serialize(newReg)).Skills[n].Source.Ref == "deadbeef"` holds

#### Scenario: skills add without --repo stays custom — non-regression (SC-67)

- GIVEN a `t.TempDir()` with a minimal registry, manifest, and `skills/baz/SKILL.md`
- WHEN `AddCore(["baz"], ...)` is called with no `--repo` flag
- THEN exit is 0
- AND `entry.Source.Type == "custom"` and `entry.Source.Repo == ""`
- AND the manifest has "baz/SKILL.md custom"

### Requirement: --repo Preserves the SKILL.md Precondition and Performs No I/O

ID: R-129

When `AddCore` is invoked with `--repo`, it MUST still enforce the existing
SKILL.md-existence precondition before any write. `--repo` MUST NOT trigger
any network I/O, `git` subprocess, or `os/exec` call.

#### Scenario: skills add --repo still enforces the SKILL.md precondition (SC-68)

- GIVEN a `t.TempDir()` with a minimal registry and manifest, and `skills/missing/` absent
- WHEN `AddCore(["missing", "--repo", "https://example.com/repo"], ...)` is called
- THEN exit is non-zero
- AND stderr mentions "missing"
- AND registry bytes are unchanged
- AND manifest bytes are unchanged
- AND no network I/O was performed

### Requirement: External Entries Map to the custom Manifest Tag

ID: R-130

When `AddCore` appends the manifest line for an external entry, it MUST
append `<id>/SKILL.md custom` (not `managed`), consistent with the
package-manager's external-to-custom tag mapping.

#### Scenario: AddCore appends a custom-tagged manifest row for external entries

- GIVEN `AddCore` is called with `--repo` for a valid, existing skill id
- WHEN the manifest is updated
- THEN the appended row reads `<id>/SKILL.md custom`

### Requirement: CLI Passthrough Is Scoped to the add Path

ID: R-133

The `bin/labdrian-overlay` binary MUST pass through `--repo` and `--ref` flags
to `AddCore` when they are present; all other subcommand paths (apply,
capture, validate, sync-manifest, remove, list, status) MUST remain
byte-for-byte unchanged.

#### Scenario: Only the add path changes

- GIVEN the `bin/labdrian-overlay` script before and after this feature
- WHEN any subcommand other than `add` is invoked
- THEN its behavior and output are byte-for-byte identical to before the change

### Requirement: apply/capture Pipeline Isolation

ID: R-134

`cmd_apply` and the capture pipeline MUST NOT be modified by the skill
lifecycle provenance feature. The overlay deploy behavior MUST be identical
before and after.

#### Scenario: Deploy behavior is unaffected by provenance flags

- GIVEN the overlay apply/capture pipeline before and after this feature
- WHEN `cmd_apply` or capture is exercised
- THEN behavior is identical before and after the change
