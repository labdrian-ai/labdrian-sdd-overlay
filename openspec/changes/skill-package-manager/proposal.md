# Proposal: Scoped Skill Package Manager — First Slice (Read-Only Semantic Layer)

## Intent

The overlay deploys skills from a flat `overlay.manifest` TSV that records only `path` + a `managed|custom` tag. There is no declarative, per-skill view of where a skill comes from, where it installs, or how it updates. Issue #29 asks to evolve the overlay into a scoped skill package manager. This slice introduces `skills.registry.yaml` as a READ-ONLY semantic layer plus `labdrian skills list/status/validate`, delivering immediate visibility and grounding the schema in real data before any manifest generation or `add`/`remove` automation is built. Zero risk to the existing deploy pipeline.

## Scope

### In Scope
- Define `skills.registry.yaml` schema (only dimensions expressible today: `id`, `source{type: core|custom, upstream?/owner?}`, `path`, `install{defaultScope: global, targets[]}`, `lifecycle{updateStrategy: vendor-merge|overlay-only}`).
- Populate `skills.registry.yaml` from current `overlay.manifest`: `managed` → `source.core` + `vendor-merge`; `custom` → `source.custom` + `overlay-only`.
- New Go package `engine/skills/`: registry.yaml parsing + `list` + `status`, dispatched via `engine skills <verb>`.
- Forward `labdrian skills <verb>` → engine from `bin/labdrian-overlay`, mirroring the `prespec` passthrough (custom router, never the vendored `bin/overlay`).
- `labdrian skills validate`: fail-loud cross-check that registry.yaml and overlay.manifest agree (skill in one but not the other = error).

### Out of Scope
- `skills add` / `skills remove` lifecycle.
- `source: external` (third-party git fetch) and `source: project`.
- `install.scope: project` and `allowedProjects` (BLOCKED — no per-repo install target exists; needs verification).
- Generating `overlay.manifest` FROM registry.yaml (Approach B big-bang).
- `updateStrategy: pinned-ref`.
- Wiring `validate` into `cmd_apply`/`cmd_capture` (deploy pipeline stays UNCHANGED this slice).

## Capabilities

> Contract with sdd-spec. Researched `openspec/specs/` — no existing specs touch skill packaging.

### New Capabilities
- `skills-registry`: the `skills.registry.yaml` file format, its field semantics (id/source/path/install/lifecycle), and the one-time population rules mapping each `overlay.manifest` row to a registry entry. READ-ONLY descriptor; never consumed by deploy.
- `skills-cli`: `labdrian skills list`, `status`, and `validate` — hosted in `engine/skills/`, dispatched by `engine skills <verb>`, forwarded from the bash router. Includes the fail-loud registry/manifest divergence cross-check.

### Modified Capabilities
- None. `cmd_apply` and `cmd_capture` deploy behavior is unchanged; the registry is purely additive.

## Approach

Approach A from exploration: registry.yaml is an additive semantic layer on top of the existing manifest, not a replacement. Manifest stays the deploy engine's ground truth. The Go engine gains a `skills` package (unit-tested, strict TDD) that parses registry.yaml and renders metadata; the bash router forwards `skills` verbs exactly like `prespec`. `validate` reconciles the two sources and fails loud on divergence so they cannot silently drift. `core` entries are treated as vendor-managed (survive `capture` + upstream merge), `custom` entries as overlay-only.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `skills.registry.yaml` | New | Read-only per-skill descriptor, populated from manifest |
| `engine/skills/` | New | Go package: parse + list/status/validate |
| `engine/cmd/main.go` | Modified | Add `skills <verb>` dispatch case |
| `bin/labdrian-overlay` | Modified | `cmd_skills` forwarder (prespec pattern) + usage entry |
| `overlay.manifest` | Modified | Add `engine/skills/*` + `skills.registry.yaml` rows |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| registry.yaml and manifest diverge | Med | `validate` fails loud; document it as the reconciliation gate |
| `core` registry entries lost on `capture`/upstream merge | Low | Treat as managed; add their files to manifest as `managed`/`custom` per ownership |
| Schema overfits today and blocks later slices | Med | Only encode dimensions expressible now; leave room for external/project/pinned-ref later |

## Rollback Plan

Delete `skills.registry.yaml`, remove the `engine/skills/` package, revert the `skills` dispatch case in `main.go`, drop `cmd_skills` from `bin/labdrian-overlay`, and remove the new `overlay.manifest` rows. The deploy pipeline is untouched, so removal has zero effect on `apply`/`capture`.

## Dependencies

- None external. Builds only on the existing engine binary + bash router (`architecture/overlay-gentle-ai-separation`).

## Assumptions

- `id` is a stable kebab-case identifier derived from each skill's directory/path (one entry per skill, not per file).
- `validate` is a STANDALONE command this slice (not wired into `apply`), to honor the "deploy pipeline unchanged" red line.
- `install.defaultScope` is fixed to `global` and `targets` enumerates the three existing TARGET_PATHS; no other scope/target values are valid yet.
- Reference/asset files under a skill dir inherit their skill's registry entry rather than getting their own.

## Success Criteria

- [ ] `skills.registry.yaml` exists with one entry per skill, faithfully derived from `overlay.manifest`.
- [ ] `labdrian skills list` and `status` render per-skill source/install/lifecycle metadata.
- [ ] `labdrian skills validate` exits non-zero when registry.yaml and manifest diverge, zero when aligned.
- [ ] `engine/skills/` has unit tests (strict TDD); deploy pipeline behavior is byte-identical to before.
- [ ] `bin/overlay` (vendored) is untouched; all new CLI lives in the router + engine.
