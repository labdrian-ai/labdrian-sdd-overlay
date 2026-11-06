# Skill Manifest Generation Specification

## Purpose

Define the `skills sync-manifest` verb, which regenerates the skill rows of `overlay.manifest` from the registry, as issue #29 slice 4 specified it.

## Issue #29 slice 4 (`skill-manifest-gen`), synced and renumbered

Synced by Phase 9 item C11 from the delta spec of the change `skill-manifest-gen` (issue #29, slice 4), archived under `openspec/changes/archive/`, which keeps the original numbers. The text is that spec's, but for two things: every requirement and scenario ID is the original plus 500 (R-090 to R-111 are R-590 to R-611, SC-40 to SC-54 are SC-540 to SC-554), and every heading is one level deeper. Its scope, assumptions and non-goals stay in the archive. Later changes supersede parts of it; `openspec/decisions/standalone-platform-roadmap.md` (the row on Phase 8 and these four changes) records which. The section records slice 4 as it was specified, so "this slice" in it means slice 4. Its Go names are those of the slice, and the code has moved them since: `SyncManifest` returns `([]byte, ChangeReport, error)` (`engine/skills/sync.go`) behind a ports-based `app.SyncManifest` (`engine/skills/app/sync.go`), and `engine/cmd/skills_cli.go` dispatches the verb, where the section names `[]SyncEvent`, `SyncCore` and a `SkillsCore` switch in `engine/skills/skills.go`; read the requirements for the behaviour they state, not for those names. R-609 forbids reading `skills.registry.yaml`, while R-590, R-596 and R-608 read it; the command reads the registry and never writes it, so R-609 holds for writes only.

### Definitions

| Term | Definition |
|------|-----------|
| **Skill row** | A manifest line whose first whitespace-delimited field ends with `/SKILL.md` and whose leading path component is not an infra prefix (`engine/`, `_shared/`). Identical to `loadManifestViewReader` skill-row detection. |
| **Non-skill row** | Every manifest line that is not a skill row: blank lines, comment lines (`#`), infra-prefix rows (`engine/*`, `_shared/*`), asset rows (`references/*`, `evals/*`), and the `skills.registry.yaml` tracking row. |
| **Registry-derived tag** | `"managed"` when `source.type == "core"`, `"custom"` when `source.type == "custom"`. |
| **Orphan skill row** | A skill row whose directory name (first path component) matches no registry entry's `path` field. |
| **SyncEvent** | Struct `{ Kind string; Dir string }` where `Kind ∈ {"added", "dropped", "retagged"}`. |
| **Anchor** | The byte position of the first skill row in the original manifest. If no skill rows exist, the anchor is the end of file (append mode). |

---

### Requirements

#### 3.1 Pure Partition and Regeneration

**R-590** [new-capability]  
WHEN `SyncManifest(reg Registry, manifestBytes []byte)` is called,  
THEN it SHALL return `(outBytes []byte, events []SyncEvent)` as a pure function with no I/O side effects.

**R-591** [new-capability]  
The regenerated skill block SHALL contain exactly one line per registry entry, in registry order.  
The line format SHALL be `<entry.path>/SKILL.md <tag>\n` where `<tag>` is the registry-derived tag
(`"managed"` ↔ `source.type == "core"`, `"custom"` ↔ `source.type == "custom"`).

**R-592** [new-capability]  
Row ordering rule — SyncManifest SHALL apply the anchor-replacement algorithm:

1. Walk the input lines sequentially.
2. Emit each non-skill line verbatim.
3. At the first skill row encountered (the anchor), emit the entire regenerated skill block.
4. Skip all subsequent original skill rows (both registry-matched and orphan).
5. If no skill rows exist in the input, append the skill block after all non-skill lines.

Non-skill rows preserve their original relative order. Non-skill rows that appeared between skill
rows in the original are emitted after the skill block.

**R-593** [new-capability]  
The output of SyncManifest SHALL end with exactly one newline (`\n`) when the registry is non-empty,
matching the EOF convention enforced by the rest of the lifecycle commands.

#### 3.2 Orphan Handling

**R-594** [new-capability]  
An orphan skill row (no matching registry `path`) SHALL be dropped from the output — not re-emitted.
Registry state is the single source of truth for which `*/SKILL.md` rows belong in the manifest.

**R-595** [new-capability]  
For every orphan dropped, SyncManifest SHALL append `SyncEvent{Kind: "dropped", Dir: <dir>}` to its
return slice.

**R-596** [new-capability]  
For every registry entry whose directory was absent from the original manifest skill rows, SyncManifest
SHALL append `SyncEvent{Kind: "added", Dir: <entry.path>}`.

**R-597** [new-capability]  
For every manifest skill row whose tag disagreed with the registry-derived tag — including TAG_MISMATCH
rows (`HasConflict == false` and `me.Tag != registry-derived-tag`) and MIXED_TAG rows
(`HasConflict == true`, any tag distribution) — SyncManifest SHALL append
`SyncEvent{Kind: "retagged", Dir: <dir>}`.

#### 3.3 Post-condition Invariant

**R-598** [new-capability]  
For all inputs `(reg, manifestBytes)`, the output of `SyncManifest` SHALL satisfy:  
`Diff(reg, loadManifestViewReader(outBytes)) == []` (empty divergence slice).  
This is the write-side inverse of `skills validate`.

**R-599** [new-capability]  
SyncCore SHALL verify R-598 before calling `writeFileAtomic`. If `Diff` returns any divergence after
regeneration, SyncCore SHALL print each divergence to stderr and exit 1 without touching
`overlay.manifest`.

#### 3.4 Idempotency

**R-600** [new-capability]  
`SyncManifest(reg, SyncManifest(reg, m)[0])` SHALL return bytes identical to `SyncManifest(reg, m)[0]`
for any `reg` and `m`. Equivalently: a second sync on an already-synced manifest is a byte-identical
no-op with an empty SyncEvents slice.

#### 3.5 Atomic and Safe Writes

**R-601** [new-capability]  
SyncCore SHALL write `overlay.manifest` using the `writeFileAtomic` + `os.Rename` pattern (matching
AddCore/RemoveCore). The temp file is created in the same directory as `overlay.manifest`.

**R-602** [new-capability]  
If any I/O step fails after the temp file is created but before `os.Rename` completes, SyncCore SHALL
call `os.Remove` on the temp file and leave the original `overlay.manifest` byte-unchanged.

**R-603** [new-capability]  
SyncCore SHALL perform validate-before-write on the regenerated bytes: call `loadManifestViewReader`
on the in-memory output, then `Diff(reg, mv)`. If this cross-check returns any divergence or a parse
error, SyncCore SHALL exit 1 without writing. This reuses the identical guard in AddCore/RemoveCore
(ADR-9 step 7).

#### 3.6 CLI Dispatch

**R-604** [feature]  
`SkillsCore` SHALL dispatch verb `"sync-manifest"` to `SyncCore` in the existing `switch` statement
in `engine/skills/skills.go`. No other file is modified for dispatch.

**R-605** [feature]  
The error messages for empty-verb and unknown-verb branches in `SkillsCore` SHALL include
`"sync-manifest"` in the listed supported verbs.

**R-606** [feature]  
`labdrian skills sync-manifest` (wrapper binary) SHALL route to the engine binary with argument
`skills sync-manifest`, which dispatches via `SkillsCore("sync-manifest", ...)`.

**R-607** [feature]  
SyncCore SHALL exit 0 on success and write a summary line to stdout that names all three counters:
added, dropped, and retagged. Example: `"sync-manifest: 2 added, 1 dropped, 0 retagged\n"`.

**R-608** [feature]  
SyncCore SHALL exit non-zero (fail-loud) on any of: registry read failure, manifest read failure,
registry parse error, manifest parse error, post-condition Diff failure, or file write failure.
Each failure SHALL print a descriptive message to stderr.

#### 3.7 Red-Line Invariants

**R-609** [fix]  
SyncCore SHALL write ONLY `overlay.manifest`. It SHALL NOT read or write `skills.registry.yaml`,
SHALL NOT call `AddEntry` or `RemoveEntry`, and SHALL NOT call `Serialize`.

**R-610** [fix]  
The vendored `bin/overlay` binary SHALL NOT be referenced, executed, or modified by the
`sync-manifest` command, its supporting functions, or its tests.

**R-611** [fix]  
`cmd_apply` and `cmd_capture` behavior SHALL be unchanged. SyncCore SHALL have no `init()`
registration, no global variable mutations, and no changes to apply/capture code paths.

---

### Acceptance Scenarios

#### SC-540 — Aligned manifest: byte-identical output (idempotency base case)

**Given**:
- Registry: `[{id:"alpha", path:"alpha", source.type:"custom"}, {id:"beta", path:"beta", source.type:"core"}]`
- Manifest bytes: `"alpha/SKILL.md custom\nbeta/SKILL.md managed\n"`

**When**: `SyncManifest(reg, manifestBytes)` is called

**Then**:
- Returned bytes equal input bytes exactly
- `SyncEvents` slice is empty (len == 0)

---

#### SC-541 — TAG_MISMATCH: resolved to registry-derived tag

**Given**:
- Registry: `[{id:"alpha", source.type:"custom"}]`
- Manifest: `"alpha/SKILL.md managed\n"` (wrong tag — registry says custom)

**When**: `SyncManifest(reg, manifestBytes)` is called

**Then**:
- Output = `"alpha/SKILL.md custom\n"`
- `SyncEvents` = `[{Kind:"retagged", Dir:"alpha"}]`

---

#### SC-542 — MIXED_TAG: single correct row emitted, retag event

**Given**:
- Registry: `[{id:"alpha", source.type:"core"}]`
- Manifest: `"alpha/SKILL.md managed\nalpha/SKILL.md custom\n"` (two rows, conflicting tags)

**When**: `SyncManifest(reg, manifestBytes)` is called

**Then**:
- Output contains exactly one row `"alpha/SKILL.md managed\n"` (registry-derived: core→managed)
- `SyncEvents` = `[{Kind:"retagged", Dir:"alpha"}]`

---

#### SC-543 — Orphan skill row dropped, event emitted

**Given**:
- Registry: `[{id:"alpha", source.type:"custom"}]`
- Manifest: `"alpha/SKILL.md custom\norphan/SKILL.md custom\n"`

**When**: `SyncManifest(reg, manifestBytes)` is called

**Then**:
- Output = `"alpha/SKILL.md custom\n"` (`orphan` row absent)
- `SyncEvents` = `[{Kind:"dropped", Dir:"orphan"}]`

---

#### SC-544 — Missing registry entry: added at anchor, event emitted

**Given**:
- Registry: `[{id:"alpha", source.type:"custom"}, {id:"beta", source.type:"core"}]`
- Manifest: `"alpha/SKILL.md custom\n"` (beta absent)

**When**: `SyncManifest(reg, manifestBytes)` is called

**Then**:
- Output = `"alpha/SKILL.md custom\nbeta/SKILL.md managed\n"` (beta appended to skill block)
- `SyncEvents` = `[{Kind:"added", Dir:"beta"}]`

---

#### SC-545 — Non-skill rows preserved verbatim, in original relative order

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

#### SC-546 — Non-skill rows interspersed in skill zone: anchor model applied

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

#### SC-547 — No existing skill rows: skill block appended at end

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

#### SC-548 — Post-condition invariant holds for all outputs (property test)

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

#### SC-549 — Atomic write: I/O failure leaves manifest byte-unchanged

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

#### SC-550 — SyncCore writes only overlay.manifest (red-line)

**Given**: a valid registry and aligned manifest; SyncCore called with a spy `readFile`

**When**: `SyncCore` succeeds (exit 0)

**Then**:
- `writeFileAtomic` is called exactly once, targeting the `overlay.manifest` path
- `skills.registry.yaml` is never written or renamed
- `Serialize` is never called (asserted via compilation: `sync.go` MUST NOT import or call `Serialize`)

---

#### SC-551 — SkillsCore dispatches "sync-manifest" to SyncCore (unit)

**Given**: valid registry + manifest bytes in `t.TempDir()`

**When**: `SkillsCore("sync-manifest", args, readFile, stdout, stderr, exitFn)` is called

**Then**:
- `exitFn` called with 0
- `stderr` is empty
- `stdout` contains the summary line with "added", "dropped", "retagged" counts

---

#### SC-552 — SkillsCore error messages include "sync-manifest" in supported verb list

**Given**: N/A

**When**:
1. `SkillsCore("", args, ...)` — empty verb
2. `SkillsCore("unknown-verb", args, ...)` — unrecognized verb

**Then** (both cases):
- `exitFn` called with 1
- `stderr` output contains the string `"sync-manifest"`

---

#### SC-553 — Idempotency: second sync is byte-identical, zero events

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

#### SC-554 — Table-driven: source.type → manifest tag mapping (pure unit)

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
