# Dialogs, Modals, and Drawers

## Choice

Use dialogs for focused confirmation, short forms, and bounded decisions that
intentionally interrupt the current page. Use drawer-style interactions only for
contextual side workflows where keeping page context visible materially helps.

## Structure

Keep title, description, content, and footer actions semantically separated.
Preserve Radix accessibility wiring through existing Genesis wrappers.

## Sizing Tiers

- Small/default: confirmations, short decisions, simple content.
- Medium form: multi-field forms with clear submit and cancel actions.
- Tall/complex: scrollable content with sticky or clearly reachable actions.

Avoid arbitrary one-off dimensions when a documented tier fits. Dialog sizing is
a known recipe debt, so prefer nearby Genesis examples and document follow-up
work when a new tier is needed.

## Destructive Dialogs

Destructive dialogs must name the destructive outcome and require an explicit
confirmation action. Do not rely on color alone to communicate risk.
