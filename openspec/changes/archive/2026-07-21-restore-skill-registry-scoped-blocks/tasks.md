# Tasks: Restore Skill Registry Scoped Blocks

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated changed lines | 120–220 |
| 800-line budget risk | Low |
| Chained PRs recommended | Yes — forced |
| Suggested split | PR 1 atomic repair → PR 2 independent verification receipt |
| Delivery strategy | force-chained |
| Chain strategy | stacked-to-main |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: Low
800-line budget risk: Low

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| 1 | One indivisible repair transaction | PR 1 → main | transaction assertions | `bin/labdrian-overlay status-hooks` | guarded registry rollback and persistent `.lock` identity |
| 2 | Independently review recorded acceptance evidence | PR 2 → main (after PR 1) | direct manifest/byte comparisons | `status-hooks` captured result | verification receipt only; no runtime mutation |

## Phase 1: RED Baseline and Guards

- [x] 1.1 From `/home/labdrian/labdrian-sdd-overlay`, verify the Git-root guard; RED-test a wrong cwd/path and prove `.atl/skill-registry.md` is unchanged.
- [x] 1.2 Directly `lstat`, byte-copy, SHA-256, and mode-capture `.atl/skill-registry.md`; record lock existence/inode and capture its identity after first creation when initially absent; snapshot named excluded surfaces and engine SHA-256.
- [x] 1.3 RED-run `bin/labdrian-overlay status-hooks` for missing-scope/nonzero evidence; test exact markers, rows, and containment, failing before mutation on malformed/duplicate ranges.

## Phase 2: Atomic GREEN Restoration

- [x] 2.1 In one fail-fast transaction, run only the three designed `$HOME/.claude/bin/gentle-ai-overlay propagate --require-registry` commands against `.atl/skill-registry.md`; do not install, sync, rebuild, restart, use TUI, or contact remotes.
- [x] 2.2 GREEN-assert exactly one matching marker pair and first-cell row for `minimalism-contract`, `skill-discovery-safety`, and `anti-generic-design`, with every row inside its pair.
- [x] 2.3 Remove only the three complete generated ranges in an ephemeral verifier and byte-compare the remainder and mode with the `.atl/skill-registry.md` baseline; directly compare excluded-surface manifests.

## Phase 3: Idempotence, Finalization, and Health

- [x] 3.1 Snapshot repaired registry bytes/hash/mode; rerun the same three commands and assert no registry diff, duplicate marker, or duplicate row.
- [x] 3.2 **Historical and superseded/non-reusable:** After flock release, the archived run removed `.atl/skill-registry.md.lock` to restore its initially absent baseline, then proved it absent.
- [x] 3.3 Run `bin/labdrian-overlay status-hooks`, capture stdout/stderr/exit, and require exit `0` with no restored-scope missing warning.

## Phase 4: Rollback and Review

- [x] 4.1 **Historical and superseded/non-reusable:** On the first-attempt verifier failure, the archived run restored captured registry bytes/hash/mode and the absent lock baseline, then verified rollback and all 96 excluded entries. It did not compare for foreign writes before restore.
- [x] 4.2 Review direct filesystem evidence: only `.atl/skill-registry.md` is intentionally regenerated, unrelated content is preserved, and no excluded surface or remote action occurred.

## Post-review safety correction (not executed in this archived run)

Future operations MUST retain the persistent `.atl/skill-registry.md.lock` path/inode after flock release and compare current registry bytes/hash with the transaction's last known written snapshot before restoring. A mismatch MUST preserve foreign writes and require manual recovery. This normative guidance is not a checked task and does not claim historical execution. This section is future-only normative guidance, is excluded from the checked historical task count, and does not retroactively change completed execution evidence.
