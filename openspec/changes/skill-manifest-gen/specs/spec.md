# Spec: `skills sync-manifest` (Issue #29, Slice 4)

## Scope

Delta spec for `skill-manifest-gen` slice 4. R-NNN numbering continues from slice 3 (R-089):
requirements **R-090–R-111**, scenarios **SC-40–SC-54**. This spec covers:

1. **Partition + regen** — pure `SyncManifest` partitions manifest lines, regenerates `*/SKILL.md`
   rows from the registry, preserves every non-skill row verbatim.
2. **Orphan handling** — drops rows whose directory is absent from the registry; prints every
   add/drop/retag event.
3. **Post-condition invariant** — `Diff(reg, regeneratedManifest)` is empty after every sync.
4. **Idempotency** — a second sync on an already-synced manifest is a byte-identical no-op.
5. **Atomic + safe write** — validate-before-write, `writeFileAtomic` + `os.Rename`, original
   untouched on any failure.
6. **CLI dispatch** — `SkillsCore` + `labdrian skills sync-manifest` passthrough.
7. **Red-line invariants** — only `overlay.manifest` is written; registry, vendored binary,
   and apply/capture paths are untouched.

---

## Definitions

| Term | Definition |
|------|-----------|
| **Skill row** | A manifest line whose first whitespace-delimited field ends with `/SKILL.md` and whose leading path component is not an infra prefix (`engine/`, `_shared/`). Identical to `loadManifestViewReader` skill-row detection. |
| **Non-skill row** | Every manifest line that is not a skill row: blank lines, comment lines (`#`), infra-prefix rows (`engine/*`, `_shared/*`), asset rows (`references/*`, `evals/*`), and the `skills.registry.yaml` tracking row. |
| **Registry-derived tag** | `"managed"` when `source.type == "core"`, `"custom"` when `source.type == "custom"`. |
| **Orphan skill row** | A skill row whose directory name (first path component) matches no registry entry's `path` field. |
| **SyncEvent** | Struct `{ Kind string; Dir string }` where `Kind ∈ {"added", "dropped", "retagged"}`. |
| **Anchor** | The byte position of the first skill row in the original manifest. If no skill rows exist, the anchor is the end of file (append mode). |

---

## Assumptions

| ID    | Assumption |
|-------|-----------|
| A-090 | The `SyncManifest` return type is `([]byte, []SyncEvent)` — pure, no I/O, matching the established AddEntry/RemoveEntry pure-function pattern. |
| A-091 | Ordering rule: the pseudocode "emit non-skill lines verbatim; at first skill row emit the full regen block; skip all subsequent original skill rows" is the canonical form. Non-skill rows that appeared between skill rows in the original are emitted after the skill block (their relative order w.r.t. each other is preserved). |
| A-092 | MIXED_TAG rows (HasConflict=true) always produce a "retagged" SyncEvent, because the original state was ambiguous and the output resolves it unambiguously. |
| A-093 | SyncCore signature follows AddCore/RemoveCore: `SyncCore(args []string, readFile readFileFn, stdout, stderr io.Writer, exit func(int))`. |

---

## Requirements

### 3.1 Pure Partition and Regeneration

**R-090** [new-capability]  
WHEN `SyncManifest(reg Registry, manifestBytes []byte)` is called,  
THEN it SHALL return `(outBytes []byte, events []SyncEvent)` as a pure function with no I/O side effects.

**R-091** [new-capability]  
The regenerated skill block SHALL contain exactly one line per registry entry, in registry order.  
The line format SHALL be `<entry.path>/SKILL.md <tag>\n` where `<tag>` is the registry-derived tag
(`"managed"` ↔ `source.type == "core"`, `"custom"` ↔ `source.type == "custom"`).

**R-092** [new-capability]  
Row ordering rule — SyncManifest SHALL apply the anchor-replacement algorithm:

1. Walk the input lines sequentially.
2. Emit each non-skill line verbatim.
3. At the first skill row encountered (the anchor), emit the entire regenerated skill block.
4. Skip all subsequent original skill rows (both registry-matched and orphan).
5. If no skill rows exist in the input, append the skill block after all non-skill lines.

Non-skill rows preserve their original relative order. Non-skill rows that appeared between skill
rows in the original are emitted after the skill block.

**R-093** [new-capability]  
The output of SyncManifest SHALL end with exactly one newline (`\n`) when the registry is non-empty,
matching the EOF convention enforced by the rest of the lifecycle commands.

### 3.2 Orphan Handling

**R-094** [new-capability]  
An orphan skill row (no matching registry `path`) SHALL be dropped from the output — not re-emitted.
Registry state is the single source of truth for which `*/SKILL.md` rows belong in the manifest.

**R-095** [new-capability]  
For every orphan dropped, SyncManifest SHALL append `SyncEvent{Kind: "dropped", Dir: <dir>}` to its
return slice.

**R-096** [new-capability]  
For every registry entry whose directory was absent from the original manifest skill rows, SyncManifest
SHALL append `SyncEvent{Kind: "added", Dir: <entry.path>}`.

**R-097** [new-capability]  
For every manifest skill row whose tag disagreed with the registry-derived tag — including TAG_MISMATCH
rows (`HasConflict == false` and `me.Tag != registry-derived-tag`) and MIXED_TAG rows
(`HasConflict == true`, any tag distribution) — SyncManifest SHALL append
`SyncEvent{Kind: "retagged", Dir: <dir>}`.

### 3.3 Post-condition Invariant

**R-098** [new-capability]  
For all inputs `(reg, manifestBytes)`, the output of `SyncManifest` SHALL satisfy:  
`Diff(reg, loadManifestViewReader(outBytes)) == []` (empty divergence slice).  
This is the write-side inverse of `skills validate`.

**R-099** [new-capability]  
SyncCore SHALL verify R-098 before calling `writeFileAtomic`. If `Diff` returns any divergence after
regeneration, SyncCore SHALL print each divergence to stderr and exit 1 without touching
`overlay.manifest`.

### 3.4 Idempotency

**R-100** [new-capability]  
`SyncManifest(reg, SyncManifest(reg, m)[0])` SHALL return bytes identical to `SyncManifest(reg, m)[0]`
for any `reg` and `m`. Equivalently: a second sync on an already-synced manifest is a byte-identical
no-op with an empty SyncEvents slice.

### 3.5 Atomic and Safe Writes

**R-101** [new-capability]  
SyncCore SHALL write `overlay.manifest` using the `writeFileAtomic` + `os.Rename` pattern (matching
AddCore/RemoveCore). The temp file is created in the same directory as `overlay.manifest`.

**R-102** [new-capability]  
If any I/O step fails after the temp file is created but before `os.Rename` completes, SyncCore SHALL
call `os.Remove` on the temp file and leave the original `overlay.manifest` byte-unchanged.

**R-103** [new-capability]  
SyncCore SHALL perform validate-before-write on the regenerated bytes: call `loadManifestViewReader`
on the in-memory output, then `Diff(reg, mv)`. If this cross-check returns any divergence or a parse
error, SyncCore SHALL exit 1 without writing. This reuses the identical guard in AddCore/RemoveCore
(ADR-9 step 7).

### 3.6 CLI Dispatch

**R-104** [feature]  
`SkillsCore` SHALL dispatch verb `"sync-manifest"` to `SyncCore` in the existing `switch` statement
in `engine/skills/skills.go`. No other file is modified for dispatch.

**R-105** [feature]  
The error messages for empty-verb and unknown-verb branches in `SkillsCore` SHALL include
`"sync-manifest"` in the listed supported verbs.

**R-106** [feature]  
`labdrian skills sync-manifest` (wrapper binary) SHALL route to the engine binary with argument
`skills sync-manifest`, which dispatches via `SkillsCore("sync-manifest", ...)`.

**R-107** [feature]  
SyncCore SHALL exit 0 on success and write a summary line to stdout that names all three counters:
added, dropped, and retagged. Example: `"sync-manifest: 2 added, 1 dropped, 0 retagged\n"`.

**R-108** [feature]  
SyncCore SHALL exit non-zero (fail-loud) on any of: registry read failure, manifest read failure,
registry parse error, manifest parse error, post-condition Diff failure, or file write failure.
Each failure SHALL print a descriptive message to stderr.

### 3.7 Red-Line Invariants

**R-109** [fix]  
SyncCore SHALL write ONLY `overlay.manifest`. It SHALL NOT read or write `skills.registry.yaml`,
SHALL NOT call `AddEntry` or `RemoveEntry`, and SHALL NOT call `Serialize`.

**R-110** [fix]  
The vendored `bin/overlay` binary SHALL NOT be referenced, executed, or modified by the
`sync-manifest` command, its supporting functions, or its tests.

**R-111** [fix]  
`cmd_apply` and `cmd_capture` behavior SHALL be unchanged. SyncCore SHALL have no `init()`
registration, no global variable mutations, and no changes to apply/capture code paths.

---

## Acceptance Scenarios

### SC-40 — Aligned manifest: byte-identical output (idempotency base case)

**Given**:
- Registry: `[{id:"alpha", path:"alpha", source.type:"custom"}, {id:"beta", path:"beta", source.type:"core"}]`
- Manifest bytes: `"alpha/SKILL.md custom\nbeta/SKILL.md managed\n"`

**When**: `SyncManifest(reg, manifestBytes)` is called

**Then**:
- Returned bytes equal input bytes exactly
- `SyncEvents` slice is empty (len == 0)

---

### SC-41 — TAG_MISMATCH: resolved to registry-derived tag

**Given**:
- Registry: `[{id:"alpha", source.type:"custom"}]`
- Manifest: `"alpha/SKILL.md managed\n"` (wrong tag — registry says custom)

**When**: `SyncManifest(reg, manifestBytes)` is called

**Then**:
- Output = `"alpha/SKILL.md custom\n"`
- `SyncEvents` = `[{Kind:"retagged", Dir:"alpha"}]`

---

### SC-42 — MIXED_TAG: single correct row emitted, retag event

**Given**:
- Registry: `[{id:"alpha", source.type:"core"}]`
- Manifest: `"alpha/SKILL.md managed\nalpha/SKILL.md custom\n"` (two rows, conflicting tags)

**When**: `SyncManifest(reg, manifestBytes)` is called

**Then**:
- Output contains exactly one row `"alpha/SKILL.md managed\n"` (registry-derived: core→managed)
- `SyncEvents` = `[{Kind:"retagged", Dir:"alpha"}]`

---

### SC-43 — Orphan skill row dropped, event emitted

**Given**:
- Registry: `[{id:"alpha", source.type:"custom"}]`
- Manifest: `"alpha/SKILL.md custom\norphan/SKILL.md custom\n"`

**When**: `SyncManifest(reg, manifestBytes)` is called

**Then**:
- Output = `"alpha/SKILL.md custom\n"` (`orphan` row absent)
- `SyncEvents` = `[{Kind:"dropped", Dir:"orphan"}]`

---

### SC-44 — Missing registry entry: added at anchor, event emitted

**Given**:
- Registry: `[{id:"alpha", source.type:"custom"}, {id:"beta", source.type:"core"}]`
- Manifest: `"alpha/SKILL.md custom\n"` (beta absent)

**When**: `SyncManifest(reg, manifestBytes)` is called

**Then**:
- Output = `"alpha/SKILL.md custom\nbeta/SKILL.md managed\n"` (beta appended to skill block)
- `SyncEvents` = `[{Kind:"added", Dir:"beta"}]`

---

### SC-45 — Non-skill rows preserved verbatim, in original relative order

**Given**:
- Registry: `[{id:"alpha", source.type:"custom"}]`
- Manifest (4 lines):
  ```
  engine/install.sh managed
  alpha/SKILL.md custom
  _shared/common.sh managed
  skills.registry.yaml custom
  ```

**When**: `SyncManifest(reg, manifestBytes)` is called

**Then**:
- Output equals input exactly (all non-skill rows in place, skill block at anchor, no events)

---

### SC-46 — Non-skill rows interspersed in skill zone: anchor model applied

**Given**:
- Registry: `[{id:"alpha", source.type:"custom"}, {id:"beta", source.type:"custom"}]`
- Manifest (5 lines):
  ```
  engine/a.sh managed
  alpha/SKILL.md custom
  engine/b.sh managed
  beta/SKILL.md custom
  skills.registry.yaml custom
  ```

**When**: `SyncManifest(reg, manifestBytes)` is called

**Then**:
- Output (5 lines):
  ```
  engine/a.sh managed
  alpha/SKILL.md custom
  beta/SKILL.md custom
  engine/b.sh managed
  skills.registry.yaml custom
  ```
- Skill block emitted at anchor (line 2), replacing `alpha` and `beta` skill rows.
- `engine/b.sh managed` (was between skill rows) moves to after the skill block.
- `SyncEvents` is empty (both skills were already present with correct tags).

---

### SC-47 — No existing skill rows: skill block appended at end

**Given**:
- Registry: `[{id:"alpha", source.type:"custom"}]`
- Manifest (2 non-skill lines):
  ```
  engine/install.sh managed
  skills.registry.yaml custom
  ```

**When**: `SyncManifest(reg, manifestBytes)` is called

**Then**:
- Output (3 lines):
  ```
  engine/install.sh managed
  skills.registry.yaml custom
  alpha/SKILL.md custom
  ```
- `SyncEvents` = `[{Kind:"added", Dir:"alpha"}]`

---

### SC-48 — Post-condition invariant holds for all outputs (property test)

**Given**: any registry `reg` and any manifest bytes `m` (including drifted, empty-skill, and
mixed-tag manifests)

**When**:
```go
out, _ := SyncManifest(reg, m)
mv, _ := loadManifestViewReader(bytes.NewReader(out))
divs := Diff(reg, mv)
```

**Then**: `len(divs) == 0`

_Test pattern_: table-driven with at least: aligned, TAG_MISMATCH, orphan, missing entry, MIXED_TAG,
empty manifest, no-skill manifest.

---

### SC-49 — Atomic write: I/O failure leaves manifest byte-unchanged

**Given**:
- A manifest file written to `t.TempDir()`
- A drifted registry (so sync would write a new manifest)
- `writeFileAtomic` fails after temp creation (simulated via write-error injection)

**When**: `SyncCore(args, readFile, stdout, stderr, exit)` is called

**Then**:
- Original manifest file bytes are byte-unchanged
- `exit` is called with code 1
- stderr contains a descriptive error message
- No temp file remains in the directory

_Note: integration test; mark `testing.Short()` skip when real filesystem ops cannot be injected._

---

### SC-50 — SyncCore writes only overlay.manifest (red-line)

**Given**: a valid registry and aligned manifest; SyncCore called with a spy `readFile`

**When**: `SyncCore` succeeds (exit 0)

**Then**:
- `writeFileAtomic` is called exactly once, targeting the `overlay.manifest` path
- `skills.registry.yaml` is never written or renamed
- `Serialize` is never called (asserted via compilation: `sync.go` MUST NOT import or call `Serialize`)

---

### SC-51 — SkillsCore dispatches "sync-manifest" to SyncCore (unit)

**Given**: valid registry + manifest bytes in `t.TempDir()`

**When**: `SkillsCore("sync-manifest", args, readFile, stdout, stderr, exitFn)` is called

**Then**:
- `exitFn` called with 0
- `stderr` is empty
- `stdout` contains the summary line with "added", "dropped", "retagged" counts

---

### SC-52 — SkillsCore error messages include "sync-manifest" in supported verb list

**Given**: N/A

**When**:
1. `SkillsCore("", args, ...)` — empty verb
2. `SkillsCore("unknown-verb", args, ...)` — unrecognized verb

**Then** (both cases):
- `exitFn` called with 1
- `stderr` output contains the string `"sync-manifest"`

---

### SC-53 — Idempotency: second sync is byte-identical, zero events

**Given**: any registry `reg`; any manifest bytes `m` (possibly drifted)

**When**:
```go
m2, _ := SyncManifest(reg, m)
m3, events2 := SyncManifest(reg, m2)
```

**Then**:
- `bytes.Equal(m2, m3) == true`
- `len(events2) == 0`

_Test pattern_: table-driven over aligned, TAG_MISMATCH, orphan, and missing-entry inputs.

---

### SC-54 — Table-driven: source.type → manifest tag mapping (pure unit)

| `source.type` | Expected tag in output |
|---|---|
| `"core"` | `"managed"` |
| `"custom"` | `"custom"` |

**Given**: for each row, a registry entry with the given `source.type` and a manifest with the wrong
tag for that entry

**When**: `SyncManifest(reg, manifestBytes)` is called

**Then**:
- Output line uses the expected tag from the table
- `SyncEvents` contains exactly `{Kind:"retagged", Dir:<dir>}`

---

## Test File Contract

Per go-testing skill (STRICT TDD):

- **`engine/skills/sync_test.go`** — table-driven unit tests for `SyncManifest` (SC-40–SC-48,
  SC-50, SC-53, SC-54); `t.TempDir()` for file-write scenarios; no real filesystem paths.
- **`engine/skills/skills_test.go`** — `SkillsCore` dispatch tests for SC-51, SC-52 (extend
  existing file following the established `exitFn` injection pattern).
- SC-49 (atomic write failure) uses `t.TempDir()` and error injection; tagged with
  `testing.Short()` skip only if the scenario requires real OS rename behavior not injectable.
- No test reads `overlay.manifest` or `skills.registry.yaml` from the repo root.
