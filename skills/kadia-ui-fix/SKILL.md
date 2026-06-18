---
name: kadia-ui-fix
description: >
  Senior Frontend Engineering + UI Implementation for KADIA website. Works ONLY on residual visual/functional issues — never repeats completed fixes.
  Trigger: When fixing residual UI issues, CTA routing, numbering, header, footer, spacing, cards, or responsive on kadiacompany.com.
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## When to Use

- Fixing residual UI issues that survived previous QA rounds
- Correcting CTA routing and link destinations
- Fixing broken numbering formats
- Adjusting header/footer layout
- Reducing dead space or excessive padding
- Integrating images into internal pages
- Responsive validation at specific breakpoints

## Critical Rules

1. **LOAD kadia-content-guard FIRST** — never change narrative content
2. **Don't repeat completed fixes** — check git log before touching anything
3. **Build must pass** after every change: `npm run build`
4. **Test responsive** at: 1440, 1280, 1024, 768, 480, 390, 360

## Stack Context

- **Framework**: Next.js 15 App Router
- **Styling**: Tailwind CSS 4
- **Components**: `components/sections/`, `components/layout/`, `components/ui/`
- **Content**: `lib/content/home.ts`, `lib/content/pages.ts`
- **Routes**: `lib/site-map.ts`
- **Assets**: `public/assets/img/{desktop,mobile}/`

## Patterns

### Header (components/layout/header.tsx)
- Logo + nav + optional CTA
- Sticky with scroll-aware behavior
- Mobile: hamburger menu
- z-50 always

### Section Pattern
```tsx
<section className="section-padding relative overflow-hidden" id="section-id">
  {/* Optional background image */}
  <div className="absolute inset-0 z-0">
    <picture>...</picture>
    <div className="absolute inset-0 bg-white/[0.88]" />
  </div>
  {/* Content */}
  <div className="container-custom relative z-10">
    {/* section content */}
  </div>
</section>
```

### CTA Pattern
```tsx
<Button href="/ruta-correcta" className="bg-[#34163B] text-white ...">
  Label del CTA
</Button>
```

### Numbering Pattern
```tsx
<span>{`0${index + 1}`}</span>  // Renders: 01, 02, 03
```
NEVER use separate spans like `<span>0</span><span>1</span>` — CSS can break them apart.

### Card Pattern (BentoGrid)
- Use `space-y-2` between elements (not space-y-4+)
- Padding: `p-4 sm:p-5` (not p-8+)
- Height: `auto-rows-auto` (not forced heights)

## Site Routes

| Route | Page |
|-------|------|
| `/` | Home |
| `/arquitectura-empresarial` | Arquitectura Empresarial |
| `/arquitectura-tecnologica` | Arquitectura Tecnológica |
| `/inteligencia-artificial` | Inteligencia Artificial |
| `/metodologia` | Metodología |
| `/diagnostico-estrategico` | Diagnóstico Estratégico |
| `/casos` | Casos |
| `/sobre-kadia` | Sobre KADIA |
| `/contacto` | Contacto |

## Commands

```bash
# Build check
npm run build

# Dev server
npx next dev -p 3000

# Find broken numbering
grep -rn "0 [0-9]" components/ lib/

# Find all CTAs/links
grep -rn "href=" components/sections/

# Check for missing tildes
grep -rn "tecnologica\|diagnostico\|metodologia\|operacion" lib/ app/
```

## Available Images

All optimized (WebP + JPG, desktop 1600px + mobile 800px):
- `hero-architecture` — Hero background
- `section-signals` — Problem section
- `section-ia-fallback` — IA section fallback
- `section-cases` — Cases section
- `section-methodology` — Methodology section
- `section-about-kadia` — About section (portrait format)

Located in: `public/assets/img/{desktop,mobile}/`

## Resources

- Content guard: Load `kadia-content-guard` first
- Visual QA: `kadia-visual-qa` for review after fixes
- Assets guide: `docs/VISUAL_ASSETS_GUIDE.md`
