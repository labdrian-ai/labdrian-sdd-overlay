# Migration Notes

These are known design-system debts. Do not fix them opportunistically during an
unrelated UI task. Record or propose a focused follow-up when they block work.

## Known Debts

- DataTable row-click behavior may have an accessibility risk and should be
  reviewed before expanding interactive row patterns.
- Service Desk has the strongest page/list patterns; Genesis does not yet have a
  universal feature shell across every area.
- Dialog sizing needs a clearer shared recipe.
- Semantic status colors still need a shared recipe outside Service Desk; the
  Service Desk badge recipe in `references/status-colors.md` is the canonical
  pattern to extend.
- `CustomSelect` is legacy/secondary. Prefer direct shared `Select` for new
  work unless an existing bridge case requires `CustomSelect`.

## Resolved Debts

- Duplicate `buttonVariants` source was resolved by keeping the canonical export
  in `shared/components/ui/button.tsx` and removing `button.variants.ts`.

## Migration Rule

When a known debt appears during implementation, keep the requested change
focused. Do not modify application code or shared primitive APIs unless the task
explicitly includes that migration.
