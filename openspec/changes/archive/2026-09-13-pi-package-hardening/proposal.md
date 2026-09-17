# Proposal: Pi Package Hardening and GADU as a Real Pi Subagent

## Intent

Four load-bearing debts share one pipeline: `pipkg` misses mode drift, allows registry `path` traversal, and never checks SKILL.md `name` (#312); `sync-check --target pi` compares a `main`-built package against the current checkout (#315); review receipts are burned by `acknowledge-approved` before closure-feedback reads them, so three changes archived with self-asserted anchors (#316); and GADU is unreachable from Pi/gentle-pi. GADU's own asset is built, verified, and archived through exactly this machinery, so the owner confirmed resolving all four in one change (R-001..R-016), sliced at `sdd-tasks`.

## Scope

### In Scope
- pipkg integrity: mode-aware `Check`, 0755 build root, path containment in `validateEntry`, SKILL.md `name`==directory (R-001..R-004).
- Build provenance: `labdrian.builtFrom` in `package.json`; `sync-check --target pi` compares against that ref's tree, disclosing a `main` fallback (R-005..R-007).
- Receipt capture: persist `gentle-ai.review-receipt/v2` to `openspec/changes/<change>/review-receipts/<lineage>.json` before acknowledgement; `archive-anchor-gate` and closure-feedback read `final_candidate_tree` from it; archive blocks without a receipt unless an explicit owner override is recorded (R-008..R-011).
- GADU subagent: install `pi-subagents-j0k3r` when neither recognized package is present; link generated `agents/GADU.md` to `~/.pi/agent/agents/GADU.md` as overlay-owned; frontmatter verified; distinct extension/link status; selective uninstall (R-012..R-016).
- Closes #312, #315, #316.

### Out of Scope
- Changes to gentle-pi, Pi, or OpenCode; Pi session-end sync.
- Uninstalling the Subagents extension package itself.
- A user-facing choice between `pi-subagents-j0k3r` and `pi-subagents`.

## Capabilities

### New Capabilities
- `pipkg-integrity`: mode drift, build-root mode, path containment, SKILL.md name validation.
- `pi-build-provenance`: `labdrian.builtFrom` recording and ref-based sync-check comparison with fallback disclosure.
- `review-receipt-capture`: pre-acknowledge receipt persistence, receipt-sourced `approved_tree`, archive block/override.
- `gadu-pi-subagent`: extension install/no-op, overlay-owned GADU link, frontmatter compatibility, status, uninstall.

### Modified Capabilities
- `pi-runtime-target`: "Package-Delivered Skills and Agents" currently forbids writing under `~/.pi/agent/agents/`; amend to permit the single overlay-owned `GADU.md`. "Honest Status", "Pi-Scoped Uninstall", and "Pi Drift Detection" gain the extension/link states, GADU-link removal, and builtFrom-ref comparison.
- `actuals-instrumentation`: the "no receipt → self-asserted" path becomes an archive block unless an owner override is recorded.

## Approach

- Extend existing seams, no new modules: `engine/pipkg` and `engine/skills/parse.go` for R-001..R-004; additive `package.json` field consumed by `cmd_sync_check` for R-005..R-007.
- Receipt capture is an orchestrator-workflow step plus a small Go validator in `tools/archive-anchor-gate`; the persisted file is the only verified `approved_tree` source.
- GADU coexists with gentle-pi's `installSddAssets` because gentle-pi only overwrites its manifest-owned files; `tools: '*'` is verified to parse as a wildcard (research obs #3372), so the generator template stays; design confirms `model: opus` alias resolution.
- Strict TDD; Pi-integration tests use a scratch Pi home under the existing `TestMain` guards.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `engine/pipkg/pipkg.go` | Modified | Mode compare, 0755 root, containment, `builtFrom` |
| `engine/skills/parse.go` | Modified | `validateEntry` path + SKILL.md name checks |
| `bin/labdrian-overlay` | Modified | `cmd_sync_check` ref comparison; GADU install/status/uninstall |
| `tools/archive-anchor-gate` | Modified | Receipt-sourced `approved_tree`, block/override |
| `~/.claude/skills/_shared/sdd-orchestrator-workflow.md`, `inception-pipeline` closure-feedback | Modified | Capture step before acknowledge; read persisted receipt |
| `openspec/changes/<change>/review-receipts/` | New | Persisted receipt class |
| `engine/gadu/persona`, `agents/GADU.md` | Modified (if needed) | Frontmatter shape |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Capture step missed → receipt gone | Med | R-011 blocks archive; override is explicit and recorded |
| Collision with gentle-pi asset pruning | Low | Overlay-owned marker; non-interference integration test |
| `builtFrom` ref unresolvable locally | Med | R-007 fallback with disclosure |
| Third-party extension changes frontmatter contract | Med | Pin verified version in design; status reports `not-installed`/`conflict` honestly |
| Live-registry regression from name check | Low | Run validation over the live registry in verify |

## Rollback Plan

Each slice is an independent stacked PR: revert the slice. `builtFrom` is additive; removing it restores today's `main` comparison. Receipt files are inert data. GADU link and extension are removed by the selective uninstall; `pi remove` restores the prior Pi state.

## Dependencies

- `pi-subagents-j0k3r` (npm, owner-accepted external dependency).
- Archived `pi-runtime-target` constraints (ownership, scratch-home tests).

## Success Criteria

- [ ] GADU dispatches via gentle-pi `subagent_*` from the overlay-linked file.
- [ ] `sync-check --target pi` on a feature branch reports no false drift (#315).
- [ ] `pipkg` rejects mode drift, traversal, and name mismatch (#312).
- [ ] This change archives with a receipt-sourced `approved_tree` (#316).
