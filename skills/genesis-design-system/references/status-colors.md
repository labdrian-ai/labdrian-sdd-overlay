# Status Colors and Semantic Styling

## Token-First Rule

Prefer Genesis tokens and documented semantic recipes over ad hoc hard-coded
colors. Start with neutral styling for labels, categories, and metadata.

Use semantic status styling only when color communicates domain state: success
or active, warning or attention, destructive or error, and informational.

## Accessibility

Status badges and callouts must pair color with text. Never rely on color alone.

## Recipes

- Neutral metadata: token-first muted text, border, and background.
- Success/active: token-compatible green or success recipe only when the domain
  state benefits from positive emphasis.
- Warning/attention: warning recipe for pending, risky, or needs-attention
  states.
- Destructive/error: destructive recipe for errors, irreversible actions, or
  data-loss risk.
- Informational: informational recipe for guidance, routes, and non-blocking
  notes.

Semantic color recipes are a known design-system debt across most features.
When nearby Genesis code already has a consistent recipe, extend it. When
recipes conflict, keep the change focused and record a follow-up instead of
inventing a new color system.

## Canonical Badge Recipe (Service Desk)

The Service Desk badges in
`apps/frontend/src/features/service-desk/components/badges/` are the strongest
semantic badge recipe in Genesis. Extend this pattern for new status badges
instead of inventing a new one:

- Build on the shared `Badge` primitive with `variant="outline"`.
- Define an exported config record per domain value:
  `Record<DomainValue, { label: string; className: string; dotClassName: string }>`.
- Pair light and dark classes in each recipe, e.g.
  `border-green-200 bg-green-50 text-green-700 dark:border-green-900/60 dark:bg-green-950/40 dark:text-green-300`.
- Provide a neutral fallback config (`bg-muted text-muted-foreground
  border-border`) for unknown or missing values, guarded by a type predicate.
- Render an `aria-hidden` color dot plus a visible text label, never color
  alone.
- Keep each badge component test-covered next to its implementation.
