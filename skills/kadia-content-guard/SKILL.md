---
name: kadia-content-guard
description: >
  Content restriction guard for KADIA website. Enforces the rule that ONLY visual/functional fixes are allowed — never narrative, commercial or conceptual changes.
  Trigger: When working on any KADIA website fix, before making any edit.
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## When to Use

ALWAYS load this skill BEFORE any kadia-ui-fix or kadia-visual-qa work. It defines what you CAN and CANNOT touch.

## Critical Restriction

**NEVER change:**
- Texto narrativo, comercial ni conceptual ya aprobado
- Títulos, párrafos, claims, propuesta de valor ni estructura argumental
- La voz de marca de KADIA
- La propuesta de valor ni el enfoque comercial
- La estructura general de la página

**ONLY fix:**
- Errores visuales (layout, spacing, alignment)
- Contraste (text legibility over backgrounds)
- Espaciado (padding, margins, gaps)
- Integración de imágenes
- Alineación (flex, grid, responsive)
- Enlaces y CTAs (routing, destinations)
- Responsive (mobile, tablet, desktop)
- Tildes o errores ortográficos residuales VISIBLES
- Accesibilidad (aria-labels, focus states, touch targets)

## Decision Tree

```
¿El cambio modifica el significado del texto?
├── SÍ → NO HACERLO
└── NO → ¿Es un error visual, funcional o de accesibilidad?
    ├── SÍ → APLICAR
    └── NO → NO HACERLO
```

## Examples

### ✅ PERMITIDO
- `tecnologica` → `tecnológica` (tilde residual)
- `text-[10px]` → `text-xs` (accesibilidad)
- `href="#"` → `href="/contacto"` (link funcional)
- `gap-2` → `gap-4` (espaciado visual)
- Agregar `aria-label` a un link (accesibilidad)

### ❌ PROHIBIDO
- Reescribir un párrafo del hero "para que suene mejor"
- Cambiar "Estudio de arquitectura empresarial" por otra propuesta
- Reordenar secciones para "mejor flujo narrativo"
- Agregar claims o datos nuevos no aprobados
- Modificar la propuesta de valor

## Resources

- Content source: `lib/content/home.ts`, `lib/content/pages.ts`
- Site map: `lib/site-map.ts`
