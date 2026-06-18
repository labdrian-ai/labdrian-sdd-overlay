# Component Composition

## Shared Primitive Threshold

Create or modify a shared primitive only when the pattern is cross-feature,
accessible, token-aligned, has a stable API, and has a clear owner. Otherwise,
keep the composition feature-local under `apps/frontend/src/features/**`.

Shared primitives belong in `apps/frontend/src/shared/components/ui/`. Domain
behavior belongs in the feature tree, even when it is reused inside one feature
domain.

## Existing Primitives First

- Import existing primitives from `@/shared/components/ui/*`.
- Do not run `shadcn init`.
- Do not vendor new shadcn or Radix component copies unless a separate approved
  change requires it.
- Preserve Radix accessibility wiring through Genesis wrappers.

## Styling and Variants

- Use `cn()` from `@/shared/lib` when conditional classes can conflict.
- Prefer Genesis tokens such as `bg-background`, `text-foreground`,
  `text-muted-foreground`, `border-border`, `bg-card`, `text-card-foreground`,
  and `ring-ring`.
- Use CVA only for reusable `variant` or `size` APIs.
- For one-off feature composition, use `cn()` directly instead of inventing a
  variant helper.

## Buttons and Actions

- Use the existing `Button` primitive and variants before local styling.
- Keep one primary/high-emphasis action per local action group.
- Put page-level actions in the header or collection toolbar.
- Put row-level or section-level actions near the content they affect.
- Destructive actions require destructive styling and confirmation when the
  action is irreversible, deletes data, revokes access, or changes security or
  billing state.

## Tables and Interactive Rows

Interactive rows must remain keyboard and accessibility safe. If a row opens a
detail view, ensure keyboard users can trigger the same action and that nested
row actions do not conflict with the row interaction.

Use existing table/list primitives before creating local table markup. Preserve
column APIs, focus behavior, visible action affordances, and accessible labels.

## Component Splitting

Split components by concern, readability, and testability. Do not split only
because a generic hard line-count rule says so. Prefer extraction when it makes
state ownership, rendering branches, or test setup clearer.
