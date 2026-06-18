# Streamdown Renderer

## Wrapper Contract

Use `StreamdownRenderer` from
`apps/frontend/src/shared/components/streamdown-renderer.tsx`. Do not instantiate
`Streamdown` directly in feature components when the shared wrapper satisfies
the need.

Current wrapper props:

```tsx
type StreamdownRendererProps = {
  content: string;
  className?: string;
  isStreaming?: boolean;
};
```

## Project Choices

- Use plugins `{ code, mermaid }` from `@streamdown/code` and
  `@streamdown/mermaid`.
- Use `caret="block"` and bind `isAnimating` to the wrapper `isStreaming` prop.
- Keep `skipHtml` enabled for chat content.
- Keep `linkSafety={{ enabled: false }}` unless product requirements explicitly
  restore Streamdown's confirmation modal.
- Keep code, table, and Mermaid controls enabled.
- Mermaid uses `download`, `copy`, `fullscreen`, and `panZoom` controls.
- Use `shikiTheme={["github-light", "github-dark"]}` to match current light and
  dark behavior.
- Customize presentation through the wrapper `components` map, not by scattering
  setup across feature components.

## Semantic Blockquotes

The wrapper detects variants from rendered text. Keep exact prefixes to avoid
false positives.

| Prefix / Pattern | Variant | Intent |
| --- | --- | --- |
| `Route:` | `route` | Navigation or menu route guidance |
| `Warning:` | `warning` | Risk or caution callout |
| `Tip:` | `tip` | Helpful recommendation |
| `(N)` or `` `(N)` `` at start | `route` | Backward-compatible numbered route marker |

## Security

Assistant output is untrusted markdown. Do not enable raw HTML or new plugin
classes without checking frontend bundle and security impact.
