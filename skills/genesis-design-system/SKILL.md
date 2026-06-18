---
name: genesis-design-system
description: "Authoritative Genesis frontend design-system guidance for React + Vite UI, shared primitives, forms, dialogs, tables, status styling, and Streamdown. Trigger: when creating, modifying, reviewing, or refactoring Genesis frontend UI components, forms, dialogs, tables, page/list layouts, shared visual primitives, badges/status styling, or chat markdown rendering."
license: Apache-2.0
metadata:
  author: genesis
  version: "1.1"
---

## Activation Contract

Use this skill for UI work in `apps/frontend`. This is the single authoritative
Genesis project skill for frontend design-system behavior.

Do not load retired project UI skills such as `genesis-frontend-ui` or
`genesis-streamdown`; their useful rules are consolidated here and in
`references/`.

## Hard Rules

Rules are listed in precedence order: when two rules conflict, the lower number
wins.

1. The user's exact requested UX/UI delta wins unless it violates accessibility,
   security, or explicit Genesis constraints.
2. Nearby Genesis implementation and this project design-system skill beat
   generic global React, Tailwind, Vercel, shadcn, or component-pattern skills.
3. Genesis frontend is React + Vite. Do not introduce Next.js, React Server
   Components, Server Actions, or App Router advice unless the project changes.
4. Use existing shared primitives from `@/shared/components/ui/*`. Do not run
   `shadcn init` and do not vendor new component library copies.
5. Use `cn()` from `@/shared/lib` for conditional or conflicting classes.
6. Use `createZodResolverV4`, not `zodResolver`, for React Hook Form + Zod v4.
7. Use `StreamdownRenderer`; never instantiate `Streamdown` directly in feature
   components.
8. Prefer Genesis tokens and documented semantic status recipes over ad hoc
   hard-coded colors.
9. Interactive table rows must be keyboard and accessibility safe.
10. Split or refactor components by concern, readability, and testability, not
    by generic hard line-count dogma.

## Decision Gates

| Situation | Action |
| --- | --- |
| Requested UI delta conflicts with a generic skill | Follow the request and Genesis rules. |
| Requested UI delta conflicts with accessibility, security, or explicit Genesis constraints | Preserve the constraint and explain the conflict. |
| Existing nearby feature has a clear pattern | Extend that pattern before inventing a new recipe. |
| Pattern is cross-feature, accessible, token-aligned, has a stable API, and has a clear owner | Consider a shared primitive. |
| Pattern is one-off, domain-specific, or still evolving | Keep it feature-local. |
| Streamed markdown rendering is needed | Use the shared `StreamdownRenderer` wrapper. |
| RHF form uses Zod v4 | Use `createZodResolverV4` from `@/shared/lib`. |

## Core Paths

| Concern | Canonical Path |
| --- | --- |
| Shared UI primitives | `apps/frontend/src/shared/components/ui/` |
| Class merging helper | `apps/frontend/src/shared/lib/utils.ts` |
| Zod v4 RHF resolver | `apps/frontend/src/shared/lib/zod-resolver.ts` |
| Feature UI composition | `apps/frontend/src/features/**/components/` |
| Streamed markdown wrapper | `apps/frontend/src/shared/components/streamdown-renderer.tsx` |

## Execution Steps

1. Inspect the nearby Genesis implementation for the target feature before
   applying generic framework guidance.
2. Apply the user's requested UX/UI delta directly, unless a priority rule blocks
   it.
3. Use shared primitives and tokens first; keep feature-specific composition in
   the owning feature tree.
4. Read only the relevant reference file below when the task needs that detail.
5. Validate with targeted static checks or frontend tests when code changes are
   made.

## Progressive References

Read only the reference file the task needs.

| Task involves | Read |
| --- | --- |
| Shared vs feature placement, variants, `cn()`, tables, cards, actions, component splitting | `references/component-composition.md` |
| React Hook Form + Zod v4 forms, selects, validation, submit states | `references/forms.md` |
| Page shells, headers, list views, toolbars, filters, pagination, collection states | `references/page-list-recipes.md` |
| Dialogs, drawers, confirmations, modal forms | `references/dialogs.md` |
| Badges, callouts, status colors, semantic tokens, destructive/warning/success styling | `references/status-colors.md` |
| Chat markdown, code blocks, Mermaid, tables, links, the Streamdown wrapper | `references/streamdown.md` |
| Touching shared primitives, or a UI inconsistency that looks like known design-system debt | `references/migration-notes.md` |

## Do Not

- Do not edit global user skills to solve Genesis UI drift.
- Do not create app-local skills under `apps/*/.agents/skills/`.
- Do not bypass existing wrappers for dialogs, dropdowns, selects, sheets,
  tables, buttons, or Streamdown without a documented reason.
- Do not promote a component to `shared/components/ui` only to avoid a small
  local duplicate.
- Do not claim Genesis has a universal page shell across every feature; Service
  Desk currently has the strongest page/list patterns.

## Output Contract

When reporting UI work, include the Genesis design-system rules used, any
accessibility/security constraint that changed the requested delta, validation
performed, and follow-up design-system debt that was intentionally not fixed.
