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

#### Scenario: AddCore assertions for a lint-clean SKILL.md are unchanged

- GIVEN the existing `AddCore` test suite (SC-65 through SC-68 and prior
  cases), whose SKILL.md fixtures are updated to be lint-clean (zero hard
  lint errors) while every test assertion stays exactly as written
- WHEN this capability's `LintSkill` gate is added
- THEN every existing `AddCore` test still passes, with only its SKILL.md
  fixtures changed and none of its assertions changed

#### Scenario: A previously-passing fixture that is not lint-clean is now refused

- GIVEN a SKILL.md fixture that passed `AddCore` before this change but has
  a hard lint error (for example a missing `metadata.version`)
- WHEN `AddCore` is run against it after this capability's gate is added
- THEN the add is refused, proving the gate rejects what it used to accept

#### Scenario: The lint gate runs before any manifest or registry write

- GIVEN a SKILL.md with a hard lint error and a registry/manifest pair that
  would otherwise validate
- WHEN `AddCore` processes the add
- THEN the lint check happens before the manifest is appended and before
  the registry is serialized
- AND no partial write (manifest-only or registry-only) occurs as a result
  of the refusal

## Issue #29 slice 3 (`skill-lifecycle`), synced and renumbered

Synced by Phase 9 item C11 from the delta spec of the change `skill-lifecycle` (issue #29, slice 3), archived under `openspec/changes/archive/`, which keeps the original numbers. The text is that spec's, but for two things: every requirement and scenario ID is the original plus 400 (R-050 to R-083 are R-450 to R-483, SC-20 to SC-38 are SC-420 to SC-438), and every heading is one level deeper. Its scope, assumptions and non-goals stay in the archive. Later changes supersede parts of it; `openspec/decisions/standalone-platform-roadmap.md` (the row on Phase 8 and these four changes) records which. The section records slice 3 as it was specified, so "this slice" in it means slice 3, and where it conflicts with a requirement above this heading (for example R-462 against R-127 on `--repo`), the requirement above holds.

### Flag Contract for `AddCore` / `RemoveCore`

Both functions accept the same flag set as existing verb cores. The `cmd_skills` shell
function in `bin/labdrian-overlay` already injects all three defaults if the caller does
not supply them:

| Flag | Default | Purpose |
|------|---------|---------|
| `--registry <path>` | `skills.registry.yaml` | Path to the registry file to mutate |
| `--manifest <path>` | `overlay.manifest` | Path to the manifest file to keep in sync |
| `--source-root <path>` | `skills` | Root directory under which `<id>/SKILL.md` is checked |
| `<id>` (positional, after flags) | — | The skill identifier to add or remove |

`cmd_skills` in `bin/labdrian-overlay` is **not modified** — the existing `--registry`,
`--manifest`, and `--source-root` injection already covers the new verbs (R-478).

---

### Requirements

#### Serializer

**R-450** [new-capability]
WHEN `Serialize(reg Registry)` is called,
THEN it SHALL return `([]byte, error)` containing a complete strict-subset YAML
representation of `reg`, or a non-nil error if serialization fails internally.

**R-451** [new-capability]
The output of `Serialize` SHALL be confined to the accepted YAML subset:
block style only; no flow mappings (`{}`), no flow sequences (`[]`),
no YAML anchors (`&`/`*`), no YAML tags (`!`), no block scalars (`|`/`>`),
no inline comments (` # ` inside a value line).

**R-452** [new-capability]
`Serialize` SHALL use exactly 2-space indentation at every nesting level.

**R-453** [new-capability]
Scalar quoting rule: a scalar value SHALL be wrapped in double quotes when ANY of
the following is true:
- (a) the value is the empty string
- (b) the value contains the substring `": "`
- (c) the value contains the substring `" # "`
- (d) the value starts with `"` or `'`

In all other cases the scalar SHALL be emitted unquoted.
Special rule: the top-level `version` field value SHALL always be double-quoted,
regardless of the conditions above, matching the existing `skills.registry.yaml` style.

**R-454** [new-capability]
Round-trip invariant: for any `Registry` value `r` that was itself produced by
`ParseRegistry` without error, calling `ParseRegistry(Serialize(r))` SHALL return
a `Registry` equal to `r` (every field equal) with a nil error.

**R-455** [new-capability]
`Serialize` SHALL be deterministic: identical `Registry` values SHALL always produce
identical byte slices.

**R-456** [new-capability]
Entry ordering: `Serialize` SHALL emit entries in the order they appear in
`Registry.Skills`. No sorting or reordering is applied.

**R-457** [new-capability]
Conditional `upstream` block: the `source.upstream` sub-mapping SHALL be emitted
only when `entry.Source.Upstream != nil`. For `source.type: custom` entries (where
`entry.Source.Upstream` is nil by the schema enforced by `validateEntry`) the
`upstream` key SHALL be absent from the output.

**R-458** [new-capability]
Conditional `allowedProjects` sequence: the `allowedProjects` sequence SHALL be
emitted only when `len(entry.Install.AllowedProjects) > 0`. It SHALL be omitted
for entries with a nil or empty slice.

**R-459** [new-capability]
The output of `Serialize` SHALL, when fed back to `ParseRegistry`, produce a
registry in which every entry passes `validateEntry` without error.

---

#### `skills add`

**R-460** [feature]
WHEN `skills add <id>` is invoked AND the file `<source-root>/<id>/SKILL.md` does
not exist on disk,
THEN the command SHALL exit with a non-zero status, write a descriptive error
message to stderr, and leave the registry file and manifest file byte-unchanged.

**R-461** [feature]
WHEN `skills add <id>` is invoked AND `<id>` is already present in the registry,
THEN the command SHALL exit with a non-zero status, write a descriptive error
message to stderr, and leave both files byte-unchanged.
`add` on an existing id is an ERROR, never a silent no-op.

**R-462** [feature]
WHEN `skills add <id>` succeeds (all preconditions pass),
THEN a new entry SHALL be appended to the end of `Registry.Skills` with the
following field values:

| Field | Value |
|-------|-------|
| `id` | `<id>` |
| `path` | `<id>` (no trailing slash) |
| `source.type` | `custom` |
| `source.upstream` | absent (nil) |
| `install.defaultScope` | `global` |
| `install.targets` | `[claude, opencode, codex]` (in that order) |
| `install.allowedProjects` | absent (empty slice) |
| `lifecycle.updateStrategy` | `overlay-only` |

**R-463** [feature]
Before writing the registry, `skills add` SHALL call `validateEntry` on the
new entry. If validation returns a non-nil error, the command SHALL exit non-zero
and leave both files unchanged.

**R-464** [feature]
The registry write by `skills add` SHALL be atomic: the new content SHALL be
written to a temporary file in the same directory as the registry, then that file
SHALL be renamed to the registry path. If any error occurs before the rename
completes, the original registry file SHALL remain byte-unchanged.

**R-465** [feature]
The manifest write by `skills add` SHALL be atomic: the updated manifest content
SHALL be written to a temporary file, then renamed to the manifest path.
Both temporary files SHALL be fully prepared before either rename is executed.
If either temporary write fails, no rename SHALL occur and both original files
SHALL remain byte-unchanged.

**R-466** [feature]
`skills add <id>` SHALL append exactly one row to `overlay.manifest`:
```
<id>/SKILL.md custom
```
This row is appended at the end of the manifest. Existing manifest rows are
preserved in their original order.

**R-467** [feature]
After a successful `skills add <id>`:
- `ParseRegistry` on the registry file SHALL return no error.
- The returned `Registry.Skills` SHALL contain an entry with `id == "<id>"`.
- `Validate(registry, manifest)` SHALL report zero divergences for the new entry.

**R-468** [feature]
`skills add` SHALL exit 0 on success and write a single confirmation line to
stdout (e.g., `added: <id>`).

---

#### `skills remove`

**R-469** [feature]
WHEN `skills remove <id>` is invoked AND `<id>` is NOT present in the registry,
THEN the command SHALL exit with a non-zero status, write a descriptive error
message to stderr, and leave both files byte-unchanged.

**R-470** [feature]
WHEN `skills remove <id>` succeeds,
THEN the entry with the given `<id>` SHALL be absent from the registry.
All other entries SHALL be preserved in their original relative order.

**R-471** [feature]
`skills remove <id>` SHALL NOT delete, move, or modify any file under
`<source-root>/<id>/` on disk. The skill directory is left fully intact.

**R-472** [feature]
The registry write by `skills remove` SHALL be atomic (temp+rename), with the
same dual-temp contract as `add` (R-464, R-465): both temp files prepared before
either rename; on any temp-write error, both originals remain byte-unchanged.

**R-473** [feature]
`skills remove <id>` SHALL remove from `overlay.manifest` every line whose path
component equals `<id>/SKILL.md`. Non-matching lines are preserved verbatim.

**R-474** [feature]
After a successful `skills remove <id>`:
- `ParseRegistry` on the registry file SHALL return no error.
- The returned `Registry.Skills` SHALL NOT contain any entry with `id == "<id>"`.
- `Validate(registry, manifest)` SHALL report zero divergences
  (the id is absent from both files).

**R-475** [feature]
`skills remove` SHALL exit 0 on success and write a single confirmation line to
stdout (e.g., `removed: <id>`).

---

#### CLI Dispatch

**R-476** [feature]
`SkillsCore` (in `engine/skills/skills.go`) SHALL handle verb `"add"` and verb
`"remove"`, dispatching to `AddCore` and `RemoveCore` respectively, using the
same I/O-injection pattern as existing verb cores
(`readFile`, `stdout`, `stderr`, `exit func(int)`).

**R-477** [feature]
`AddCore` and `RemoveCore` SHALL also accept a `statFile` (or equivalent) injection
for probing `<source-root>/<id>/SKILL.md` existence, so the precondition check is
testable without real filesystem paths.

**R-478** [feature]
`bin/labdrian-overlay cmd_skills` SHALL NOT be modified. The existing injection of
`--registry`, `--manifest`, and `--source-root` defaults already covers the new
verbs.

**R-479** [feature]
The files `bin/overlay`, `cmd_apply`, `cmd_capture`, and `engine/cmd/main.go`
SHALL be unchanged by this slice.

**R-480** [feature]
The error messages emitted by `SkillsCore` for an unknown or missing verb SHALL
include `add` and `remove` in the listed set of supported verbs.

---

#### Red-Line Invariants

**R-481** [fix]
No new third-party (non-stdlib) Go imports SHALL be introduced. `serialize.go`
and `lifecycle.go` SHALL use only packages from the Go standard library.

**R-482** [fix]
A failed registry write (any error path in `add` or `remove`, including simulated
I/O errors) SHALL leave the registry file byte-unchanged. This is an invariant
verified by `t.TempDir`-based tests that inject write errors.

**R-483** [fix]
A failed manifest write, when it occurs before either rename executes, SHALL also
leave the registry file byte-unchanged (both files safe together per R-465/R-472).

---

### Acceptance Scenarios

#### Serializer

**SC-420** — Round-trip: real 18-entry registry

```
Given:  the raw bytes of the real skills.registry.yaml (18 entries, all fields)
When:   reg, _ := ParseRegistry(bytes.NewReader(rawBytes))
        out, err := Serialize(reg)                       // must not error
        reg2, err2 := ParseRegistry(bytes.NewReader(out)) // must not error
Then:   err == nil && err2 == nil
        reg.Version == reg2.Version
        len(reg.Skills) == 18 && len(reg2.Skills) == 18
        for i in 0..17: reg.Skills[i] == reg2.Skills[i]  // field-by-field equality
```

Test file: `engine/skills/serialize_test.go`, function `TestSerializeRoundTripRealRegistry`.

**SC-421** — Round-trip: project-scoped entry with allowedProjects

```
Given:  reg := Registry{Version: "1", Skills: []Entry{{
          ID: "p-skill", Path: "p-skill",
          Source: Source{Type: "custom"},
          Install: Install{
            DefaultScope: "project",
            Targets: []string{"claude", "opencode", "codex"},
            AllowedProjects: []string{"project-a", "project-b"},
          },
          Lifecycle: Lifecycle{UpdateStrategy: "overlay-only"},
        }}}
When:   out, _ := Serialize(reg)
        reg2, err := ParseRegistry(bytes.NewReader(out))
Then:   err == nil
        reg2.Skills[0].Install.DefaultScope == "project"
        reg2.Skills[0].Install.AllowedProjects == ["project-a", "project-b"]
```

Test: `TestSerializeRoundTripAllowedProjects`.

**SC-422** — Determinism

```
Given:  any valid Registry reg (use the 18-entry real registry)
When:   out1, _ := Serialize(reg)
        out2, _ := Serialize(reg)
Then:   bytes.Equal(out1, out2)
```

Test: `TestSerializeDeterministic`.

**SC-423** — Upstream block absent for custom entries

```
Given:  reg with one custom entry (source.type="custom", upstream=nil)
When:   out, _ := Serialize(reg)
Then:   !bytes.Contains(out, []byte("upstream"))
```

Test: `TestSerializeNoUpstreamForCustom`.

**SC-424** — Upstream block present for core entries

```
Given:  reg with one core entry (source.type="core", upstream.owner="some-owner")
When:   out, _ := Serialize(reg)
        reg2, _ := ParseRegistry(bytes.NewReader(out))
Then:   reg2.Skills[0].Source.Upstream != nil
        reg2.Skills[0].Source.Upstream.Owner == "some-owner"
```

Test: `TestSerializeUpstreamForCore`.

**SC-425** — Golden: two-entry registry (one core, one custom)

```
Given:  reg := Registry{Version: "1", Skills: []Entry{
          {ID: "core-skill", Path: "core-skill",
           Source: Source{Type: "core", Upstream: &Upstream{Owner: "acme"}},
           Install: Install{DefaultScope: "global",
                            Targets: []string{"claude", "opencode", "codex"}},
           Lifecycle: Lifecycle{UpdateStrategy: "vendor-merge"}},
          {ID: "my-skill", Path: "my-skill",
           Source: Source{Type: "custom"},
           Install: Install{DefaultScope: "global",
                            Targets: []string{"claude", "opencode", "codex"}},
           Lifecycle: Lifecycle{UpdateStrategy: "overlay-only"}},
        }}
When:   out, _ := Serialize(reg)
        (on first run with -update flag, write out to testdata/golden/two_entry.yaml)
Then:   bytes.Equal(out, goldenBytes)       // golden match
        ParseRegistry succeeds on out
        validateEntry passes for both entries
```

Test: `TestSerializeGoldenTwoEntry` (table-driven golden pattern per go-testing skill).

---

#### `skills add`

**SC-426** — Success path

```
Given:  tmp := t.TempDir()
        write minimal valid registry to tmp/skills.registry.yaml (one custom entry "existing")
        write matching manifest to tmp/overlay.manifest with row "existing/SKILL.md custom"
        os.MkdirAll(tmp/skills/foo) and write tmp/skills/foo/SKILL.md
When:   AddCore(
          []string{"--registry", tmp+"/skills.registry.yaml",
                   "--manifest", tmp+"/overlay.manifest",
                   "--source-root", tmp+"/skills", "foo"},
          ..., stdout, stderr, exitFn)
Then:   exit code 0
        ParseRegistry(registry file) succeeds
        returned registry has id="foo" at end of Skills slice
        registry entry for "foo" has source.type="custom", defaultScope="global",
          targets=[claude,opencode,codex], updateStrategy="overlay-only"
        overlay.manifest contains line "foo/SKILL.md custom"
        Validate(registry, manifest) returns zero divergences
```

Test: `TestAddCoreSuccess` in `engine/skills/lifecycle_test.go`.

**SC-427** — Missing SKILL.md

```
Given:  valid registry (no "foo" entry); skills/foo/ dir exists but SKILL.md absent
When:   AddCore([..., "foo"], ...)
Then:   exit code != 0
        stderr contains non-empty error message mentioning "foo"
        registry file is byte-unchanged (stat shows same mtime or byte comparison)
        manifest file is byte-unchanged
```

Test: `TestAddCoreMissingSkillMD`.

**SC-428** — ID already present

```
Given:  valid registry containing entry with id="foo"; skills/foo/SKILL.md exists
When:   AddCore([..., "foo"], ...)
Then:   exit code != 0
        stderr contains error message
        registry file is byte-unchanged
```

Test: `TestAddCoreIDAlreadyPresent`.

**SC-429** — Atomic safety: registry temp write fails

```
Given:  valid registry; skills/foo/SKILL.md present; registry directory made read-only
        (chmod 0555 on the dir so temp file creation fails)
When:   AddCore([..., "foo"], ...)
Then:   exit code != 0
        original registry file content unchanged
        original manifest content unchanged
```

Test: `TestAddCoreRegistryWriteFailureIsAtomic`. (Use `t.TempDir`; chmod dir before call.)

**SC-430** — Validates before write: bad entry rejected

```
Given:  a custom AddCore implementation where the entry is constructed with an
        invalid updateStrategy (inject via table test or a wrapper)
When:   the validate-before-write check runs
Then:   exit code != 0; registry unchanged
```

Test: `TestAddCoreValidateBeforeWrite`. (Can be unit-tested on the pure AddEntry helper.)

---

#### `skills remove`

**SC-431** — Success path

```
Given:  valid registry with entries ["existing", "foo", "other"]
        manifest with rows "existing/SKILL.md custom", "foo/SKILL.md custom",
          "other/SKILL.md custom"
When:   RemoveCore([..., "foo"], ...)
Then:   exit code 0
        ParseRegistry(registry) returns skills ["existing", "other"] in that order
        manifest does NOT contain "foo/SKILL.md"
        manifest DOES contain "existing/SKILL.md custom" and "other/SKILL.md custom"
        Validate(registry, manifest) returns zero divergences
```

Test: `TestRemoveCoreSuccess`.

**SC-432** — ID absent

```
Given:  valid registry without id="foo"
When:   RemoveCore([..., "foo"], ...)
Then:   exit code != 0
        stderr contains error message
        registry file byte-unchanged
```

Test: `TestRemoveCoreIDAbsent`.

**SC-433** — Skill dir not deleted

```
Given:  valid registry with "foo"; skills/foo/SKILL.md exists
When:   RemoveCore([..., "foo"], ...)
Then:   exit code 0
        skills/foo/SKILL.md still exists (os.Stat does not return an error)
```

Test: `TestRemoveCoreDoesNotDeleteDir`.

**SC-434** — Atomic safety: manifest temp write fails

```
Given:  valid registry with "foo"; manifest directory made read-only after
        registry temp is written (or inject manifest write error)
When:   RemoveCore([..., "foo"], ...)
Then:   exit code != 0
        (if both renames blocked by dual-temp contract) registry is byte-unchanged
```

Test: `TestRemoveCoreManifestWriteFailureIsAtomic`.

---

#### Add + Remove cycle

**SC-435** — Validate stays aligned through add+remove

```
Given:  minimal valid registry (one entry "base") + matching manifest
        skills/new-skill/SKILL.md present
When:   AddCore([..., "new-skill"], ...)   // step 1
        RemoveCore([..., "new-skill"], ...) // step 2
Then:   after step 1: Validate returns zero divergences; "new-skill" in registry
        after step 2: Validate returns zero divergences; "new-skill" NOT in registry
        "base" entry still present and unchanged after both operations
```

Test: `TestAddRemoveCycleValidateAligned`.

---

#### CLI dispatch

**SC-436** — SkillsCore dispatches add and remove

```
Given:  in-memory setup with valid registry + SKILL.md present (injected via mocks)
When:   SkillsCore("add", [..., "new-id"], ...) → should not emit "unknown verb"
        SkillsCore("remove", [..., "some-id"], ...)
Then:   neither call writes "unknown skills verb" to stderr
```

Test: `TestSkillsCoreDispatchAddRemove`.

**SC-437** — Unknown verb message includes add and remove

```
Given:  SkillsCore("bogus", [], ...)
When:   called
Then:   exit code 1
        stderr contains both "add" and "remove" as supported verbs
```

Test: `TestSkillsCoreUnknownVerbMessage`.

**SC-438** — Zero stdlib-only imports (build-time assertion)

```
Given:  go list -f '{{join .Imports "\n"}}' ./engine/skills/...
Then:   no line matches a non-stdlib import path that was not present before this slice
```

Can be checked as a `go build` pass in CI or as a comment-enforced manual gate.
