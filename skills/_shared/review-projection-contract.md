---
applies_to_phases: [sdd-apply, sdd-verify]
excluded_phases: [sdd-explore, sdd-propose, sdd-spec, sdd-design, sdd-tasks, sdd-archive]
injection_point: "## Skills to load before work"
---
# Review Projection Contract

> Advisory scope: this contract applies to the phases that produce or gate a review candidate
> (`sdd-apply`, `sdd-verify`). `engine/gate/gate.go` matches `subagent_type` against the
> frontmatter above to inject it; `sdd-apply` is not vendored in this repository, so that
> injection is the only path this rule has into it.

## The hazard

`gentle-ai review` projects the **workspace** by default, and `sdd-apply` commits each work
unit as it goes. Once apply finishes the tree is clean, so the workspace projection has nothing
left to see: `base_tree == current_candidate_tree`, `paths: []`, zero lenses, correction budget
`0`.

That review does not fail. It inspects nothing and issues an **APPROVED RECEIPT over nothing** —
every defect in the work you meant to review ships unreviewed and formally approved. This
happened in this repository and cost six undetected CRITICAL defects.

## The rule

NEVER start a review without first proving the candidate covers paths. Run the guard:

```
bin/labdrian-overlay review-preflight [--base-ref <remote>/<branch>]
```

| Exit | Meaning | Action |
|---|---|---|
| `0` | the candidate covers changed paths | start the review |
| `1` | EMPTY candidate | do NOT start; apply a remedy below |
| `2` | usage error — nothing was inspected | fix the invocation and re-run |
| `3` | no verdict could be reached (fails closed) | do NOT start; this is absence of proof, never proof of safety |

Exit `3` is not a soft `0`. The guard could not observe the projection, so nothing is known
about what a review would cover. Treating it as permission reproduces the hazard one layer up.

## Remedies for an EMPTY candidate

1. **Project base-to-HEAD.** Pass `--base-ref <remote>/<branch>` (for example
   `--base-ref origin/main`) to the guard AND to the review you start. Use this when the work
   is already committed. If the guard still reports EMPTY under `--base-ref`, the ref is wrong:
   repoint it at the ref this branch actually diverged from.
2. **Review before committing.** Leave the work uncommitted until the review has started, so
   the default workspace projection still covers it.

Never work around the guard by skipping it. A skipped guard and a clean tree produce the exact
approved-receipt-over-nothing outcome this contract exists to prevent.
