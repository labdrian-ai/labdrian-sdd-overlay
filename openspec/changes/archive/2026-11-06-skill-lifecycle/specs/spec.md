# Spec: Mutable Skill Registry — `skills add` / `skills remove` (issue #29, slice 3)

## Scope

Delta spec for `skill-lifecycle` slice 3. Numbering continues from slice 2: requirements
R-050+, scenarios SC-20+. This spec covers:

1. A strict-subset YAML **serializer** (`Serialize(Registry) ([]byte, error)`)
2. **`skills add <id>`** — register an existing skill dir into the registry + manifest
3. **`skills remove <id>`** — remove a registry entry + manifest row
4. **CLI dispatch** — SkillsCore + `bin/labdrian-overlay cmd_skills` passthrough
5. **Red-line invariants** — crash safety, zero third-party deps, zero global mutations

---

## Assumptions

These assumptions resolve the two design forks flagged in the proposal. No design artifact
exists; the spec adopts the proposal-recommended path and labels them explicitly so the
design phase (if run) can override with a superseding design decision.

| ID    | Assumption |
|-------|-----------|
| A-050 | **Manifest sync is included**: `skills add`/`remove` ALSO keep `overlay.manifest` in sync. The registry-only alternative (let `validate` report divergence) is rejected. |
| A-051 | **Full re-emit serializer**: `Serialize` reconstructs the entire file from the parsed model. Comments and hand-formatting in the original registry are not preserved after any write operation. |
| A-052 | **Path = id (no trailing slash)**: the `path` field for a newly added entry equals `<id>` exactly, matching the format of all 18 existing entries in `skills.registry.yaml`. |

---

## Flag Contract for `AddCore` / `RemoveCore`

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
`--manifest`, and `--source-root` injection already covers the new verbs (R-078).

---

## Requirements

### Serializer

**R-050** [new-capability]
WHEN `Serialize(reg Registry)` is called,
THEN it SHALL return `([]byte, error)` containing a complete strict-subset YAML
representation of `reg`, or a non-nil error if serialization fails internally.

**R-051** [new-capability]
The output of `Serialize` SHALL be confined to the accepted YAML subset:
block style only; no flow mappings (`{}`), no flow sequences (`[]`),
no YAML anchors (`&`/`*`), no YAML tags (`!`), no block scalars (`|`/`>`),
no inline comments (` # ` inside a value line).

**R-052** [new-capability]
`Serialize` SHALL use exactly 2-space indentation at every nesting level.

**R-053** [new-capability]
Scalar quoting rule: a scalar value SHALL be wrapped in double quotes when ANY of
the following is true:
- (a) the value is the empty string
- (b) the value contains the substring `": "`
- (c) the value contains the substring `" # "`
- (d) the value starts with `"` or `'`

In all other cases the scalar SHALL be emitted unquoted.
Special rule: the top-level `version` field value SHALL always be double-quoted,
regardless of the conditions above, matching the existing `skills.registry.yaml` style.

**R-054** [new-capability]
Round-trip invariant: for any `Registry` value `r` that was itself produced by
`ParseRegistry` without error, calling `ParseRegistry(Serialize(r))` SHALL return
a `Registry` equal to `r` (every field equal) with a nil error.

**R-055** [new-capability]
`Serialize` SHALL be deterministic: identical `Registry` values SHALL always produce
identical byte slices.

**R-056** [new-capability]
Entry ordering: `Serialize` SHALL emit entries in the order they appear in
`Registry.Skills`. No sorting or reordering is applied.

**R-057** [new-capability]
Conditional `upstream` block: the `source.upstream` sub-mapping SHALL be emitted
only when `entry.Source.Upstream != nil`. For `source.type: custom` entries (where
`entry.Source.Upstream` is nil by the schema enforced by `validateEntry`) the
`upstream` key SHALL be absent from the output.

**R-058** [new-capability]
Conditional `allowedProjects` sequence: the `allowedProjects` sequence SHALL be
emitted only when `len(entry.Install.AllowedProjects) > 0`. It SHALL be omitted
for entries with a nil or empty slice.

**R-059** [new-capability]
The output of `Serialize` SHALL, when fed back to `ParseRegistry`, produce a
registry in which every entry passes `validateEntry` without error.

---

### `skills add`

**R-060** [feature]
WHEN `skills add <id>` is invoked AND the file `<source-root>/<id>/SKILL.md` does
not exist on disk,
THEN the command SHALL exit with a non-zero status, write a descriptive error
message to stderr, and leave the registry file and manifest file byte-unchanged.

**R-061** [feature]
WHEN `skills add <id>` is invoked AND `<id>` is already present in the registry,
THEN the command SHALL exit with a non-zero status, write a descriptive error
message to stderr, and leave both files byte-unchanged.
`add` on an existing id is an ERROR, never a silent no-op.

**R-062** [feature]
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

**R-063** [feature]
Before writing the registry, `skills add` SHALL call `validateEntry` on the
new entry. If validation returns a non-nil error, the command SHALL exit non-zero
and leave both files unchanged.

**R-064** [feature]
The registry write by `skills add` SHALL be atomic: the new content SHALL be
written to a temporary file in the same directory as the registry, then that file
SHALL be renamed to the registry path. If any error occurs before the rename
completes, the original registry file SHALL remain byte-unchanged.

**R-065** [feature]
The manifest write by `skills add` SHALL be atomic: the updated manifest content
SHALL be written to a temporary file, then renamed to the manifest path.
Both temporary files SHALL be fully prepared before either rename is executed.
If either temporary write fails, no rename SHALL occur and both original files
SHALL remain byte-unchanged.

**R-066** [feature]
`skills add <id>` SHALL append exactly one row to `overlay.manifest`:
```
<id>/SKILL.md custom
```
This row is appended at the end of the manifest. Existing manifest rows are
preserved in their original order.

**R-067** [feature]
After a successful `skills add <id>`:
- `ParseRegistry` on the registry file SHALL return no error.
- The returned `Registry.Skills` SHALL contain an entry with `id == "<id>"`.
- `Validate(registry, manifest)` SHALL report zero divergences for the new entry.

**R-068** [feature]
`skills add` SHALL exit 0 on success and write a single confirmation line to
stdout (e.g., `added: <id>`).

---

### `skills remove`

**R-069** [feature]
WHEN `skills remove <id>` is invoked AND `<id>` is NOT present in the registry,
THEN the command SHALL exit with a non-zero status, write a descriptive error
message to stderr, and leave both files byte-unchanged.

**R-070** [feature]
WHEN `skills remove <id>` succeeds,
THEN the entry with the given `<id>` SHALL be absent from the registry.
All other entries SHALL be preserved in their original relative order.

**R-071** [feature]
`skills remove <id>` SHALL NOT delete, move, or modify any file under
`<source-root>/<id>/` on disk. The skill directory is left fully intact.

**R-072** [feature]
The registry write by `skills remove` SHALL be atomic (temp+rename), with the
same dual-temp contract as `add` (R-064, R-065): both temp files prepared before
either rename; on any temp-write error, both originals remain byte-unchanged.

**R-073** [feature]
`skills remove <id>` SHALL remove from `overlay.manifest` every line whose path
component equals `<id>/SKILL.md`. Non-matching lines are preserved verbatim.

**R-074** [feature]
After a successful `skills remove <id>`:
- `ParseRegistry` on the registry file SHALL return no error.
- The returned `Registry.Skills` SHALL NOT contain any entry with `id == "<id>"`.
- `Validate(registry, manifest)` SHALL report zero divergences
  (the id is absent from both files).

**R-075** [feature]
`skills remove` SHALL exit 0 on success and write a single confirmation line to
stdout (e.g., `removed: <id>`).

---

### CLI Dispatch

**R-076** [feature]
`SkillsCore` (in `engine/skills/skills.go`) SHALL handle verb `"add"` and verb
`"remove"`, dispatching to `AddCore` and `RemoveCore` respectively, using the
same I/O-injection pattern as existing verb cores
(`readFile`, `stdout`, `stderr`, `exit func(int)`).

**R-077** [feature]
`AddCore` and `RemoveCore` SHALL also accept a `statFile` (or equivalent) injection
for probing `<source-root>/<id>/SKILL.md` existence, so the precondition check is
testable without real filesystem paths.

**R-078** [feature]
`bin/labdrian-overlay cmd_skills` SHALL NOT be modified. The existing injection of
`--registry`, `--manifest`, and `--source-root` defaults already covers the new
verbs.

**R-079** [feature]
The files `bin/overlay`, `cmd_apply`, `cmd_capture`, and `engine/cmd/main.go`
SHALL be unchanged by this slice.

**R-080** [feature]
The error messages emitted by `SkillsCore` for an unknown or missing verb SHALL
include `add` and `remove` in the listed set of supported verbs.

---

### Red-Line Invariants

**R-081** [fix]
No new third-party (non-stdlib) Go imports SHALL be introduced. `serialize.go`
and `lifecycle.go` SHALL use only packages from the Go standard library.

**R-082** [fix]
A failed registry write (any error path in `add` or `remove`, including simulated
I/O errors) SHALL leave the registry file byte-unchanged. This is an invariant
verified by `t.TempDir`-based tests that inject write errors.

**R-083** [fix]
A failed manifest write, when it occurs before either rename executes, SHALL also
leave the registry file byte-unchanged (both files safe together per R-065/R-072).

---

## Acceptance Scenarios

### Serializer

**SC-20** — Round-trip: real 18-entry registry

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

**SC-21** — Round-trip: project-scoped entry with allowedProjects

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

**SC-22** — Determinism

```
Given:  any valid Registry reg (use the 18-entry real registry)
When:   out1, _ := Serialize(reg)
        out2, _ := Serialize(reg)
Then:   bytes.Equal(out1, out2)
```

Test: `TestSerializeDeterministic`.

**SC-23** — Upstream block absent for custom entries

```
Given:  reg with one custom entry (source.type="custom", upstream=nil)
When:   out, _ := Serialize(reg)
Then:   !bytes.Contains(out, []byte("upstream"))
```

Test: `TestSerializeNoUpstreamForCustom`.

**SC-24** — Upstream block present for core entries

```
Given:  reg with one core entry (source.type="core", upstream.owner="some-owner")
When:   out, _ := Serialize(reg)
        reg2, _ := ParseRegistry(bytes.NewReader(out))
Then:   reg2.Skills[0].Source.Upstream != nil
        reg2.Skills[0].Source.Upstream.Owner == "some-owner"
```

Test: `TestSerializeUpstreamForCore`.

**SC-25** — Golden: two-entry registry (one core, one custom)

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

### `skills add`

**SC-26** — Success path

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

**SC-27** — Missing SKILL.md

```
Given:  valid registry (no "foo" entry); skills/foo/ dir exists but SKILL.md absent
When:   AddCore([..., "foo"], ...)
Then:   exit code != 0
        stderr contains non-empty error message mentioning "foo"
        registry file is byte-unchanged (stat shows same mtime or byte comparison)
        manifest file is byte-unchanged
```

Test: `TestAddCoreMissingSkillMD`.

**SC-28** — ID already present

```
Given:  valid registry containing entry with id="foo"; skills/foo/SKILL.md exists
When:   AddCore([..., "foo"], ...)
Then:   exit code != 0
        stderr contains error message
        registry file is byte-unchanged
```

Test: `TestAddCoreIDAlreadyPresent`.

**SC-29** — Atomic safety: registry temp write fails

```
Given:  valid registry; skills/foo/SKILL.md present; registry directory made read-only
        (chmod 0555 on the dir so temp file creation fails)
When:   AddCore([..., "foo"], ...)
Then:   exit code != 0
        original registry file content unchanged
        original manifest content unchanged
```

Test: `TestAddCoreRegistryWriteFailureIsAtomic`. (Use `t.TempDir`; chmod dir before call.)

**SC-30** — Validates before write: bad entry rejected

```
Given:  a custom AddCore implementation where the entry is constructed with an
        invalid updateStrategy (inject via table test or a wrapper)
When:   the validate-before-write check runs
Then:   exit code != 0; registry unchanged
```

Test: `TestAddCoreValidateBeforeWrite`. (Can be unit-tested on the pure AddEntry helper.)

---

### `skills remove`

**SC-31** — Success path

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

**SC-32** — ID absent

```
Given:  valid registry without id="foo"
When:   RemoveCore([..., "foo"], ...)
Then:   exit code != 0
        stderr contains error message
        registry file byte-unchanged
```

Test: `TestRemoveCoreIDAbsent`.

**SC-33** — Skill dir not deleted

```
Given:  valid registry with "foo"; skills/foo/SKILL.md exists
When:   RemoveCore([..., "foo"], ...)
Then:   exit code 0
        skills/foo/SKILL.md still exists (os.Stat does not return an error)
```

Test: `TestRemoveCoreDoesNotDeleteDir`.

**SC-34** — Atomic safety: manifest temp write fails

```
Given:  valid registry with "foo"; manifest directory made read-only after
        registry temp is written (or inject manifest write error)
When:   RemoveCore([..., "foo"], ...)
Then:   exit code != 0
        (if both renames blocked by dual-temp contract) registry is byte-unchanged
```

Test: `TestRemoveCoreManifestWriteFailureIsAtomic`.

---

### Add + Remove cycle

**SC-35** — Validate stays aligned through add+remove

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

### CLI dispatch

**SC-36** — SkillsCore dispatches add and remove

```
Given:  in-memory setup with valid registry + SKILL.md present (injected via mocks)
When:   SkillsCore("add", [..., "new-id"], ...) → should not emit "unknown verb"
        SkillsCore("remove", [..., "some-id"], ...)
Then:   neither call writes "unknown skills verb" to stderr
```

Test: `TestSkillsCoreDispatchAddRemove`.

**SC-37** — Unknown verb message includes add and remove

```
Given:  SkillsCore("bogus", [], ...)
When:   called
Then:   exit code 1
        stderr contains both "add" and "remove" as supported verbs
```

Test: `TestSkillsCoreUnknownVerbMessage`.

**SC-38** — Zero stdlib-only imports (build-time assertion)

```
Given:  go list -f '{{join .Imports "\n"}}' ./engine/skills/...
Then:   no line matches a non-stdlib import path that was not present before this slice
```

Can be checked as a `go build` pass in CI or as a comment-enforced manual gate.

---

## Files Affected

| File | Status | Description |
|------|--------|-------------|
| `engine/skills/serialize.go` | New | `Serialize(Registry) ([]byte, error)` — strict-subset YAML emitter |
| `engine/skills/lifecycle.go` | New | Pure `AddEntry`/`RemoveEntry` + `AddCore`/`RemoveCore` with atomic I/O executor |
| `engine/skills/serialize_test.go` | New | Round-trip table tests + golden (SC-20–SC-25) |
| `engine/skills/lifecycle_test.go` | New | Add/remove behavior tests (SC-26–SC-37) |
| `engine/skills/skills.go` | Modified | Dispatch `add`/`remove` verbs; update unknown-verb error message |
| `skills.registry.yaml` | Data | Mutated at runtime by `add`/`remove`; not changed by the implementation |
| `bin/labdrian-overlay` | Unchanged | `cmd_skills` already injects all required defaults |
| `bin/overlay` | Unchanged | — |
| `engine/cmd/main.go` | Unchanged | — |

---

## Non-Goals (carried from proposal)

- External source fetch / git pull / pinned-ref
- Scaffolding or creating new skill file content (`add` registers existing dirs only)
- Deleting skill directories on `remove`
- Project-scope `add` flags (`allowedProjects` on the command line)
- Generating the manifest FROM the registry (Approach B)
- Preserving comments or hand-formatting after a registry write
