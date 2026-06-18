# Forms

## React Hook Form + Zod v4

Use `createZodResolverV4` from `@/shared/lib`; do not use `zodResolver` from
`@hookform/resolvers` for Genesis Zod v4 forms.

```tsx
const resolver = createZodResolverV4(CreateThingSchema);

const form = useForm<CreateThingInput>({
  resolver,
  defaultValues,
});
```

Instantiate the resolver outside the component when it has no per-render
dependencies. Type `useForm<T>()` with the schema input or output used by the
component.

## Field Anatomy

Use this order: label, control, optional description or helper text, validation
error. Avoid placeholder-only labels when the form has visible labels elsewhere.

Inline errors belong under the affected control. Form-level errors belong near
the submit action or at the top of the form when they block submission globally.

## Submit Behavior

Submit buttons must show pending and disabled state during submission. Preserve
the user's entered values on failed submission or while async work is pending.

## Selects

Prefer direct shared `Select` usage from `@/shared/components/ui/*` for new
work. `CustomSelect` is legacy/secondary and should be used only for existing
option-description or React Hook Form bridge cases that already depend on that
API.
