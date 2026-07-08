# Proposal: Overlay Coherence Mini-Fix

## Intent

Resolve coherence gaps in wrapper commands, hook status semantics, GADU portable wording, and docs. Keep the engine as control-plane, preserve `bin/overlay` as untouched baseline, and make read-only checks fail loud instead of mutating state.

## Scope

### In Scope
- Add `bin/labdrian-overlay gadu-generate [--check]` forwarding to the engine with correct `OVERLAY_DIR` handling.
- Make `status-hooks` truly read-only by failing clearly when the engine binary is absent; keep build/deploy behavior only in `install-hooks`.
- Remove runtime-specific “You run on Claude” wording from `engine/gadu/persona/body.md` and regenerate all generated GADU artifacts: `agents/GADU.md`, `opencode/agents/GADU.md`, `skills/gadu-operator/SKILL.md`.
- Update README and directly related OpenSpec docs/tasks to describe three generated artifacts and wrapper behavior accurately.
- Add focused shell/Go tests where testable; keep validation module-scoped.

### Out of Scope
- Editing `bin/overlay` vendored/baseline file.
- Changing runtime lifecycle behavior unrelated to this coherence issue.
- Creating commits, pushing, or opening PRs.

## Approach

Use the wrapper as a thin command bridge: dispatch `gadu-generate` to the engine binary with `OVERLAY_DIR` set to the repo root. Tighten `status-hooks` to report a missing binary with actionable `install-hooks` guidance and perform no build. Update canonical GADU body, regenerate outputs, and keep docs focused on command behavior, the generated artifact list, and the read-only/mutating hook split.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `bin/labdrian-overlay` | Modified | Add wrapper command and read-only `status-hooks` failure path. |
| `engine/gadu/persona/body.md` | Modified | Runtime-neutral canonical persona body. |
| `agents/GADU.md`, `opencode/agents/GADU.md`, `skills/gadu-operator/SKILL.md` | Modified | Regenerated from canonical source. |
| `README.md`, related OpenSpec artifacts | Modified | Correct command and artifact-count docs. |
| `engine/gadu/*`, wrapper checks | Modified | Focused tests/checks for generator and wrapper semantics. |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Scripts relied on implicit `status-hooks` build | Medium | Clear error points to `overlay install-hooks`. |
| Generated persona wording changes unexpectedly | Low | Regenerate from one canonical source and run `gadu-generate --check`. |
| Docs drift from command behavior again | Low | Update docs with implementation and tests in same work unit. |

## Rollback Plan

Revert the wrapper changes, canonical GADU body edit, regenerated artifacts, tests, and documentation updates as one work unit. `install-hooks` remains the explicit recovery path for any missing engine binary.

## Dependencies

- Existing Go engine `gadu-generate [--check]` command.
- Existing overlay manifest routes for generated GADU artifacts.

## Success Criteria

- [ ] `bin/labdrian-overlay gadu-generate --check` reaches the engine with the repo root as `OVERLAY_DIR`.
- [ ] `status-hooks` does not build or deploy the engine binary when it is missing.
- [ ] All three generated GADU artifacts match runtime-neutral canonical body output.
- [ ] README and related OpenSpec docs describe the wrapper command and three artifacts accurately.
- [ ] Module-scoped validation passes for touched surfaces.
