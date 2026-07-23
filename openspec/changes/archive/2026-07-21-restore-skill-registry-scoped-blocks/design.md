# Design: Restore Skill Registry Scoped Blocks

## Technical Approach

Capture the registry baseline and persistent lock identity, propagate once per scope, verify acceptance, repeat for idempotence, retain the sidecar, and run the read-only health check. No source-code design or behavior changes are permitted.

## Architecture Decisions

| Option | Tradeoff | Decision |
|---|---|---|
| Hand-edit generated rows | Simple but bypasses contract parsing and marker ownership | Reject; use `gentle-ai-overlay propagate` exclusively. |
| Broad overlay install/sync/refresh | May regenerate healthy runtime targets or the whole registry | Reject; invoke only three narrow propagations. |
| One repair transaction with a registry baseline and persistent lock identity | Adds evidence steps but supports guarded rollback | Adopt; stop on any failed command or invariant. |
| Existing tests plus transaction checks | No production tests are added; generated state gets RED/GREEN evidence | Adopt for strict TDD. |

## Data Flow

```text
contracts/assets -> propagator -> registry + persistent lock sidecar
baseline ---------------------> preservation and guarded rollback oracle
registry -> uniqueness -> second pass -> sidecar identity check -> status-hooks
```

## File Changes

| File | Action | Description |
|---|---|---|
| `.atl/skill-registry.md` | Regenerate | Add only the three owned marker blocks and rows. |
| `.atl/skill-registry.md.lock` | Retain | Treat it as persistent lock infrastructure and preserve its path/inode once created. |
| All source, binaries, hooks, contracts, TUI, and runtime targets | None | Read-only or untouched. |

## Interfaces / Contracts

Run from the verified root returned by `git rev-parse --show-toplevel`; require it to equal `/home/labdrian/labdrian-sdd-overlay`. Require the already-installed `$HOME/.claude/bin/gentle-ai-overlay` to be executable and record its SHA-256; do not rebuild it:

```sh
ENGINE="$HOME/.claude/bin/gentle-ai-overlay"
REGISTRY=".atl/skill-registry.md"
LOCK="$REGISTRY.lock"
"$ENGINE" propagate --registry "$REGISTRY" --contract-file skills/_shared/minimalism-contract.md --contract-path skills/_shared/minimalism-contract.md --require-registry
"$ENGINE" propagate --registry "$REGISTRY" --embedded-contract skill-discovery-safety --contract-path skills/_shared/skill-discovery-safety.md --require-registry
"$ENGINE" propagate --registry "$REGISTRY" --embedded-contract anti-generic-design --contract-path skills/_shared/anti-generic-design.md --require-registry
```

The embedded modes provide distinct marker/row identities for the latter two scopes. Each exact BEGIN marker, END marker, and first-cell row label (`minimalism-contract`, `skill-discovery-safety`, `anti-generic-design`) must occur once, with each row inside its matching pair. Because `acquireRegistryLock` opens `REGISTRY.lock` with `O_CREATE|O_RDWR`, the sidecar is persistent lock infrastructure: once created, reuse its path/inode lifecycle. Flock release unlocks and closes only; never unlink, rename, or replace the sidecar to recreate baseline absence.

## Testing Strategy

| Stage | Evidence | Pass boundary |
|---|---|---|
| Baseline/RED | Resolve the expected root. Directly `lstat` both ignored paths. Copy the required registry byte-for-byte and record SHA-256/mode. Record lock existence and, when available, inode/hash/mode; if initially absent, capture its identity after first creation. Capture direct hash/mode manifests for named excluded surfaces. Run uniqueness assertions. | Assertions fail only because the three scopes are absent; malformed/duplicate state blocks mutation. Git status is not evidence for either ignored `.atl` path. |
| GREEN | Run the three commands separately with fail-fast exit handling. | Every command exits 0; exact marker/row uniqueness and containment pass. |
| Preservation | An ephemeral verifier removes only the three complete generated ranges, including insertion delimiters, then byte-compares with baseline; malformed/nested/unmatched ranges fail. Compare excluded-surface manifests directly. | Outside bytes and registry mode are unchanged; no excluded surface changed. |
| Idempotence | Snapshot repaired registry bytes/hash/mode, rerun all three commands, then compare directly. | Registry bytes/hash/mode and uniqueness are unchanged. |
| Sidecar finalization | After all propagations exit and flock is released, retain the sidecar and directly `lstat` its path/inode. | The sidecar remains present at the same path/inode recorded after creation; no unlink, rename, or replacement occurs. |
| Health | Run `bin/labdrian-overlay status-hooks`, capturing stdout, stderr, and exit code. | Exit 0 and no missing-scope warning for any restored scope. |

Existing evidence is in `engine/propagator/propagator_test.go` and `engine/cmd/main_test.go`; no source tests are added.

## Threat Matrix

| Boundary | Applicability | Safe/failure behavior | Planned RED tests |
|---|---|---|---|
| Documentation-like paths | N/A — no executable-file classification | No classification occurs. | None |
| Git repository selection | Applicable | Operate only when the resolved root is the expected absolute path; mismatch fails before snapshot or mutation. | Run the root guard from a wrong cwd/path and require failure with an unchanged registry. |
| Commit state | N/A — no staging or commit command | Index is untouched. | None |
| Push state | N/A — push is prohibited | No remote mutation. | None |
| PR commands | N/A — PR commands are prohibited | No remote mutation. | None |

## Migration / Rollout

No migration required. On any failure after mutation, first compare current registry bytes/hash with this transaction's last known written snapshot. Only on equality may the captured registry bytes/mode be restored atomically; on mismatch, preserve current bytes, stop, and report manual recovery without overwriting. Keep the lock sidecar at its persistent path/inode, verify excluded-surface manifests are unchanged, and do not run install, sync, refresh, rebuild, restart, TUI, Git push, PR, or any remote action.

## Open Questions

None.
