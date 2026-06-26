# Spec: skill-external-provenance (Issue #29, Slice 5)

**Change:** `skill-external-provenance`  
**Slice:** 5 of issue #29 — external = provenance metadata, zero fetch  
**Requirement IDs:** R-112–R-134  
**Scenario IDs:** SC-55–SC-69  
**Continues from:** slice 4 (skill-manifest-gen) — last IDs: R-111, SC-54

---

## 1. Non-regression contract

All requirements and scenarios from slices 1–4 (R-001–R-111, SC-01–SC-54) remain in
force. No existing passing test may regress.

---

## 2. Delta requirements

### 2.1 Schema — source type

**R-112** [feature] EARS: When `source.type` is `external`, the parser MUST accept
it as a valid source type alongside `core` and `custom`. The `validSourceTypes` set
MUST include `"external"`.

**R-113** [feature] EARS: When a skill entry sets `source.type: external`, the parser
MUST parse optional sub-fields `repo` (string) and `ref` (string) directly under the
`source` mapping. Unknown keys under `source` MUST still be rejected with a
line-numbered error.

**R-114** [feature] EARS: When `source.type` is `external` and no `repo` field is
present (absent or empty string after unquoting), the parser MUST return a
line-numbered error that names the entry id and states that `repo` is required for
`external` entries.

**R-115** [fix] EARS: When `source.type` is `core` or `custom` and a `repo` or `ref`
field appears under `source`, the parser MUST return a line-numbered error that names
the entry id, the offending key, and the actual source type. The error MUST be
emitted before any partial state is returned (fail-loud, mirrors the
allowedProjects-on-global guard from R-042).

**R-116** [feature] EARS: When `source.type` is `external` and `ref` is absent, the
parse MUST succeed (ref is optional / recommended, not required). The `Source.Ref`
field MUST be the empty string in that case.

**R-117** [fix] EARS: When `source.type` is `core`, the existing requirement that
`upstream.owner` be non-empty (enforced in `validateEntry`) MUST continue to hold
unchanged.

### 2.2 Serializer

**R-118** [feature] EARS: When `Serialize` processes an entry with
`source.type == "external"`, it MUST emit a `repo` line under the `source` block when
`Source.Repo` is non-empty, and a `ref` line when `Source.Ref` is non-empty. Both
lines MUST appear at indent 6 (inside `source:` at indent 4).

**R-119** [feature] EARS: When `Serialize` emits a `repo` or `ref` value, it MUST
apply the same `scalar()`/`needsQuote()` quoting rules used for all other string
scalars.

**R-120** [feature] EARS: When `checkRepresentable` validates an entry, it MUST check
`source.repo` and `source.ref` values using the existing `representable()` function.
A repo or ref value containing forbidden characters MUST cause `Serialize` to return a
non-nil error naming the entry id and the offending field.

**R-121** [feature] EARS: `parse(serialize(r)) == r` (round-trip identity) MUST hold
for any `Registry` that contains external entries with `Repo` and/or `Ref` values,
where equality is evaluated by `reflect.DeepEqual`.

### 2.3 Manifest tag mapping

**R-122** [feature] EARS: When `tagMatchesSourceType` in `validate.go` evaluates a
manifest tag against a registry source type, `source.type == "external"` MUST match
the manifest tag `"custom"`. The existing mapping (`"managed"` ↔ `"core"`,
`"custom"` ↔ `"custom"`) MUST be preserved.

**R-123** [feature] EARS: When `registryTag` in `sync.go` derives the manifest tag
for an entry, `source.type == "external"` MUST produce the tag `"custom"`. The
existing mapping (`"core"` → `"managed"`, all other → `"custom"`) MUST be preserved.

**R-124** [feature] EARS: When `Validate` is called with a registry that contains an
external entry and a manifest that tags that entry's path as `custom`, MUST report
zero divergences (exit 0).

**R-125** [feature] EARS: When `SyncManifest` processes a registry containing an
external entry, it MUST emit `<path>/SKILL.md custom` in the output manifest row for
that entry.

### 2.4 `skills add` — provenance flags

**R-126** [feature] EARS: When `parseFlags` processes `--repo <url>` in the argument
list, it MUST return the url in a new `repo` output parameter. When `--ref <sha>` is
present, it MUST return the sha in a new `ref` output parameter. Both flags are
optional and position-independent with respect to existing flags and the id argument.

**R-127** [feature] EARS: When `AddEntry` (or its replacement `AddEntryWithProvenance`)
is called with a non-empty `repo` string, the returned entry MUST have
`source.type == "external"` and `Source.Repo` set to the supplied url. When `ref` is
also supplied (non-empty), `Source.Ref` MUST be set to the supplied sha.

**R-128** [feature] EARS: When `AddEntry` or the entry-builder is called without a
`repo` argument (empty string), the returned entry MUST have `source.type == "custom"`
and `Source.Repo == ""`, preserving existing behavior. No manifest tag or downstream
behavior changes.

**R-129** [feature] EARS: When `AddCore` is invoked with `--repo`, it MUST still
enforce the existing SKILL.md-existence precondition (R-060) before any write.
`--repo` MUST NOT trigger any network I/O, `git` subprocess, or `os/exec` call.

**R-130** [feature] EARS: When `AddCore` appends the manifest line for an external
entry, it MUST append `<id>/SKILL.md custom` (not `managed`), consistent with
R-123 and R-125.

### 2.5 Zero-fetch invariant (critical)

**R-131** [fix] EARS: At all times, the `engine/skills` package MUST NOT import
`net/http`, `net`, `os/exec`, or any package whose import path contains `git`. This
invariant MUST be verifiable by scanning the output of
`go list -f '{{join .Imports "\n"}}' ./engine/skills/...` and asserting none of the
forbidden paths appear.

**R-132** [fix] EARS: The implementation of slice 5 MUST NOT add any new non-stdlib
import to `engine/skills`. The only allowed imports are those already present in the
package before this change (stdlib only — mirrors R-081 from slice 3).

### 2.6 Red lines

**R-133** [fix] EARS: The `bin/labdrian-overlay` binary MUST pass through `--repo`
and `--ref` flags to `AddCore` when they are present; all other subcommand paths
(apply, capture, validate, sync-manifest, remove, list, status) MUST remain
byte-for-byte unchanged.

**R-134** [fix] EARS: `cmd_apply` and the capture pipeline MUST NOT be modified by
this change. The overlay deploy behavior MUST be identical before and after.

---

## 3. Acceptance scenarios

### SC-55 — Parse external entry with repo and ref

```
Given: a registry YAML with one entry:
  source:
    type: external
    repo: https://github.com/example/skills
    ref: a1b2c3d

When:  ParseRegistry is called

Then:  entry.Source.Type == "external"
       entry.Source.Repo == "https://github.com/example/skills"
       entry.Source.Ref  == "a1b2c3d"
       error == nil
```

### SC-56 — Parse external entry with repo only (ref absent)

```
Given: a registry YAML with source.type: external and repo present, no ref field

When:  ParseRegistry is called

Then:  entry.Source.Repo non-empty
       entry.Source.Ref == ""
       error == nil
```

### SC-57 — Parse rejects external entry missing repo

```
Given: a registry YAML with source.type: external and no repo field

When:  ParseRegistry is called

Then:  error non-nil
       error message contains the entry id
       error message contains "repo"
       error message contains a line number
```

### SC-58 — Parse rejects repo on core entry

```
Given: a registry YAML with source.type: core and a repo field under source

When:  ParseRegistry is called

Then:  error non-nil
       error message contains the entry id
       error message contains "repo"
       error message contains the actual source type ("core")
       error message contains a line number
```

### SC-59 — Parse rejects ref on custom entry

```
Given: a registry YAML with source.type: custom and a ref field under source

When:  ParseRegistry is called

Then:  error non-nil
       error message contains the entry id
       error message contains "ref"
       error message contains the actual source type ("custom")
       error message contains a line number
```

### SC-60 — Round-trip: external entry with repo and ref

```
Given: a Registry value with one external entry where Source.Repo and Source.Ref
       are both non-empty

When:  Serialize(reg) is called, then ParseRegistry on the result

Then:  reflect.DeepEqual(reg, parsed) == true
       serialized bytes contain "repo:" under the source block
       serialized bytes contain "ref:" under the source block
```

### SC-61 — Serialize rejects repo value with forbidden chars

```
Given: a Registry with an external entry where Source.Repo contains "{"

When:  Serialize(reg) is called

Then:  error non-nil
       error message names the entry id and "source.repo"
       no bytes returned
```

### SC-62 — tagMatchesSourceType: external matches custom tag

```
Given: tag == "custom", sourceType == "external"

When:  tagMatchesSourceType(tag, sourceType) is called

Then:  result == true
```

```
Given: tag == "managed", sourceType == "external"

When:  tagMatchesSourceType(tag, sourceType) is called

Then:  result == false
```

### SC-63 — Validate: external entry aligned against custom manifest tag

```
Given: a Registry with one external entry at path "my-ext-skill"
       a manifest with row "my-ext-skill/SKILL.md custom"

When:  Diff(reg, mv) is called

Then:  []Divergence{} — no divergences
```

### SC-64 — Sync: external entry emits custom tag row

```
Given: a Registry with one external entry (path "my-ext-skill")
       an empty manifest

When:  SyncManifest(reg, manifest) is called

Then:  output contains "my-ext-skill/SKILL.md custom"
       output does NOT contain "my-ext-skill/SKILL.md managed"
       ChangeReport.Added contains "my-ext-skill"
```

### SC-65 — `skills add --repo <url>` creates external entry

```
Given: a t.TempDir() with minimal registry + manifest + skills/foo/SKILL.md

When:  AddCore(["foo", "--repo", "https://github.com/example/skills"], ...)
       is called

Then:  exit 0
       registry entry for "foo" has Source.Type == "external"
       registry entry for "foo" has Source.Repo == "https://github.com/example/skills"
       manifest has "foo/SKILL.md custom"
       Validate(reg, manifestPath) returns nil error
```

### SC-66 — `skills add --repo <url> --ref <sha>` creates external entry with ref

```
Given: t.TempDir() with minimal registry + manifest + skills/bar/SKILL.md

When:  AddCore(["bar", "--repo", "https://example.com/repo", "--ref", "deadbeef"], ...)

Then:  exit 0
       entry.Source.Ref == "deadbeef"
       round-trip: parse(serialize(newReg)).Skills[n].Source.Ref == "deadbeef"
```

### SC-67 — `skills add` without `--repo` stays custom (non-regression)

```
Given: t.TempDir() with minimal registry + manifest + skills/baz/SKILL.md

When:  AddCore(["baz"], ...) — no --repo flag

Then:  exit 0
       entry.Source.Type == "custom"
       entry.Source.Repo == ""
       manifest has "baz/SKILL.md custom"
```

### SC-68 — `skills add --repo` still enforces SKILL.md precondition

```
Given: t.TempDir() with minimal registry + manifest; skills/missing/ dir absent

When:  AddCore(["missing", "--repo", "https://example.com/repo"], ...)

Then:  exit != 0
       stderr mentions "missing"
       registry bytes unchanged
       manifest bytes unchanged
       no network I/O was performed
```

### SC-69 — Zero-fetch import scan (build-time assertion)

```
Given: the engine/skills package as modified by this slice

When:  go list -f '{{join .Imports "\n"}}' ./engine/skills/... is executed
       (or equivalent: go/packages analysis in a test)

Then:  output does NOT contain "net/http"
       output does NOT contain "net"
       output does NOT contain "os/exec"
       output does NOT contain any path containing "git"
```

> Implementation note: this scenario MUST be expressed as a `go test` function
> using `go/packages` or `runtime/debug` to enumerate imports at test time, or as
> a shell assertion in CI. The test must fail if any forbidden import is added.

---

## 4. Assumptions and decision log

| ID  | Assumption | Rationale |
|-----|------------|-----------|
| A-4 | `ref` is optional (recommended, not required) for `external` entries | Proposal states "ref recommended"; operator can vendor without a pinned ref initially |
| A-5 | external maps to manifest tag `custom` (not a new tag) | External skills are vendored locally; they behave identically to custom in install and sync paths; no manifest format change |
| A-6 | `repo` and `ref` are inert string scalars; no URL parsing or validation | Tool never fetches; URL format is for human audit only — over-validation would break offline use |
| A-7 | `--repo` and `--ref` are additive flags; `addEntry` extended (not replaced) | Preserves backward compatibility; `add <id>` without flags must still produce a custom entry |
| A-8 | `Source` struct gains `Repo string` and `Ref string` fields (zero-value for core/custom) | Zero values serialize as absent in YAML (serializer emits them only when non-empty); no wire format break |

---

## 5. Files affected

| File | Change |
|------|--------|
| `engine/skills/types.go` | Add `Repo string` and `Ref string` to `Source` struct |
| `engine/skills/parse.go` | Accept `"external"` in `validSourceTypes`; parse `repo`/`ref` in `parseSource`; validate in `validateEntry` |
| `engine/skills/serialize.go` | Emit `repo`/`ref` under `source` block; extend `checkRepresentable` |
| `engine/skills/validate.go` | Add `"external"` → `"custom"` branch in `tagMatchesSourceType` |
| `engine/skills/sync.go` | `registryTag` already returns `"custom"` for non-core; verify/document no change needed unless explicit branch required |
| `engine/skills/lifecycle.go` | Extend `parseFlags` with `--repo`/`--ref`; extend `AddEntry` or add `AddEntryWithProvenance`; update `AddCore` to pass provenance |
| `bin/labdrian-overlay` | Pass through `--repo` and `--ref` flags to `AddCore`; update usage text |
| `engine/skills/parse_test.go` | SC-55, SC-56, SC-57, SC-58, SC-59 |
| `engine/skills/serialize_test.go` | SC-60, SC-61 |
| `engine/skills/validate_test.go` | SC-62, SC-63 |
| `engine/skills/sync_test.go` | SC-64 |
| `engine/skills/lifecycle_test.go` | SC-65, SC-66, SC-67, SC-68 |
| `engine/skills/import_test.go` (new) | SC-69 — zero-fetch assertion |
