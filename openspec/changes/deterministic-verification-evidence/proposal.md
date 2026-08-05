# Proposal: Deterministic Verification Evidence

## Intent

The overlay narrates LLM judgments where machine-checkable facts exist: `skills/sdd-verify/SKILL.md:79` says non-zero test exits are CRITICAL while `strict-tdd-verify.md:121/:127/:266` downgrades linter/typecheck findings to WARNING, and `gentle-ai review capture-evidence` — the authoritative evidence rail — has zero call sites in this repo. After this change a red deterministic check produces FAIL and evidence is reproducible machine output on the existing rail, instead of PASS WITH WARNINGS backed by prose.

## Scope

### In Scope
- Severity policy: determinism decides blocking membership (R-002); red deterministic checks CRITICAL (R-003); coverage/deadcode stay WARNING (R-004).
- Go runner under `tools/`, hardcoded v1 check set (`gofmt`, `go vet`, `staticcheck` blocking; `deadcode` WARNING) emitting `tool | exit_code | summary` rows: counts + top-5 + path, zero renders `0`, never raw dumps or banned literals (R-005..R-008).
- `normalize` (mutating, pre-`review start`) vs `check` (byte-neutral, post-freeze) subcommands; ordering declared in `sdd-verify` (R-009..R-011).
- `bin/labdrian-overlay` dispatch seam; `sdd-verify` pipes stdout into `capture-evidence --input -`; outcomes `passed`/`verification_failed`/`procedural_tooling_failed`; missing tool or missing CLI never blocks — tooling failure takes precedence (R-012..R-016).
- CI: `staticcheck` blocking, `deadcode` informational (R-017/R-018); manifest rows for new `skills/` files (R-019); `tools` testing layer in `openspec/config.yaml`.

### Out of Scope
- Browser-mode visual runner — split to a future change; destination repo is a pending user decision; visual artifacts structurally cannot enter the review contract.
- Config-driven check set; shellcheck over `bin/labdrian-overlay`; knip; ts-prune; Lighthouse CI.
- Any upstream `gentle-ai` modification (external binary v2.2.4).

## Capabilities

### New Capabilities
- `deterministic-verification-policy`: determinism-based severity across sdd-verify and CI (R-002..R-004, R-017, R-018).
- `deterministic-check-runner`: `tools/` runner, row contract, bounded summaries, mode separation (R-005..R-010).
- `verification-evidence-capture`: CLI seam, capture-evidence wiring, outcome selection, ordering declaration, manifest registration (R-011..R-016, R-019).

### Modified Capabilities
None.

## Approach

Four validated chained slices (feature-branch-chain, 800-line budget): `severity-policy-and-ci-gates` → `deterministic-check-runner-core` → `runner-mode-separation` → `capture-evidence-wiring`. Slice 1 lands the stricter policy first so all Go code is authored under it. Runner mirrors `tools/entry-contract-validator`; CLI guards mirror `cmd_validate_entry_contract`'s fail-loud exit-3 pattern. No `project-architect` amendment required.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `skills/sdd-verify/{strict-tdd-verify.md,SKILL.md}` | Modified | Severity, ordering, wiring |
| `tools/<runner>/` | New | Go module |
| `bin/labdrian-overlay` | Modified | Dispatch + presence guards |
| `.github/workflows/ci.yml` | Modified | Gates + runner job |
| `openspec/config.yaml`, `overlay.manifest` | Modified | `tools` layer; conditional rows |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Slice 2 (est. 500-780) exceeds 800-line budget | Med | Re-forecast pre-apply; pre-agreed split seam |
| Flaky/unavailable check blocks → permanent dead end (obs #2668) | Low | R-002 exclusion + R-016 precedence |
| Assumption: native review never consumes `strict-tdd-verify.md` (documented, not source-verified) | Low | Named assumption; chain ordering isolates slice 1 |

## Rollback Plan

Slices revert independently: markdown/YAML revert (1); delete `tools/` module and CI job (2-3); revert dispatch and skill wiring (4). No data migration; upstream bytes untouched (R-001).

## Dependencies

- External `gentle-ai` v2.2.4 rail (free-form string, max 4194304 bytes); `staticcheck`/`deadcode` provisioning in CI.

## Success Criteria

- [ ] Red `go vet`/`gofmt`/`staticcheck` yields FAIL, not PASS WITH WARNINGS.
- [ ] Deterministic evidence block reproducible byte-for-byte on rerun.
- [ ] `capture-evidence` gains a real call site; payload never contains banned literals.
- [ ] No run stranded in `review/escalate-correction-verification` by a flaky or missing tool.
