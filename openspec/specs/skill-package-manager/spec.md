# Skill Package Manager Specification

## Purpose

Define the registry schema, parser, serializer, and validate/sync behavior for
skill package entries (`skills.registry.yaml` <-> `overlay.manifest`),
including provenance metadata for vendored external skills. External skills
are always locally vendored and human-reviewed before registration; this
capability records provenance only and never fetches, clones, or executes
remote resources.

## Requirements

### Requirement: External Source Type Accepted

ID: R-112

When `source.type` is `external`, the parser MUST accept it as a valid source
type alongside `core` and `custom`. The `validSourceTypes` set MUST include
`"external"`.

#### Scenario: Parser accepts external source type

- GIVEN a registry YAML entry with `source.type: external`
- WHEN `ParseRegistry` is called
- THEN the entry is accepted with no "unknown source type" error

### Requirement: External Provenance Fields — repo and ref

ID: R-113, R-116

When a skill entry sets `source.type: external`, the parser MUST parse
optional sub-fields `repo` (string) and `ref` (string) directly under the
`source` mapping. Unknown keys under `source` MUST still be rejected with a
line-numbered error. When `ref` is absent, parsing MUST still succeed and
`Source.Ref` MUST be the empty string.

#### Scenario: Parse external entry with repo and ref (SC-55)

- GIVEN a registry YAML with one entry: `source: {type: external, repo: https://github.com/example/skills, ref: a1b2c3d}`
- WHEN `ParseRegistry` is called
- THEN `entry.Source.Type == "external"`
- AND `entry.Source.Repo == "https://github.com/example/skills"`
- AND `entry.Source.Ref == "a1b2c3d"`
- AND error is nil

#### Scenario: Parse external entry with repo only, ref absent (SC-56)

- GIVEN a registry YAML with `source.type: external` and `repo` present, no `ref` field
- WHEN `ParseRegistry` is called
- THEN `entry.Source.Repo` is non-empty
- AND `entry.Source.Ref == ""`
- AND error is nil

### Requirement: External Entry Requires repo

ID: R-114

When `source.type` is `external` and no `repo` field is present (absent or
empty string after unquoting), the parser MUST return a line-numbered error
that names the entry id and states that `repo` is required for `external`
entries.

#### Scenario: Parse rejects external entry missing repo (SC-57)

- GIVEN a registry YAML with `source.type: external` and no `repo` field
- WHEN `ParseRegistry` is called
- THEN error is non-nil
- AND the error message contains the entry id
- AND the error message contains "repo"
- AND the error message contains a line number

### Requirement: repo/ref Forbidden on core and custom

ID: R-115

When `source.type` is `core` or `custom` and a `repo` or `ref` field appears
under `source`, the parser MUST return a line-numbered error that names the
entry id, the offending key, and the actual source type. The error MUST be
emitted before any partial state is returned (fail-loud, mirrors the
`allowedProjects`-on-global guard).

#### Scenario: Parse rejects repo on core entry (SC-58)

- GIVEN a registry YAML with `source.type: core` and a `repo` field under `source`
- WHEN `ParseRegistry` is called
- THEN error is non-nil
- AND the error message contains the entry id, "repo", the actual source type ("core"), and a line number

#### Scenario: Parse rejects ref on custom entry (SC-59)

- GIVEN a registry YAML with `source.type: custom` and a `ref` field under `source`
- WHEN `ParseRegistry` is called
- THEN error is non-nil
- AND the error message contains the entry id, "ref", the actual source type ("custom"), and a line number

### Requirement: core upstream Requirement Unchanged

ID: R-117

When `source.type` is `core`, the existing requirement that `upstream.owner`
be non-empty (enforced in `validateEntry`) MUST continue to hold unchanged.

#### Scenario: core entry without upstream.owner still fails

- GIVEN a registry YAML with `source.type: core` and an empty or absent `upstream.owner`
- WHEN `ParseRegistry` is called
- THEN error is non-nil naming the missing `upstream.owner`

### Requirement: Serializer Emits repo/ref for External Entries

ID: R-118, R-119

When `Serialize` processes an entry with `source.type == "external"`, it MUST
emit a `repo` line under the `source` block when `Source.Repo` is non-empty,
and a `ref` line when `Source.Ref` is non-empty. Both lines MUST appear at
indent 6 (inside `source:` at indent 4). Emitted `repo`/`ref` values MUST use
the same `scalar()`/`needsQuote()` quoting rules as all other string scalars.

#### Scenario: Serialize emits repo and ref lines

- GIVEN a `Registry` with an external entry where `Source.Repo` and `Source.Ref` are both non-empty
- WHEN `Serialize(reg)` is called
- THEN the output contains a `repo:` line and a `ref:` line under that entry's `source:` block, correctly indented and quoted

### Requirement: repo/ref Representable Character Validation

ID: R-120

When `checkRepresentable` validates an entry, it MUST check `source.repo` and
`source.ref` values using the existing `representable()` function. A repo or
ref value containing forbidden characters MUST cause `Serialize` to return a
non-nil error naming the entry id and the offending field.

#### Scenario: Serialize rejects repo value with forbidden chars (SC-61)

- GIVEN a `Registry` with an external entry where `Source.Repo` contains `"{"`
- WHEN `Serialize(reg)` is called
- THEN error is non-nil
- AND the error message names the entry id and "source.repo"
- AND no bytes are returned

### Requirement: Round-Trip Identity for External Entries

ID: R-121

`parse(serialize(r)) == r` (round-trip identity) MUST hold for any `Registry`
that contains external entries with `Repo` and/or `Ref` values, where equality
is evaluated by `reflect.DeepEqual`.

#### Scenario: Round-trip external entry with repo and ref (SC-60)

- GIVEN a `Registry` value with one external entry where `Source.Repo` and `Source.Ref` are both non-empty
- WHEN `Serialize(reg)` is called, then `ParseRegistry` is called on the result
- THEN `reflect.DeepEqual(reg, parsed) == true`
- AND the serialized bytes contain `repo:` and `ref:` under the source block

### Requirement: external Maps to Manifest Tag custom

ID: R-122, R-123

When `tagMatchesSourceType` evaluates a manifest tag against a registry source
type, `source.type == "external"` MUST match the manifest tag `"custom"`.
When `registryTag` derives the manifest tag for an entry, `source.type ==
"external"` MUST produce the tag `"custom"`. The existing mappings
(`"managed"` <-> `"core"`, `"custom"` <-> `"custom"`) MUST be preserved.

#### Scenario: tagMatchesSourceType — external matches custom tag (SC-62)

- GIVEN `tag == "custom"`, `sourceType == "external"`
- WHEN `tagMatchesSourceType(tag, sourceType)` is called
- THEN result is `true`
- GIVEN `tag == "managed"`, `sourceType == "external"`
- WHEN `tagMatchesSourceType(tag, sourceType)` is called
- THEN result is `false`

### Requirement: Validate and Sync Recognize External Entries

ID: R-124, R-125

When `Validate` is called with a registry that contains an external entry and
a manifest that tags that entry's path as `custom`, it MUST report zero
divergences (exit 0). When `SyncManifest` processes a registry containing an
external entry, it MUST emit `<path>/SKILL.md custom` in the output manifest
row for that entry.

#### Scenario: Validate — external entry aligned against custom manifest tag (SC-63)

- GIVEN a `Registry` with one external entry at path "my-ext-skill" and a manifest with row "my-ext-skill/SKILL.md custom"
- WHEN `Diff(reg, mv)` is called
- THEN the result is an empty divergence list

#### Scenario: Sync — external entry emits custom tag row (SC-64)

- GIVEN a `Registry` with one external entry (path "my-ext-skill") and an empty manifest
- WHEN `SyncManifest(reg, manifest)` is called
- THEN the output contains "my-ext-skill/SKILL.md custom"
- AND the output does NOT contain "my-ext-skill/SKILL.md managed"
- AND `ChangeReport.Added` contains "my-ext-skill"

### Requirement: Zero-Fetch Invariant — Forbidden Imports

ID: R-131

At all times, the `engine/skills` package MUST NOT import `net/http`, `net`,
`os/exec`, or any package whose import path contains `git`. This invariant
MUST be verifiable by scanning the output of `go list -f '{{join .Imports
"\n"}}' ./engine/skills/...` and asserting none of the forbidden paths appear.

#### Scenario: Zero-fetch import scan is clean (SC-69)

- GIVEN the `engine/skills` package
- WHEN `go list -f '{{join .Imports "\n"}}' ./engine/skills/...` is executed (or an equivalent `go/packages`-based test)
- THEN the output does not contain "net/http", "net", "os/exec", or any path containing "git"

### Requirement: No New Non-Stdlib Imports

ID: R-132

The implementation of the external-provenance feature MUST NOT add any new
non-stdlib import to `engine/skills`. The only allowed imports are those
already present in the package before this change (stdlib only).

#### Scenario: Import allowlist stays stdlib-only

- GIVEN the diff that introduces external-provenance support
- WHEN the import list of `engine/skills` is compared before and after
- THEN no new non-stdlib import appears

### Requirement: Exit-Code Consumer Enumeration Precedes The Behavior Change

ID: R-001

WHERE a change adds a divergence class to `skills validate`'s exit contract,
the change MUST persist a named enumeration of every consumer of that exit
code, together with the assessed impact on each consumer, before any code
path emits the new class.

#### Scenario: Enumeration exists before the new class ships

- GIVEN a change that adds `UNREGISTERED_ON_DISK` and `MISSING_ON_DISK` to the exit contract
- WHEN the implementing code path is merged
- THEN a persisted consumer enumeration (path/line and per-consumer disposition) already exists in the change history

### Requirement: Existing Registry/Manifest Divergence Classes Remain Unchanged

ID: R-008

WHILE the on-disk cross-check is active, `skills validate` MUST keep the
`MISSING_IN_MANIFEST`, `MISSING_IN_REGISTRY`, `TAG_MISMATCH`, and `MIXED_TAG`
outcomes, their diagnostic format, and their exit codes exactly as they were
before this change.

#### Scenario: Existing behavior and green paths are unaffected

- GIVEN the current `skills validate` test suite for the four registry/manifest classes
- WHEN the on-disk cross-check is added
- THEN every existing test passes with no assertion edits
- AND a registry/manifest pair with no divergence of any kind still exits 0

### Requirement: On-Disk Diagnostics Name A Working Remediation

ID: R-009

WHEN `skills validate` exits non-zero because of an on-disk divergence, the
diagnostic MUST name the manifest-row edit that resolves it and MUST NOT cite
a command that cannot resolve it (`sync-manifest` cannot clear
`UNREGISTERED_ON_DISK` for reference files or `_shared/*` files, because
`isSkillRow` only regenerates `*/SKILL.md` rows).

#### Scenario: Diagnostic names the manifest-row edit, not sync-manifest

- GIVEN an `UNREGISTERED_ON_DISK` or `MISSING_ON_DISK` diagnostic
- WHEN its text is inspected
- THEN it names editing the specific `overlay.manifest` row/path as the remediation
- AND it does not claim `sync-manifest` resolves the divergence

### Requirement: Documented Exit Contract Covers The New Classes

ID: R-010

WHERE `skills validate` can exit non-zero because of an on-disk divergence,
the overlay's documentation (CLI help text, README, and the command's doc
comment) MUST state that the command also cross-checks the skills directory
against `overlay.manifest`.

#### Scenario: Documentation matches the real contract

- GIVEN the documentation sites for `skills validate` (help text, README, `engine/cmd/main.go` doc comment)
- WHEN their content is compared against the actual exit contract
- THEN each site states that an on-disk divergence also causes a non-zero exit
