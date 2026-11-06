# Tasks: skill-manifest-gen (Issue #29, Slice 4 — `skills sync-manifest`)

Spec: R-090–R-111 / SC-40–SC-54  
Design: pure SyncManifest + SyncCore I/O shell mirroring AddCore/RemoveCore  
Branch: `skill-project-scope/pr-2-executor` → `main`  
Delivery: single PR (feature-branch-chain)

---

## Dependency Graph

```
T-01 → T-02 → T-03 → T-04 ─┬─ T-05 (parallel)
                             ├─ T-06 (parallel)
                             └─ T-07 (parallel)
```

---

## Tasks

### T-01 — Write failing tests for `SyncManifest` pure function
**Sequential. Must pass red before T-02.**  
File: `engine/skills/sync_test.go` (create)  
Satisfies: R-090, R-092, R-093, R-094, R-095, R-096, R-097, R-098, R-099, R-100

Write a table-driven `TestSyncManifest` function covering all pure-function scenarios. The file must compile but every case must fail (SyncManifest does not exist yet).

Test cases (one table entry per scenario):

| Scenario | Input state | Expected output |
|---|---|---|
| SC-40 aligned no-op | Registry matches manifest exactly | byte-identical output, ChangeReport all-empty |
| SC-41 TAG_MISMATCH | Entry present but tag wrong (e.g. custom in manifest, core in registry) | tag corrected, `Retagged` event |
| SC-42 MIXED_TAG | Same dir has two SKILL.md rows with different tags | collapsed to one row with registry tag, `Retagged` event |
| SC-43 orphan drop | Manifest row has no matching registry entry | row absent from output, `Dropped` event |
| SC-44 missing entry | Registry entry absent from manifest | row inserted at skill-block anchor, `Added` event |
| SC-45 non-skill rows verbatim | Manifest has comments, blank lines, engine/* rows | all preserved byte-for-byte |
| SC-46 interspersed non-skill | Non-skill rows between skill rows | non-skill rows move to preservedBefore/After; skill block emitted once at anchor |
| SC-47 no-anchor (zero skill rows) | Manifest has only non-skill rows | skill block appended at EOF |
| SC-48 post-condition invariant | Any mutation | `Diff(reg, loadManifestViewReader(out))` must return empty slice |
| SC-53 idempotency | out1 = Sync(reg, in); out2 = Sync(reg, out1) | bytes.Equal(out1, out2) |
| SC-54 tag mapping table | source.type=core → "managed"; source.type=custom → "custom" | correct tag in output row |

Additional assertions for every case:
- Output ends with exactly one `\n`
- No error returned unless post-condition fails

---

### T-02 — Implement `SyncManifest`, `ChangeReport`, `isSkillRow`
**Sequential. After T-01 red.**  
File: `engine/skills/sync.go` (create)  
Satisfies: R-090, R-091, R-092, R-093, R-094, R-095, R-096, R-097, R-098, R-099, R-100

```go
type ChangeReport struct {
    Added    []string
    Dropped  []string
    Retagged []string
}

func SyncManifest(reg Registry, manifest []byte) ([]byte, ChangeReport, error)
```

Implementation notes (all from design):

- **isSkillRow(line string) bool**: trim space; skip blank and `#`-prefix; split fields; needs `>=2` fields; `fields[0]` must end with `/SKILL.md` and contain `/`; first path component (before first `/`) must NOT match `isInfraDir`. This must match `loadManifestViewReader`'s classification exactly.
- **Partition algorithm**: single pass over lines — collect `preservedBefore` (all lines before first skill row) and `preservedAfter` (all non-skill lines after the first skill row); skip all existing skill rows; emit: `preservedBefore + skillBlock + preservedAfter`.
  - No-anchor case (zero skill rows): `preservedBefore == all lines`, `skillBlock` appended at EOF.
- **skillBlock**: one `<entry.Path>/SKILL.md <tag>\n` per registry entry, in registry order. Tag rule: `source.type == "core"` → `managed`, else `custom`.
- **Byte no-op**: if `newText == manifest` byte-for-byte, return `(manifest, ChangeReport{}, nil)` — no write-back needed (caller can detect no-op; SyncCore will skip atomic write).
- **ChangeReport events**: compare registry entries vs original manifest view (via `loadManifestViewReader`) to classify Added/Dropped/Retagged.
- **Post-condition self-check**: after building `newText`, call `loadManifestViewReader(bytes.NewReader(newText))` + `Diff(reg, mv)`; if non-empty, return `(nil, ChangeReport{}, fmt.Errorf("sync: post-condition failed: %v", divs))`.
- **Trailing newline**: `newText` must end with exactly one `\n`. Handle `manifest` ending with or without `\n`.

Run `go test ./engine/skills/... -run TestSyncManifest` until all cases pass.

---

### T-03 — Write failing tests for `SyncCore` I/O shell
**Sequential. After T-02 green.**  
File: `engine/skills/sync_test.go` (append)  
Satisfies: R-101, R-102, R-103, R-106, R-107

Write `TestSyncCore` covering:

| Sub-test | Setup | Assertion |
|---|---|---|
| SC-48 integration: drifted manifest → validate passes | Write real registry+drifted manifest to `t.TempDir()`; run SyncCore; then Validate | `Validate(reg, manifestPath)` returns nil error |
| SC-49 atomic write failure leaves file unchanged | Read-only manifest file (chmod 0444); run SyncCore | manifest bytes unchanged, exit 1, stderr non-empty |
| report printed | Valid drifted manifest | stdout contains count of Added/Dropped/Retagged or "already in sync" |
| already-in-sync no-write | Aligned manifest | exit 0, stdout contains "already in sync", manifest mtime unchanged |
| --registry/--manifest flag parsing | Custom flag paths | correct files used |

---

### T-04 — Implement `SyncCore` I/O shell
**Sequential. After T-03 red.**  
File: `engine/skills/sync.go` (append)  
Satisfies: R-101, R-102, R-103, R-104, R-105, R-106, R-107, R-109

```go
func SyncCore(args []string, readFile readFileFn, stdout, stderr io.Writer, exit func(int))
```

Implementation notes:

1. Parse `--registry` / `--manifest` flags (reuse `parseFlags` for the first two fields, or inline a minimal two-flag parser — YAGNI, minimalism-contract rung 4).
2. Read registry via `readFile(registryPath)` → `ParseRegistry`.
3. Read manifest via `readFile(manifestPath)`.
4. Call `SyncManifest(reg, manifestBytes)` → `(newText, report, err)`.
5. If error → `fmt.Fprintln(stderr, ...)`, `exit(1)`, return.
6. If `newText == manifestBytes` (byte-identical / no-op from SyncManifest): print `"overlay.manifest already in sync\n"` to stdout, `exit(0)`, return.
7. Validate-before-write: `loadManifestViewReader(bytes.NewReader(newText))` + `Diff` — if non-empty (should never be after SyncManifest's self-check, but belt-and-suspenders): stderr + exit 1.
8. Atomic write: `writeFileAtomic(manifestPath, newText)` → `tmpName`; on error remove tmp + exit 1.
9. `os.Rename(tmpName, manifestPath)`; on error remove tmp + exit 1.
10. Print summary: `fmt.Fprintf(stdout, "sync-manifest: added=%d dropped=%d retagged=%d\n", len(report.Added), len(report.Dropped), len(report.Retagged))`.
11. `exit(0)`.

Red lines: never reads or writes `registryPath` with write-mode; `readFile` is read-only.

Run `go test ./engine/skills/... -run TestSyncCore` until all cases pass.

---

### T-05 — Wire `sync-manifest` dispatch in `skills.go`
**Parallel with T-06 and T-07. After T-04 green.**  
File: `engine/skills/skills.go` (modify)  
File: `engine/skills/skills_test.go` (append)  
Satisfies: R-104, R-105, R-106, R-108 (SC-51, SC-52)

**skills.go changes** (3 lines):

```go
case "sync-manifest":
    SyncCore(stripVerb(args, "sync-manifest"), readFile, stdout, stderr, exit)
```

Extend the `""` branch error string and the `default` branch error string to include `sync-manifest` in the supported verbs list.

**skills_test.go additions** — append `TestSkillsCoreDispatchSyncManifest`:

- SC-51: `SkillsCore("sync-manifest", ...)` routes to SyncCore without emitting "unknown skills verb". Use a `t.TempDir()` with aligned registry+manifest so SyncCore exits 0.
- SC-52: Unknown verb `"bogus-after-sync"` → stderr contains `"sync-manifest"`.

---

### T-06 — Add `sync-manifest` to usage in `bin/labdrian-overlay`
**Parallel with T-05 and T-07. After T-04 green.**  
File: `bin/labdrian-overlay` (modify)  
Satisfies: R-108

In the `usage()` function, inside the `skills <verb>` block, add after the `remove <id>` line:

```
                                     sync-manifest  regenerate */SKILL.md rows from skills.registry.yaml
```

Match the indentation and column alignment of the existing verb lines.

---

### T-07 — Add `sync.go` and `sync_test.go` managed rows to `overlay.manifest`
**Parallel with T-05 and T-06. After T-04 green (or as early as T-02 merges).**  
File: `overlay.manifest` (modify)  
Satisfies: design build-time requirement (keeps `validate` aligned after implementation)

Insert after line 17 (`engine/skills/skills.go managed`):

```
engine/skills/sync.go managed
engine/skills/sync_test.go managed
```

These are inert engine rows. They mirror the pattern of the existing `engine/skills/*.go managed` block. `sync-manifest` does NOT touch infra rows — they are safe from accidental regeneration.

---

## Commit Plan (work-unit commits, feature-branch-chain)

Each commit ships a green test+code pair or a pure-text change:

| Commit | Files touched | Status |
|---|---|---|
| `test(skills): add failing SyncManifest pure-function tests (T-01)` | sync_test.go | red |
| `feat(skills): implement SyncManifest + ChangeReport + isSkillRow (T-02)` | sync.go | green |
| `test(skills): add failing SyncCore I/O shell tests (T-03)` | sync_test.go | red |
| `feat(skills): implement SyncCore atomic I/O shell (T-04)` | sync.go | green |
| `feat(skills): wire sync-manifest dispatch in SkillsCore + tests (T-05)` | skills.go, skills_test.go | green |
| `feat(overlay): add sync-manifest to labdrian-overlay usage (T-06)` | bin/labdrian-overlay | — |
| `chore(manifest): register sync.go + sync_test.go as managed rows (T-07)` | overlay.manifest | — |

---

## Review Workload Forecast

- `engine/skills/sync_test.go` (new): ~275 lines
- `engine/skills/sync.go` (new): ~170 lines
- `engine/skills/skills_test.go` (append): ~30 lines
- `engine/skills/skills.go` (modify): ~4 lines
- `bin/labdrian-overlay` (modify): ~2 lines
- `overlay.manifest` (modify): ~2 lines

**Estimated changed lines: ~483**  
**400-line budget risk: Medium**  
**Chained PRs recommended: No**  
**Decision needed before apply: No**

Rationale: all new lines land in exactly two new files (sync.go, sync_test.go). The diff is cohesive and self-contained; reviewer can follow T-01→T-04 as a pure TDD progression. A single PR is the right shape here.
