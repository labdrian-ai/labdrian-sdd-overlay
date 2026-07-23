# Proposal: Restore Skill Registry Scoped Blocks

## Intent

Restore the healthy hook-status signal by regenerating three missing, generated scope blocks in `.atl/skill-registry.md`. The repair must use the existing authoritative propagator, preserve unrelated registry content, and leave already-healthy runtime surfaces unchanged.

## Scope

### In Scope
- Invoke the existing propagator for `minimalism-contract`, `skill-discovery-safety`, and `anti-generic-design` against `.atl/skill-registry.md`.
- Restore exactly one marker-delimited block and generated row for each corresponding scope.
- Verify the registry-only diff and confirm `bin/labdrian-overlay status-hooks` exits `0`.

### Out of Scope
- Binary rebuilds; hook, contract, propagator, status-check, or TUI code/UX changes.
- Runtime synchronization or changes to Claude, OpenCode, or Codex targets.
- `upstream..main` investigation, unrelated registry cleanup, or remote delivery.

## Capabilities

### New Capabilities
None. This is generated-state restoration through existing behavior.

### Modified Capabilities
None. No specification-level requirement changes are proposed.

## Approach

Capture the pre-repair registry state, invoke the existing propagator separately for the three authoritative shared contracts, and inspect the resulting marker ranges and full diff. Do not hand-author generated rows or run broader install/sync workflows. Finish with the existing `status-hooks` health check.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `.atl/skill-registry.md` | Modified | Restore only the three generated scope blocks and rows. |
| `skills/_shared/{minimalism-contract,skill-discovery-safety,anti-generic-design}.md` | Read only | Authoritative propagation inputs. |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Broader workflow mutates healthy targets | Low | Use only the narrow propagator path. |
| Duplicate or unrelated registry changes | Low | Verify one marker pair per scope and inspect the complete diff. |

## Rollback Plan

Before rollback, compare the current registry bytes/hash with this transaction's last known written snapshot. Restore the captured pre-repair content only on equality; on mismatch, preserve current bytes, stop, and report manual recovery. Never overwrite blindly. No binary, hook, contract, runtime, or code rollback should be necessary.

## Dependencies

- Current engine propagator and the three authoritative shared contracts.
- Existing `bin/labdrian-overlay status-hooks` command.

## Success Criteria

- [ ] Each named scope block and generated row occurs exactly once.
- [ ] Content outside the three generated marker ranges is unchanged.
- [ ] `bin/labdrian-overlay status-hooks` exits `0` without missing-scope warnings.
- [ ] No excluded surface is modified and no remote delivery occurs.
