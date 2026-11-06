# Skill Package Manager Specification

## Purpose

Define the registry schema, parser, serializer, and validate/sync behavior for
skill package entries (`skills.registry.yaml` <-> `overlay.manifest`),
including provenance metadata for vendored external skills. External skills
are always locally vendored and human-reviewed before registration; this
capability records provenance only and never fetches, clones, or executes
remote resources.

> Since Phase 9 unit H15 the registry is read through the domain's port: `skills.ReadRegistry`
> (the name `ReadRegistry` stands for what this spec calls the parser: the YAML adapter
> `engine/skills/registryyaml` decodes the file and the domain's `Registry.Validate` judges it,
> with the words of every refusal unchanged) and written by `registryyaml.Encode`. The scenarios
> keep their names; the behaviors are those the program has always had, except the reader policy
> of Phase 9 unit H16 (decision Q5): the file's `version` is found first and selects the decoder;
> a key the reader does not know is left out and reported (`Registry.Unread`) instead of refusing
> the file, and a registry that has one is not rewritten by `add` or `remove`; and a field that
> install, approval or projection depend on is refused, naming the field and the line, when it is
> in a shape the reader does not read. The set and the rule are at the top of
> `engine/skills/registryyaml/doc.go`.

## Requirements

### Requirement: External Source Type Accepted

ID: R-112

When `source.type` is `external`, the parser MUST accept it as a valid source
type alongside `core` and `custom`. The `validSourceTypes` set MUST include
`"external"`.

#### Scenario: Parser accepts external source type

- GIVEN a registry YAML entry with `source.type: external`
- WHEN `ReadRegistry` is called
- THEN the entry is accepted with no "unknown source type" error

### Requirement: External Provenance Fields — repo and ref

ID: R-113, R-116

When a skill entry sets `source.type: external`, the parser MUST parse
optional sub-fields `repo` (string) and `ref` (string) directly under the
`source` mapping. A key under `source` that the reader does not know MUST NOT
refuse the file (since H16, decision Q5): it is left out and reported with its
line. When `ref` is absent, parsing MUST still succeed and
`Source.Ref` MUST be the empty string.

#### Scenario: Parse external entry with repo and ref (SC-55)

- GIVEN a registry YAML with one entry: `source: {type: external, repo: https://github.com/example/skills, ref: a1b2c3d}`
- WHEN `ReadRegistry` is called
- THEN `entry.Source.Type == "external"`
- AND `entry.Source.Repo == "https://github.com/example/skills"`
- AND `entry.Source.Ref == "a1b2c3d"`
- AND error is nil

#### Scenario: Parse external entry with repo only, ref absent (SC-56)

- GIVEN a registry YAML with `source.type: external` and `repo` present, no `ref` field
- WHEN `ReadRegistry` is called
- THEN `entry.Source.Repo` is non-empty
- AND `entry.Source.Ref == ""`
- AND error is nil

#### Scenario: An unknown key under source is tolerated and reported (SC-70)

- GIVEN a registry YAML with `source.type: external`, `repo`, `ref` and, between them, a key under `source` that the reader does not know (`mirror: elsewhere`)
- WHEN `ReadRegistry` is called
- THEN error is nil and the entry is read with the `repo` and the `ref` it says
- AND `Registry.Unread` holds exactly one note, `line N: unknown key "mirror" in source`, where N is the line of the key
- (proved by `TestAnUnknownKeyUnderSourceIsToleratedAndReportedWithItsLine` in `engine/skills/registryyaml/policy_test.go`)

### Requirement: External Entry Requires repo

ID: R-114

When `source.type` is `external` and no `repo` field is present (absent or
empty string after unquoting), the parser MUST return a line-numbered error
that names the entry id and states that `repo` is required for `external`
entries.

#### Scenario: Parse rejects external entry missing repo (SC-57)

- GIVEN a registry YAML with `source.type: external` and no `repo` field
- WHEN `ReadRegistry` is called
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
- WHEN `ReadRegistry` is called
- THEN error is non-nil
- AND the error message contains the entry id, "repo", the actual source type ("core"), and a line number

#### Scenario: Parse rejects ref on custom entry (SC-59)

- GIVEN a registry YAML with `source.type: custom` and a `ref` field under `source`
- WHEN `ReadRegistry` is called
- THEN error is non-nil
- AND the error message contains the entry id, "ref", the actual source type ("custom"), and a line number

### Requirement: core upstream Requirement Unchanged

ID: R-117

When `source.type` is `core`, the existing requirement that `upstream.owner`
be non-empty (enforced in `validateEntry`) MUST continue to hold unchanged.

#### Scenario: core entry without upstream.owner still fails

- GIVEN a registry YAML with `source.type: core` and an empty or absent `upstream.owner`
- WHEN `ReadRegistry` is called
- THEN error is non-nil naming the missing `upstream.owner`

### Requirement: Serializer Emits repo/ref for External Entries

ID: R-118, R-119

When `Encode` processes an entry with `source.type == "external"`, it MUST
emit a `repo` line under the `source` block when `Source.Repo` is non-empty,
and a `ref` line when `Source.Ref` is non-empty. Both lines MUST appear at
indent 6 (inside `source:` at indent 4). Emitted `repo`/`ref` values MUST use
the same `scalar()`/`needsQuote()` quoting rules as all other string scalars.

#### Scenario: Encode emits repo and ref lines

- GIVEN a `Registry` with an external entry where `Source.Repo` and `Source.Ref` are both non-empty
- WHEN `Encode(reg)` is called
- THEN the output contains a `repo:` line and a `ref:` line under that entry's `source:` block, correctly indented and quoted

### Requirement: repo/ref Representable Character Validation

ID: R-120

When `checkRepresentable` validates an entry, it MUST check `source.repo` and
`source.ref` values using the existing `representable()` function. A repo or
ref value containing forbidden characters MUST cause `Encode` to return a
non-nil error naming the entry id and the offending field.

#### Scenario: Encode rejects repo value with forbidden chars (SC-61)

- GIVEN a `Registry` with an external entry where `Source.Repo` contains `"{"`
- WHEN `Encode(reg)` is called
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
- WHEN `Encode(reg)` is called, then `ReadRegistry` is called on the result
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

## Issue #29 slice 1 (`skill-package-manager`), synced and renumbered

Synced by Phase 9 item C11 from the delta spec of the change `skill-package-manager` (issue #29, slice 1), archived under `openspec/changes/archive/`, which keeps the original numbers. The text is that spec's, but for two things: every requirement and scenario ID is the original plus 200 (R-001 to R-044 are R-201 to R-244, SC-01 to SC-13 are SC-201 to SC-213), and every heading is one level deeper. Its scope, assumptions and non-goals stay in the archive. Later changes supersede parts of it; `openspec/decisions/standalone-platform-roadmap.md` (the row on Phase 8 and these four changes) records which. The section records slice 1 as it was specified, so "this slice" in it means slice 1, and where it conflicts with a requirement above this heading (for example R-203 against R-112 on `external`), the requirement above holds.

### Part 1 — `skills-registry`: YAML Schema and Parsing

#### Schema Definition

The file `skills.registry.yaml` at the repository root MUST conform to the following
top-level structure when parsed by the engine:

```
version:  string  (required, value "1" in this slice)
skills:   sequence of Entry  (required, may be empty)
```

Each `Entry` MUST carry:

```
id:       string  (required — stable kebab-case identifier, unique per file)
source:   Source  (required)
path:     string  (required — relative path within the skills deploy tree)
install:  Install (required)
lifecycle: Lifecycle (required)
```

`Source`:

```
type:     string  (required — exactly "core" or "custom")
upstream: Upstream (optional — only valid when type == "core")
```

`Upstream` (allowed only when `source.type == "core"`):

```
owner:    string  (required when upstream block is present)
```

`Install`:

```
defaultScope:  string    (required — exactly "global" in this slice)
targets:       []string  (required, non-empty — elements from {"claude","opencode","codex"})
```

`Lifecycle`:

```
updateStrategy:  string  (required — exactly "vendor-merge" or "overlay-only")
```

#### Requirements — Registry Schema

**R-201** WHEN the engine parses `skills.registry.yaml`, it SHALL decode the top-level
`version` field and validate that it equals `"1"`; a missing or unexpected value SHALL
cause a non-nil parse error.

**R-202** WHEN the engine parses a registry entry, it SHALL reject any entry where `id`
is absent or empty with a non-nil error.

**R-203** WHEN the engine parses a registry entry, it SHALL reject any `source.type` value
other than `"core"` or `"custom"` with a non-nil error.

**R-204** IF `source.type` is `"custom"`, the engine SHALL reject any entry that carries
an `upstream` block with a non-nil error.

**R-205** IF `source.type` is `"core"` and an `upstream` block is present, the engine
SHALL reject any `upstream.owner` that is absent or empty with a non-nil error.

**R-206** WHEN the engine parses a registry entry, it SHALL reject any `install.defaultScope`
value other than `"global"` with a non-nil error.

**R-207** WHEN the engine parses a registry entry, it SHALL reject an empty `install.targets`
slice with a non-nil error.

**R-208** WHEN the engine parses a registry entry, it SHALL reject any `install.targets`
element that is not one of `"claude"`, `"opencode"`, or `"codex"` with a non-nil error.

**R-209** WHEN the engine parses a registry entry, it SHALL reject any
`lifecycle.updateStrategy` value other than `"vendor-merge"` or `"overlay-only"` with a
non-nil error.

**R-210** WHEN the engine parses `skills.registry.yaml`, it SHALL detect duplicate `id`
values and return a non-nil error identifying the conflicting id.

**R-211** WHEN the YAML is malformed (not valid YAML), the parser SHALL return a non-nil
error and MUST NOT return partial data.

**R-212** WHEN the engine successfully parses a valid registry file, it SHALL return
entries in the same order they appear in the file (no reordering).

#### Requirements — Population Rules

**R-213** A `managed`-tagged row in `overlay.manifest` whose path component represents a
skill directory (identified by a `*/SKILL.md` row) SHALL map to a registry entry with
`source.type = "core"` and `lifecycle.updateStrategy = "vendor-merge"`.

**R-214** A `custom`-tagged row in `overlay.manifest` whose path component represents a
skill directory (identified by a `*/SKILL.md` row) SHALL map to a registry entry with
`source.type = "custom"` and `lifecycle.updateStrategy = "overlay-only"`.

**R-215** There SHALL be exactly ONE registry entry per skill directory; non-SKILL.md
files within that directory (references/, assets/, evals/) MUST NOT produce separate entries.

**R-216** Files in `overlay.manifest` that do NOT belong to a skill directory — engine
source files (`engine/...`), schema files (`_shared/*.json`), and shared assets
(`_shared/*.md` without a SKILL.md) — SHALL NOT have registry entries.

**R-217** The `id` for each registry entry SHALL be a stable kebab-case string derived
from the skill directory name (e.g., skill directory `genesis-design-system` → `id:
genesis-design-system`).

**R-218** The `path` for each registry entry SHALL be the skill directory path relative
to the skills deploy tree root (e.g., `sdd-spec`, `genesis-design-system`), without a
leading slash.

---

### Part 2 — `skills-cli`: `engine skills` Subcommands

The engine binary gains a new top-level subcommand `skills` dispatched in
`engine/cmd/main.go`. All logic lives in a new `engine/skills/` Go package following
the project's injectable-dependency pattern (Core functions accept `io.Writer`, a file
reader, and an exit function; the `run*` wrappers call them with real OS deps).

#### Requirements — `engine skills list`

**R-219** WHEN `engine skills list` is invoked with a readable, valid `skills.registry.yaml`,
it SHALL write one line per registry entry to stdout and exit 0.

**R-220** Each output line from `engine skills list` SHALL contain, at minimum and in a
stable order: the entry `id`, `source.type`, `lifecycle.updateStrategy`, and
`install.targets` (as a comma-separated list).

**R-221** The output of `engine skills list` SHALL be deterministic: given the same
registry file content, every invocation produces byte-identical output.

**R-222** WHEN `engine skills list` cannot read or parse `skills.registry.yaml`, it
SHALL write a diagnostic to stderr and exit 1 (fail-loud).

**R-223** `engine skills list` SHALL accept a `--registry <path>` flag to override the
default registry path; the default SHALL be `skills.registry.yaml` resolved relative to
the current working directory.

#### Requirements — `engine skills status`

**R-224** WHEN `engine skills status` is invoked with a readable, valid
`skills.registry.yaml`, it SHALL write to stdout: total skill count, count of `core`
entries, count of `custom` entries, and an overall status line (`OK`), then exit 0.

**R-225** WHEN `engine skills status` cannot read or parse `skills.registry.yaml`, it
SHALL write a diagnostic to stderr and exit 1 (fail-loud).

**R-226** `engine skills status` SHALL NOT read or access `overlay.manifest`; its data
source is the registry file only.

**R-227** `engine skills status` SHALL accept a `--registry <path>` flag with the same
default as `engine skills list`.

#### Requirements — `engine skills validate`

**R-228** WHEN `engine skills validate` is invoked, it SHALL cross-check every entry in
`skills.registry.yaml` against `overlay.manifest` and vice versa.

**R-229** A registry entry with path P is "covered" in the manifest when at least one
manifest row has a path whose first directory component equals P.

**R-230** A manifest skill directory D (the top-level directory component of any
`D/SKILL.md` manifest row) is "covered" in the registry when at least one registry entry
has `path == D`.

**R-231** IF any registry entry is NOT covered in the manifest, `engine skills validate`
SHALL write a diagnostic identifying every uncovered entry to stderr and exit 1.

**R-232** IF any manifest skill directory is NOT covered in the registry, `engine skills
validate` SHALL write a diagnostic identifying every uncovered skill directory to stderr
and exit 1.

**R-233** WHEN registry and manifest are fully aligned (all registry entries covered, all
manifest skill directories covered), `engine skills validate` SHALL write a success
message to stdout and exit 0.

**R-234** `engine skills validate` SHALL be a STANDALONE command this slice; it MUST NOT
be invoked or wired inside `cmd_apply`, `cmd_capture`, or any other existing command.

**R-235** `engine skills validate` SHALL accept a `--registry <path>` flag (same default
as list/status) and a `--manifest <path>` flag (default: `overlay.manifest` in the
current working directory).

---

### Part 3 — Bash Router Forwarding

**R-236** `bin/labdrian-overlay` SHALL implement a `cmd_skills` function that forwards
`labdrian skills <verb> [args...]` to `$ENGINE_BINARY skills <verb> [args...]`, using
the same passthrough pattern as `cmd_prespec`.

**R-237** The usage block in `bin/labdrian-overlay` SHALL document the `skills` command
with its supported verbs: `list`, `status`, `validate`.

**R-238** The vendored `bin/overlay` binary SHALL NOT be modified; all new skills CLI
lives in `engine/skills/`, `engine/cmd/main.go`, and `bin/labdrian-overlay`.

---

### Part 4 — Red Lines (Non-Negotiable Invariants)

**R-239** `engine skills list`, `engine skills status`, and `engine skills validate`
SHALL NOT write to `skills.registry.yaml`, `overlay.manifest`, or any file in the
deploy target paths. These commands are read-only in this slice.

**R-240** `cmd_apply` and `cmd_capture` behavior SHALL be byte-identical to their
pre-change behavior; the registry file is purely additive and not read by deploy.

**R-241** All new Go code for the skills commands SHALL reside under `engine/skills/`
and `engine/cmd/main.go`; no skills logic is added to `bin/overlay` or any other
vendored binary.

**R-242** `engine/skills/` SHALL have `go test`-covered unit tests with at least one
table-driven test for the registry parser covering valid and invalid inputs.

---

### Part 5 — Manifest Entries

**R-243** After this change, `overlay.manifest` SHALL contain rows for
`engine/skills/*.go` source files tagged `managed`.

**R-244** After this change, `overlay.manifest` SHALL contain a row for
`skills.registry.yaml` tagged `custom`.

---

### Acceptance Scenarios

Each scenario below corresponds directly to a `go test` case in `engine/skills/`.

#### SC-201 — Parse valid registry: core + custom entries

**Given** a YAML file with version `"1"`, one entry with `source.type: core` and
`lifecycle.updateStrategy: vendor-merge`, and one entry with `source.type: custom` and
`lifecycle.updateStrategy: overlay-only`

**When** the parser is called

**Then** error is nil, result has exactly 2 entries, entries are in declaration order,
first entry `source.type == "core"`, second `source.type == "custom"`.

```go
// maps to: TestParseRegistry/valid_core_and_custom
```

#### SC-202 — Parse rejects missing `id`

**Given** a YAML file with a skill entry where the `id` field is absent

**When** the parser is called

**Then** error is non-nil, returned slice is nil or empty.

```go
// maps to: TestParseRegistry/missing_id
```

#### SC-203 — Parse rejects unknown `source.type`

**Given** a YAML entry with `source.type: external`

**When** the parser is called

**Then** error is non-nil.

```go
// maps to: TestParseRegistry/unknown_source_type
```

#### SC-204 — Parse rejects duplicate ids

**Given** a YAML file with two entries both having `id: sdd-spec`

**When** the parser is called

**Then** error is non-nil and message references the duplicate id.

```go
// maps to: TestParseRegistry/duplicate_id
```

#### SC-205 — Parse rejects upstream block on custom entry

**Given** a YAML entry with `source.type: custom` and a non-empty `upstream` block

**When** the parser is called

**Then** error is non-nil.

```go
// maps to: TestParseRegistry/upstream_on_custom_entry
```

#### SC-206 — `list` writes one line per entry, deterministic

**Given** a registry file with 3 entries (alphabetically non-sorted ids)

**When** `SkillsListCore` is called twice with the same registry content via injected reader

**Then** both calls exit 0, output lines equal 3, both outputs are byte-identical, each
line contains the entry `id`, `source.type`, `updateStrategy`, and `targets`.

```go
// maps to: TestSkillsListCore/deterministic_output
```

#### SC-207 — `list` exits 1 when registry file is missing

**Given** a `--registry` path pointing to a non-existent file

**When** `SkillsListCore` is called

**Then** exit code == 1, stderr is non-empty, stdout is empty.

```go
// maps to: TestSkillsListCore/missing_registry_file
```

#### SC-208 — `status` reports counts and exits 0

**Given** a registry with 3 core entries and 15 custom entries

**When** `SkillsStatusCore` is called

**Then** exit code == 0, stdout contains "18" (total), "3" (core count), "15" (custom
count), and the word "OK".

```go
// maps to: TestSkillsStatusCore/count_report
```

#### SC-209 — `validate` exits 0 when aligned

**Given** a registry with entries for `sdd-spec` and `gadu-orchestrate`, and a manifest
with rows `sdd-spec/SKILL.md managed` and `gadu-orchestrate/SKILL.md custom`

**When** `SkillsValidateCore` is called

**Then** exit code == 0, stdout contains "OK" or equivalent success message, stderr is empty.

```go
// maps to: TestSkillsValidateCore/aligned_registry_and_manifest
```

#### SC-210 — `validate` exits 1 on registry entry missing from manifest

**Given** a registry with an entry for `new-skill`, but the manifest has no
`new-skill/SKILL.md` row

**When** `SkillsValidateCore` is called

**Then** exit code == 1, stderr contains `"new-skill"`.

```go
// maps to: TestSkillsValidateCore/registry_entry_not_in_manifest
```

#### SC-211 — `validate` exits 1 on manifest skill not in registry

**Given** a manifest with `orphan-skill/SKILL.md custom`, but the registry has no
entry with `path: orphan-skill`

**When** `SkillsValidateCore` is called

**Then** exit code == 1, stderr contains `"orphan-skill"`.

```go
// maps to: TestSkillsValidateCore/manifest_skill_not_in_registry
```

#### SC-212 — `validate` ignores non-skill manifest rows

**Given** a registry with entries only for the skill directories, and a manifest that
also contains `engine/propagator/propagator.go managed` and
`_shared/pre-sdd-contracts.md custom`

**When** `SkillsValidateCore` is called with a fully aligned set of skill directories

**Then** exit code == 0; engine and _shared non-SKILL.md rows do not trigger divergence.

```go
// maps to: TestSkillsValidateCore/non_skill_rows_ignored
```

#### SC-213 — Deploy pipeline unchanged: `cmd_apply` not affected by registry presence or absence

**Given** a test environment where `skills.registry.yaml` does or does not exist

**When** `cmd_apply` logic is exercised (unit test of deploy path)

**Then** behavior is identical in both cases; the registry file is never opened by apply.

```go
// maps to: TestApplyIgnoresRegistry (integration-style, skippable with -short)
```
