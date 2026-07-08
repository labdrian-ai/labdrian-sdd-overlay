# Proposal: OO Quality Contract

## Intent

Create a compact local OO quality contract that distills useful SOLID/OOP ideas into advisory guidance for domain-heavy TypeScript/NestJS work without importing `ramziddin/solid-skills` or turning OO guidance into global SDD dogma.

## Scope

### In Scope
- Add `skills/_shared/oo-quality-contract.md` as a concise shared contract.
- Track the contract in `overlay.manifest` as `_shared/oo-quality-contract.md custom`.
- Define phase/language/context frontmatter: include design/tasks/apply contexts; exclude `sdd-propose` and `sdd-spec`.
- Add specs and artifact-level tests proving scope, precedence, exclusions, and no external vendoring.

### Out of Scope
- Vendoring or copying the external `solid-skills` repository.
- Globally injecting SOLID guidance into all SDD phases or non-OO work.
- Changing GADU, PR #76, or generated artifacts by hand.
- First-slice deterministic engine propagation/gate wiring; defer unless specs prove manual/artifact checks are insufficient.

## Capabilities

### New Capabilities
- `oo-quality-contract`: Advisory, context-gated OO/domain quality contract for TypeScript/NestJS-heavy work.

### Modified Capabilities
- None.

## Approach

Use the smallest viable coherent slice: create the manifest-tracked shared contract plus specs/docs/tests. The contract must state that specs, design, project conventions, minimalism, and review budget take precedence. It should use SOLID as diagnostic vocabulary only, favor simple procedural/data-oriented code when appropriate, and treat abstractions/value objects/patterns as tools justified by active variation or invariants.

### Review Budget Audit Update

The original proposal expected the first slice to stay within the 800-line review budget. Pre-commit review of the staged diff later found 883 insertions before the corrective auditability/test adjustment, mostly due to archived SDD documentation being part of the staged review surface. The maintainer/user approved a single-PR size exception on 2026-07-08; this update preserves the original forecast as historical context rather than treating it as accurate.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `skills/_shared/oo-quality-contract.md` | New | Canonical advisory contract. |
| `overlay.manifest` | Modified | Tracks `_shared/oo-quality-contract.md custom`. |
| `openspec/changes/oo-quality-contract/specs/` | New | Delta/new capability spec. |
| `engine/` | Deferred | No propagation/gate wiring in first slice. |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| OO/SOLID dogma leaks into unsuitable work | Med | Explicit context gates and precedence rules. |
| Contract becomes an unreviewed runtime behavior change | Med | First slice avoids engine wiring. |
| Manifest/registry confusion for `_shared/*` files | Low | Treat manifest as source of truth; do not hand-edit generated registry. |

## Rollback Plan

Remove `skills/_shared/oo-quality-contract.md`, delete its `overlay.manifest` entry, and archive/revert the OpenSpec change artifacts. No runtime engine wiring is changed in the recommended slice.

## Dependencies

- Existing minimalism-contract precedent and OpenSpec hybrid persistence.
- No external repository dependency.

## Success Criteria

- [ ] Contract exists, is English-only, concise, local, and non-vendored.
- [ ] `overlay.manifest` tracks the shared contract.
- [ ] Specs prove `sdd-propose` and `sdd-spec` exclusion.
- [ ] First-slice tests are artifact-level; pre-commit audit records that the staged diff exceeded the 800-line review budget and proceeded under the maintainer-approved 2026-07-08 single-PR size exception.
