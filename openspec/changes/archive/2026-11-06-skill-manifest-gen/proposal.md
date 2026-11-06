# Proposal: skill-manifest-gen (Issue #29, slice 4)

## Intent

Make `skills.registry.yaml` the source of truth for the SKILL.md rows of `overlay.manifest`, incrementally. Today both files are hand-kept in sync and `validate` only *detects* drift (TAG_MISMATCH, MIXED_TAG, MISSING_IN_REGISTRY) — there is no write-side inverse to *fix* it. This is Approach B's end-state delivered as a safe, additive `skills sync-manifest` command, not a big-bang rewrite. The deploy pipeline (`cmd_apply`) keeps reading `overlay.manifest` byte-for-byte as today.

## Scope

### In Scope
- New `labdrian skills sync-manifest` verb: regenerate `*/SKILL.md` rows from the registry — one `<id>/SKILL.md <tag>` per entry, `tag = managed` if `source.type==core` else `custom`.
- Preserve every NON-`*/SKILL.md` row verbatim: inert `engine/*` rows, `_shared/*`, `references/`, `assets/`, `evals/`, and the `skills.registry.yaml` tracking row.
- Idempotent (second run = no-op on aligned manifest); resolves TAG_MISMATCH/MIXED_TAG to registry tag; drops orphan SKILL.md rows (MISSING_IN_REGISTRY).
- Atomic temp+rename, validate-before-write, failed op leaves `overlay.manifest` byte-unchanged.
- Print a summary of rows added/dropped/retagged (not silent).
- Post-condition: `skills validate` reports the registry/manifest classes (`MISSING_IN_MANIFEST`, `MISSING_IN_REGISTRY`, `TAG_MISMATCH`, `MIXED_TAG`) aligned. `sync-manifest` regenerates only `*/SKILL.md` rows, so it cannot resolve an on-disk divergence (`UNREGISTERED_ON_DISK`/`MISSING_ON_DISK`) against a reference or `_shared/*` file.

### Out of Scope
- External git sources, pinned-ref, vendoring.
- Removing `overlay.manifest` or changing how `cmd_apply` reads it.
- Generating NON-skill rows from anything (preserved verbatim).
- Touching vendored `bin/overlay`; global `cmd_apply`/capture unchanged.

## Capabilities

### New Capabilities
- `skill-manifest-sync`: regenerate manifest SKILL.md rows from the registry, preserving all other rows, atomically and idempotently, as the write-side inverse of `validate`.

### Modified Capabilities
- None (sync is additive; `validate` behavior is unchanged and reused as the post-condition check).

## Approach

Reuse existing primitives: `ParseRegistry`, `loadManifestViewReader`, `Diff`, `writeFileAtomic`. Add a pure `SyncManifest(reg, manifestBytes) []byte` that partitions lines into (skill-rows, other-rows), regenerates skill-rows deterministically from the registry, and re-emits other-rows verbatim in place. Wire `SyncCore` mirroring `AddCore`/`RemoveCore`: cross-check `Diff(reg, regeneratedView)` must be empty before writing.

### Design forks (for sdd-design)
1. **Row ordering**: keep non-skill rows in place; regenerate skill rows at the position of their first occurrence; deterministic order for new rows. (Recommend.)
2. **Orphan SKILL.md rows (MISSING_IN_REGISTRY)**: drop them (registry is truth) but print each drop. Confirm vs. fail-loud-first.
3. **Orphan `references/`/`assets/`/`evals/` rows of a dropped skill**: preserved verbatim per the non-skill rule — flag as known residue.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `engine/skills/sync.go` | New | Pure `SyncManifest` + `SyncCore` |
| `engine/skills/skills.go` | Modified | Add `sync-manifest` verb dispatch |
| `engine/skills/sync_test.go` | New | TDD coverage |
| `overlay.manifest` | Runtime-rewritten | Only by the command, never at build |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Manifest corruption | Low | Atomic temp+rename, validate-before-write, byte-unchanged on failure |
| Silent data loss (dropped rows) | Med | Print every add/drop/retag; idempotency test |
| Ordering churn vs. apply | Low | Preserve first-occurrence position; `cmd_apply` unaffected |

## Rollback Plan

Revert the PR. `sync.go`/`sync_test.go` are new files; the `skills.go` dispatch line is the only edit. No data migration; `overlay.manifest` is only rewritten when the command runs, so simply do not run it (or `git checkout overlay.manifest`).

## Dependencies

- Merged slices 1–3 (parser, serializer, manifest reader, Diff, atomic write). Strict TDD (Go).

## Success Criteria

- [ ] `skills sync-manifest` makes a drifted manifest pass `skills validate`'s registry/manifest checks (it does not, and cannot, resolve on-disk divergences for reference or `_shared/*` files).
- [ ] Running it twice is a byte-identical no-op.
- [ ] Non-SKILL.md rows are preserved verbatim.
- [ ] Failed op leaves `overlay.manifest` byte-unchanged.
- [ ] Added/dropped/retagged rows are printed.
