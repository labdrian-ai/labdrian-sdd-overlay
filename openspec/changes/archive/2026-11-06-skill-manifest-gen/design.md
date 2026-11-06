# Design: skill-manifest-gen (sync-manifest)

## Technical Approach

Add the write-side inverse of `validate`: a `skills sync-manifest` verb that regenerates the `*/SKILL.md` rows of `overlay.manifest` from `skills.registry.yaml`, preserving every other row verbatim. Split into a PURE core `SyncManifest(reg, manifestBytes) -> (newText, ChangeReport, error)` (table-testable, includes a self-check) and an I/O shell `SyncCore` that mirrors `AddCore`/`RemoveCore` exactly: read → pure transform → validate-before-write → atomic temp+rename. Reuses `ParseRegistry`, `loadManifestViewReader`, `Diff`, `writeFileAtomic`. Only `overlay.manifest` is written (registry untouched), so it is a single-file variant of the dual-write Add/Remove flow.

## Architecture Decisions

| Decision | Choice | Rejected | Rationale |
|---|---|---|---|
| Row ordering / preservation | Partition lines: `preservedBefore` + `skillBlock` + `preservedAfter`. Drop ALL existing `*/SKILL.md` rows; re-emit the full registry-ordered skill block ONCE at the first-skill-row anchor. Non-skill lines (comments, blanks, `engine/*`, asset rows, `skills.registry.yaml`) keep exact bytes and relative order. | Regenerate each skill row in place (fails when registry adds/removes entries); strip+append block at EOF (moves rows under the tracking row). | Single insertion point is deterministic AND idempotent: run-2's first skill row is the block's first line, so the partition recomputes identically → byte-identical. |
| No-anchor case (zero skill rows) | Append `skillBlock` after `preservedBefore` (i.e. at EOF). | Special-case before tracking row. | Edge case only; simplest deterministic rule. |
| Pure vs shell | Pure `SyncManifest` does regen + self-check and returns `error`; shell does only I/O + atomic write. | Self-check in shell. | Keeps the safety guarantee table-testable; a buggy regen can NEVER reach disk. |
| Byte no-op | If `newText == originalBytes`, skip the write (print "already in sync"). | Always rewrite identical bytes. | Avoids mtime churn; satisfies "2nd run = byte no-op" literally. |
| Tag rule | `source.type=="core" → managed`, else `custom`. Emit `<entry.Path>/SKILL.md <tag>`. | — | Inverse of `tagMatchesSourceType`; keyed by `entry.Path` so `Diff` matches. |
| Orphans (MISSING_IN_REGISTRY) | Drop the skill row (not re-emitted), report it. Orphan `references/assets/evals` rows stay verbatim (non-skill rule) — known residue, out of scope to clean. | Fail loud. | Registry is source of truth; loud-but-additive beats hard failure. |
| MIXED_TAG / TAG_MISMATCH | Collapse to one row with the registry tag; report `retagged`. | — | Single emitted row removes the conflict; `Diff` then empty. |

## Data Flow

    skills.registry.yaml ─ParseRegistry─→ Registry ┐
                                                    ├─ SyncManifest (pure)
    overlay.manifest ─readFile─→ []byte ────────────┘   │ partition + regen + self-check
                                                        ▼
                              newText + ChangeReport ──→ writeFileAtomic → os.Rename
                                                        └─ report → stdout

Self-check (inside pure core, the load-bearing guard): `Diff(reg, loadManifestViewReader(newText))` MUST be empty, else return error and write nothing — the manifest stays byte-unchanged.

## File Changes

| File | Action | Description |
|---|---|---|
| `engine/skills/sync.go` | Create | `ChangeReport`, pure `SyncManifest`, `SyncCore` I/O shell, `isSkillRow` helper (mirrors `loadManifestViewReader` classification). |
| `engine/skills/skills.go` | Modify | Add `case "sync-manifest": SyncCore(stripVerb(args,"sync-manifest"), readFile, stdout, stderr, exit)`; extend verb lists in `""`/default branches. |
| `engine/skills/sync_test.go` | Create | TDD coverage (see below). |
| `overlay.manifest` | Modify (manual, build-time) | Add infra rows `engine/skills/sync.go managed` and `engine/skills/sync_test.go managed` (mirrors existing `engine/*_test.go` convention). These are infra rows, untouched by sync itself. |
| `bin/labdrian-overlay` | Modify | Add `sync-manifest  regenerate SKILL.md rows from registry (idempotent)` usage line. |
| `engine/cmd/main.go` | Unchanged | `runSkillsCore` forwards verb+args; help strings already stale (omit install/add/remove) — consistent, no edit. |

## Interfaces / Contracts

```go
type ChangeReport struct {
    Added    []string // dir names newly emitted (MISSING_IN_MANIFEST)
    Dropped  []string // orphan skill dirs removed (MISSING_IN_REGISTRY)
    Retagged []string // dirs whose tag changed (TAG_MISMATCH/MIXED_TAG)
}

// SyncManifest regenerates */SKILL.md rows from reg, preserving all other lines
// byte-for-byte. Returns the new manifest text, a change report, and a non-nil
// error if the post-condition Diff(reg, view(newText)) is non-empty.
func SyncManifest(reg Registry, manifest []byte) ([]byte, ChangeReport, error)
```

`isSkillRow(line) (dir string, ok bool)` MUST match `loadManifestViewReader`: trim, skip blank/`#`, ≥2 fields, `fields[0]` ends `/SKILL.md`, has `/`, first component not in `infraPrefixes`.

## Testing Strategy

| Layer | What | Approach |
|---|---|---|
| Unit (pure) | aligned no-op, retag, MIXED_TAG collapse, orphan drop, add missing, comments/blanks/asset rows preserved, no-anchor append, trailing-newline present/absent | Table-driven (`go-testing`). |
| Idempotency | `sync(sync(x)) == sync(x)` byte-identical; also against the real `overlay.manifest` | Dedicated test seam on pure `SyncManifest`. |
| Shell | drifted manifest → passes `validate` after sync; read/parse error leaves file byte-unchanged; report printed | `t.TempDir()`, inject `readFile`/`exit`. |

## Migration / Rollout

No migration. `overlay.manifest` is rewritten only when the command runs; `cmd_apply` reads it unchanged. Rollback = revert the PR.

## Open Questions

- [ ] None blocking. Orphan asset-row residue is accepted out-of-scope per proposal.
