# Specification: skill-package-manager (First Slice — Read-Only Semantic Layer)

## Purpose

Delta specification for the `skill-package-manager` change, first slice only.
Defines WHAT must be true after the change is applied across two new capabilities:

1. **`skills-registry`** — `skills.registry.yaml` file format, field semantics, and
   population rules derived from `overlay.manifest`.
2. **`skills-cli`** — `engine skills list`, `engine skills status`, and
   `engine skills validate` subcommands, hosted in a new `engine/skills/` Go package,
   forwarded from `bin/labdrian-overlay`.

All deterministic behaviors (YAML parsing, list output shape, validate exit codes and
divergence detection) MUST live in the Go package so they are unit-testable via `go test`.
The deploy pipeline (`cmd_apply`, `cmd_capture`) is unchanged; the registry is purely
additive.

---

## Scope Boundaries

### In Scope (this slice)

- `skills.registry.yaml` schema: `id`, `source{type: core|custom, upstream?}`, `path`,
  `install{defaultScope: global, targets[]}`, `lifecycle{updateStrategy: vendor-merge|overlay-only}`.
- One registry entry per skill directory (identified by a `*/SKILL.md` manifest row).
- New Go package `engine/skills/` with injectable-dependency design for unit testing.
- `engine skills list`, `engine skills status`, `engine skills validate` subcommands
  dispatched from `engine/cmd/main.go`.
- `cmd_skills` forwarder in `bin/labdrian-overlay` (mirrors `cmd_prespec` passthrough).
- New `overlay.manifest` rows for `engine/skills/` source files and `skills.registry.yaml`.

### Out of Scope (explicit non-goals — do NOT spec)

- `skills add` / `skills remove` lifecycle commands.
- `source: external` (third-party git fetch) and `source: project`.
- `install.scope: project`, `install.allowedProjects`, `updateStrategy: pinned-ref`.
- Generating `overlay.manifest` FROM `skills.registry.yaml` (Approach B).
- Wiring `validate` into `cmd_apply` or `cmd_capture`.
- Any modification to the vendored `bin/overlay` binary.
- TUI skill-list screen (deferred to a later slice).

---

## Part 1 — `skills-registry`: YAML Schema and Parsing

### Schema Definition

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

### Requirements — Registry Schema

**R-001** WHEN the engine parses `skills.registry.yaml`, it SHALL decode the top-level
`version` field and validate that it equals `"1"`; a missing or unexpected value SHALL
cause a non-nil parse error.

**R-002** WHEN the engine parses a registry entry, it SHALL reject any entry where `id`
is absent or empty with a non-nil error.

**R-003** WHEN the engine parses a registry entry, it SHALL reject any `source.type` value
other than `"core"` or `"custom"` with a non-nil error.

**R-004** IF `source.type` is `"custom"`, the engine SHALL reject any entry that carries
an `upstream` block with a non-nil error.

**R-005** IF `source.type` is `"core"` and an `upstream` block is present, the engine
SHALL reject any `upstream.owner` that is absent or empty with a non-nil error.

**R-006** WHEN the engine parses a registry entry, it SHALL reject any `install.defaultScope`
value other than `"global"` with a non-nil error.

**R-007** WHEN the engine parses a registry entry, it SHALL reject an empty `install.targets`
slice with a non-nil error.

**R-008** WHEN the engine parses a registry entry, it SHALL reject any `install.targets`
element that is not one of `"claude"`, `"opencode"`, or `"codex"` with a non-nil error.

**R-009** WHEN the engine parses a registry entry, it SHALL reject any
`lifecycle.updateStrategy` value other than `"vendor-merge"` or `"overlay-only"` with a
non-nil error.

**R-010** WHEN the engine parses `skills.registry.yaml`, it SHALL detect duplicate `id`
values and return a non-nil error identifying the conflicting id.

**R-011** WHEN the YAML is malformed (not valid YAML), the parser SHALL return a non-nil
error and MUST NOT return partial data.

**R-012** WHEN the engine successfully parses a valid registry file, it SHALL return
entries in the same order they appear in the file (no reordering).

### Requirements — Population Rules

**R-013** A `managed`-tagged row in `overlay.manifest` whose path component represents a
skill directory (identified by a `*/SKILL.md` row) SHALL map to a registry entry with
`source.type = "core"` and `lifecycle.updateStrategy = "vendor-merge"`.

**R-014** A `custom`-tagged row in `overlay.manifest` whose path component represents a
skill directory (identified by a `*/SKILL.md` row) SHALL map to a registry entry with
`source.type = "custom"` and `lifecycle.updateStrategy = "overlay-only"`.

**R-015** There SHALL be exactly ONE registry entry per skill directory; non-SKILL.md
files within that directory (references/, assets/, evals/) MUST NOT produce separate entries.

**R-016** Files in `overlay.manifest` that do NOT belong to a skill directory — engine
source files (`engine/...`), schema files (`_shared/*.json`), and shared assets
(`_shared/*.md` without a SKILL.md) — SHALL NOT have registry entries.

**R-017** The `id` for each registry entry SHALL be a stable kebab-case string derived
from the skill directory name (e.g., skill directory `genesis-design-system` → `id:
genesis-design-system`).

**R-018** The `path` for each registry entry SHALL be the skill directory path relative
to the skills deploy tree root (e.g., `sdd-spec`, `genesis-design-system`), without a
leading slash.

---

## Part 2 — `skills-cli`: `engine skills` Subcommands

The engine binary gains a new top-level subcommand `skills` dispatched in
`engine/cmd/main.go`. All logic lives in a new `engine/skills/` Go package following
the project's injectable-dependency pattern (Core functions accept `io.Writer`, a file
reader, and an exit function; the `run*` wrappers call them with real OS deps).

### Requirements — `engine skills list`

**R-019** WHEN `engine skills list` is invoked with a readable, valid `skills.registry.yaml`,
it SHALL write one line per registry entry to stdout and exit 0.

**R-020** Each output line from `engine skills list` SHALL contain, at minimum and in a
stable order: the entry `id`, `source.type`, `lifecycle.updateStrategy`, and
`install.targets` (as a comma-separated list).

**R-021** The output of `engine skills list` SHALL be deterministic: given the same
registry file content, every invocation produces byte-identical output.

**R-022** WHEN `engine skills list` cannot read or parse `skills.registry.yaml`, it
SHALL write a diagnostic to stderr and exit 1 (fail-loud).

**R-023** `engine skills list` SHALL accept a `--registry <path>` flag to override the
default registry path; the default SHALL be `skills.registry.yaml` resolved relative to
the current working directory.

### Requirements — `engine skills status`

**R-024** WHEN `engine skills status` is invoked with a readable, valid
`skills.registry.yaml`, it SHALL write to stdout: total skill count, count of `core`
entries, count of `custom` entries, and an overall status line (`OK`), then exit 0.

**R-025** WHEN `engine skills status` cannot read or parse `skills.registry.yaml`, it
SHALL write a diagnostic to stderr and exit 1 (fail-loud).

**R-026** `engine skills status` SHALL NOT read or access `overlay.manifest`; its data
source is the registry file only.

**R-027** `engine skills status` SHALL accept a `--registry <path>` flag with the same
default as `engine skills list`.

### Requirements — `engine skills validate`

**R-028** WHEN `engine skills validate` is invoked, it SHALL cross-check every entry in
`skills.registry.yaml` against `overlay.manifest` and vice versa.

**R-029** A registry entry with path P is "covered" in the manifest when at least one
manifest row has a path whose first directory component equals P.

**R-030** A manifest skill directory D (the top-level directory component of any
`D/SKILL.md` manifest row) is "covered" in the registry when at least one registry entry
has `path == D`.

**R-031** IF any registry entry is NOT covered in the manifest, `engine skills validate`
SHALL write a diagnostic identifying every uncovered entry to stderr and exit 1.

**R-032** IF any manifest skill directory is NOT covered in the registry, `engine skills
validate` SHALL write a diagnostic identifying every uncovered skill directory to stderr
and exit 1.

**R-033** WHEN registry and manifest are fully aligned (all registry entries covered, all
manifest skill directories covered), `engine skills validate` SHALL write a success
message to stdout and exit 0.

**R-034** `engine skills validate` SHALL be a STANDALONE command this slice; it MUST NOT
be invoked or wired inside `cmd_apply`, `cmd_capture`, or any other existing command.

**R-035** `engine skills validate` SHALL accept a `--registry <path>` flag (same default
as list/status) and a `--manifest <path>` flag (default: `overlay.manifest` in the
current working directory).

---

## Part 3 — Bash Router Forwarding

**R-036** `bin/labdrian-overlay` SHALL implement a `cmd_skills` function that forwards
`labdrian skills <verb> [args...]` to `$ENGINE_BINARY skills <verb> [args...]`, using
the same passthrough pattern as `cmd_prespec`.

**R-037** The usage block in `bin/labdrian-overlay` SHALL document the `skills` command
with its supported verbs: `list`, `status`, `validate`.

**R-038** The vendored `bin/overlay` binary SHALL NOT be modified; all new skills CLI
lives in `engine/skills/`, `engine/cmd/main.go`, and `bin/labdrian-overlay`.

---

## Part 4 — Red Lines (Non-Negotiable Invariants)

**R-039** `engine skills list`, `engine skills status`, and `engine skills validate`
SHALL NOT write to `skills.registry.yaml`, `overlay.manifest`, or any file in the
deploy target paths. These commands are read-only in this slice.

**R-040** `cmd_apply` and `cmd_capture` behavior SHALL be byte-identical to their
pre-change behavior; the registry file is purely additive and not read by deploy.

**R-041** All new Go code for the skills commands SHALL reside under `engine/skills/`
and `engine/cmd/main.go`; no skills logic is added to `bin/overlay` or any other
vendored binary.

**R-042** `engine/skills/` SHALL have `go test`-covered unit tests with at least one
table-driven test for the registry parser covering valid and invalid inputs.

---

## Part 5 — Manifest Entries

**R-043** After this change, `overlay.manifest` SHALL contain rows for
`engine/skills/*.go` source files tagged `managed`.

**R-044** After this change, `overlay.manifest` SHALL contain a row for
`skills.registry.yaml` tagged `custom`.

---

## Acceptance Scenarios

Each scenario below corresponds directly to a `go test` case in `engine/skills/`.

### SC-01 — Parse valid registry: core + custom entries

**Given** a YAML file with version `"1"`, one entry with `source.type: core` and
`lifecycle.updateStrategy: vendor-merge`, and one entry with `source.type: custom` and
`lifecycle.updateStrategy: overlay-only`

**When** the parser is called

**Then** error is nil, result has exactly 2 entries, entries are in declaration order,
first entry `source.type == "core"`, second `source.type == "custom"`.

```go
// maps to: TestParseRegistry/valid_core_and_custom
```

### SC-02 — Parse rejects missing `id`

**Given** a YAML file with a skill entry where the `id` field is absent

**When** the parser is called

**Then** error is non-nil, returned slice is nil or empty.

```go
// maps to: TestParseRegistry/missing_id
```

### SC-03 — Parse rejects unknown `source.type`

**Given** a YAML entry with `source.type: external`

**When** the parser is called

**Then** error is non-nil.

```go
// maps to: TestParseRegistry/unknown_source_type
```

### SC-04 — Parse rejects duplicate ids

**Given** a YAML file with two entries both having `id: sdd-spec`

**When** the parser is called

**Then** error is non-nil and message references the duplicate id.

```go
// maps to: TestParseRegistry/duplicate_id
```

### SC-05 — Parse rejects upstream block on custom entry

**Given** a YAML entry with `source.type: custom` and a non-empty `upstream` block

**When** the parser is called

**Then** error is non-nil.

```go
// maps to: TestParseRegistry/upstream_on_custom_entry
```

### SC-06 — `list` writes one line per entry, deterministic

**Given** a registry file with 3 entries (alphabetically non-sorted ids)

**When** `SkillsListCore` is called twice with the same registry content via injected reader

**Then** both calls exit 0, output lines equal 3, both outputs are byte-identical, each
line contains the entry `id`, `source.type`, `updateStrategy`, and `targets`.

```go
// maps to: TestSkillsListCore/deterministic_output
```

### SC-07 — `list` exits 1 when registry file is missing

**Given** a `--registry` path pointing to a non-existent file

**When** `SkillsListCore` is called

**Then** exit code == 1, stderr is non-empty, stdout is empty.

```go
// maps to: TestSkillsListCore/missing_registry_file
```

### SC-08 — `status` reports counts and exits 0

**Given** a registry with 3 core entries and 15 custom entries

**When** `SkillsStatusCore` is called

**Then** exit code == 0, stdout contains "18" (total), "3" (core count), "15" (custom
count), and the word "OK".

```go
// maps to: TestSkillsStatusCore/count_report
```

### SC-09 — `validate` exits 0 when aligned

**Given** a registry with entries for `sdd-spec` and `gadu-orchestrate`, and a manifest
with rows `sdd-spec/SKILL.md managed` and `gadu-orchestrate/SKILL.md custom`

**When** `SkillsValidateCore` is called

**Then** exit code == 0, stdout contains "OK" or equivalent success message, stderr is empty.

```go
// maps to: TestSkillsValidateCore/aligned_registry_and_manifest
```

### SC-10 — `validate` exits 1 on registry entry missing from manifest

**Given** a registry with an entry for `new-skill`, but the manifest has no
`new-skill/SKILL.md` row

**When** `SkillsValidateCore` is called

**Then** exit code == 1, stderr contains `"new-skill"`.

```go
// maps to: TestSkillsValidateCore/registry_entry_not_in_manifest
```

### SC-11 — `validate` exits 1 on manifest skill not in registry

**Given** a manifest with `orphan-skill/SKILL.md custom`, but the registry has no
entry with `path: orphan-skill`

**When** `SkillsValidateCore` is called

**Then** exit code == 1, stderr contains `"orphan-skill"`.

```go
// maps to: TestSkillsValidateCore/manifest_skill_not_in_registry
```

### SC-12 — `validate` ignores non-skill manifest rows

**Given** a registry with entries only for the skill directories, and a manifest that
also contains `engine/propagator/propagator.go managed` and
`_shared/pre-sdd-contracts.md custom`

**When** `SkillsValidateCore` is called with a fully aligned set of skill directories

**Then** exit code == 0; engine and _shared non-SKILL.md rows do not trigger divergence.

```go
// maps to: TestSkillsValidateCore/non_skill_rows_ignored
```

### SC-13 — Deploy pipeline unchanged: `cmd_apply` not affected by registry presence or absence

**Given** a test environment where `skills.registry.yaml` does or does not exist

**When** `cmd_apply` logic is exercised (unit test of deploy path)

**Then** behavior is identical in both cases; the registry file is never opened by apply.

```go
// maps to: TestApplyIgnoresRegistry (integration-style, skippable with -short)
```

---

## Assumptions Carried Forward from Proposal

- **A-1**: `install.defaultScope` is fixed to `"global"` and the three target values are
  exactly `"claude"`, `"opencode"`, `"codex"`. No other values are valid in this slice.
- **A-2**: `validate` is standalone; it MUST NOT be wired into `cmd_apply` or
  `cmd_capture`. Wiring is explicitly deferred.
- **A-3**: `id` is derived from the skill directory name only; id generation is not
  enforced at parse time (the populated YAML is hand-authored this slice), but duplicate
  detection is enforced.
- **A-4**: Reference/asset files within a skill directory are NOT checked by validate;
  only the presence of the skill directory root is cross-checked.
- **A-5**: `engine/skills/` new source files are tracked in `overlay.manifest` as
  `managed` (they originate from the upstream gentle-ai engine track).
