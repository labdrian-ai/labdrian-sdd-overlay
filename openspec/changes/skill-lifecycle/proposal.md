# Proposal: Mutable Skill Registry — `skills add` / `skills remove` (issue #29, slice 3)

## Intent

Slices 1–2 made `skills.registry.yaml` a strict-subset YAML that the engine can READ (parse, list, status, validate, install) but never WRITE. Registering or removing a skill still means hand-editing the file and risking a `validate` divergence. This slice makes the registry mutable from the CLI: `labdrian skills add <id>` / `remove <id>`. The crux is a new strict-subset YAML SERIALIZER that round-trips with the existing parser, so the file stays machine-managed and `validate`-clean.

## Scope

### In Scope
- `skills add <id>`: register an EXISTING `skills/<id>/` dir into the registry with inferred defaults (`source.type=custom`, `defaultScope=global`, `targets=[claude,opencode,codex]`, `updateStrategy=overlay-only`). Fail-loud if `skills/<id>/SKILL.md` is missing or the id already exists.
- `skills remove <id>`: delete the registry entry for `<id>`. Fail-loud if absent. Does NOT delete the on-disk skill dir.
- Strict-subset YAML serializer: parse → modify → serialize → re-parse is stable; emits ONLY the accepted subset (block style, no flow, no inline comments); preserves all existing entries; output is `validate`-clean.
- Crash-safe writes: write temp + atomic rename, and/or validate-before-write; never corrupt the registry on error.

### Out of Scope
- External source fetch / git pull / pinned-ref (deferred).
- Scaffolding/creating new skill CONTENT (add registers existing dirs only).
- Deleting skill directories on `remove`.
- Project-scope-specific `add` flags (allowedProjects).
- Generating the manifest FROM the registry (Approach B).

## Capabilities

### New Capabilities
None — extends the existing skills-package-manager capability.

### Modified Capabilities
- `skills-package-manager`: add WRITE behavior. New serializer requirements (round-trip stability, subset-only output, entry preservation, atomic write) and two new verbs (`add`, `remove`) with fail-loud preconditions. Continue numbering from slice 2 (R-050 onward, SC-20 onward).

## Approach

New `serialize.go` re-emits the WHOLE `Registry` from the parsed model (full re-emit — file is machine-managed; comments/formatting are not preserved). Pure `Serialize(Registry) ([]byte, error)`, table-tested round-trip + golden against the parser. New `lifecycle.go` (or extend `skills.go`) holds pure `AddEntry`/`RemoveEntry` model transforms; an I/O executor loads, mutates, re-validates, and atomically writes. `skills.go` dispatches `add`/`remove`; `bin/labdrian-overlay cmd_skills` forwards them (existing flag-default-injection for `--registry`). `bin/overlay`, `cmd_apply`, `cmd_capture`, `main.go` UNCHANGED.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `engine/skills/serialize.go` | New | Strict-subset YAML serializer (the crux) |
| `engine/skills/lifecycle.go` | New | Pure AddEntry/RemoveEntry + atomic-write executor |
| `engine/skills/skills.go` | Modified | Dispatch `add` / `remove` verbs |
| `engine/skills/*_test.go` | New | Round-trip table tests + golden |
| `bin/labdrian-overlay` | Modified | `cmd_skills` forwards add/remove |
| `skills.registry.yaml` | Data | Mutated by the new verbs at runtime |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Serializer emits non-round-trippable YAML (quoting/escaping edge cases) | Med | Round-trip table tests + golden; serializer is the highest-value TDD target |
| Partial/corrupt write on failure | Med | Atomic temp+rename; validate-before-write |
| Registry/manifest divergence after add/remove (manifest left stale) | Med | DESIGN FORK — flag for design (see below); recommend keeping both in sync |
| Inferred defaults wrong for non-custom skills | Low | Document defaults; user edits or re-adds; core/external out of scope |

## Design Forks (recommend; let sdd-design decide)

1. **Manifest sync**: does add/remove ALSO update `overlay.manifest` (inert tracking row + SKILL.md managed/custom rows), or only the registry (leaving `validate` to report divergence)? RECOMMEND: keep both in sync so `validate` stays green — flag the registry-only alternative.
2. **Serializer strategy**: full re-emit from the parsed model (simplest, drops comments/formatting) vs surgical in-place edit. RECOMMEND: full re-emit — the file is machine-managed.

## Assumptions (automatic mode — no question round)

- `<id>` is the skill directory name under `skills/` AND the registry `id` (1:1).
- `path` defaults to `<id>/` (the skill dir, relative to `skills/`).
- `add` on an existing id is an ERROR (not a silent no-op).
- Idempotency comes from fail-loud preconditions, not from overwrite.
- Hybrid persistence; artifacts in English; strict TDD (Go).

## Rollback Plan

Revert the feature commits (new files + `skills.go`/`cmd_skills` diffs). The registry is plain text under git; any bad write is recoverable via `git checkout skills.registry.yaml`. No global pipeline touched, so apply/capture are unaffected.

## Dependencies

- Slice 1 parser/validator (`parse.go`, `validate.go`) and types (`types.go`) — present.
- No new third-party deps (zero-dep engine invariant preserved).

## Success Criteria

- [ ] `skills add <id>` writes a valid entry; the file re-parses and `validate` passes.
- [ ] `skills remove <id>` removes the entry; remaining entries are byte-preserved in meaning and `validate` passes.
- [ ] `Serialize(Parse(x))` round-trips stably (parse → serialize → re-parse equal model).
- [ ] add on existing id, remove on absent id, and missing `SKILL.md` all fail loud (exit 1) without mutating the file.
- [ ] `bin/overlay`, `cmd_apply`, `cmd_capture`, `main.go` unchanged.
