# Page and List Recipes

## Page Shells

Default feature pages use a full-page shell that can host scrolling content
without breaking nested layouts, such as `flex h-full min-h-0 flex-col gap-6 p-6`
or the closest nearby feature equivalent.

Use centered or max-width shells for focused detail, onboarding, settings, or
form flows where unconstrained width hurts readability. Dense admin layouts are
allowed for high-volume operational screens when density is consistent.

Do not claim every Genesis feature already uses a universal page shell. Service
Desk currently has the strongest page/list patterns.

## Page Headers

Standard page headers contain title, optional lead text, optional metadata, and
optional page-level actions. Header actions should affect the whole page or
collection. Row-level, section-level, or form-step actions belong near the
content they affect.

## Collection/List Views

Compose list pages as: page shell, optional page header, optional toolbar,
list/table surface, pagination or footer controls.

- Search and filters belong above the list content and visually connected to the
  collection they affect.
- Create/import/export actions belong in the page header or toolbar, not mixed
  with row actions.
- Row actions belong at the row edge or in a row action menu.
- Pagination belongs below the list and should preserve filter and search
  context.
- Infinite scroll needs a documented product reason.

## Responsive Behavior

On constrained widths, prefer horizontal scroll for data-dense comparison tables
and card/list stacking for record summaries. Do not hide critical columns or
actions without an alternate path.

## List States

Loading states should match the final list structure: table skeleton rows for
tables and card skeletons for card lists. Empty states sit inside the list
surface and explain what is missing, why it matters, and the next action when
one exists. Collection-level errors belong at the collection level; row-level
errors stay attached to the affected row or action.
