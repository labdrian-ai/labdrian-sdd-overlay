---
name: kadia-visual-qa
description: >
  Visual QA, UI Design Review, Copy QA and Accessibility audit for KADIA website. Reviews the site for residual visual issues after implementation.
  Trigger: When doing a visual QA pass, design review, or accessibility audit on kadiacompany.com.
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## When to Use

- After a round of UI fixes, verify they actually work
- Before a deploy, do a visual quality pass
- Reviewing contrast, spacing, card balance, image integration
- Checking copy for residual accent/punctuation errors
- Validating responsive behavior at all breakpoints

## Critical Rules

1. **LOAD kadia-content-guard FIRST** — report issues but don't rewrite content
2. **Don't re-audit completed fixes** — check git log for what's already resolved
3. **Build must pass** during QA: `npm run build`
4. **Be specific** — report file, line, before/after for every issue

## QA Checklist

### 1. Contrast on Dark Backgrounds
- [ ] ALL text on dark bg is `text-white` or `text-white/80+`
- [ ] Headings: `text-white` (100%)
- [ ] Body: minimum `text-white/80`
- [ ] Labels: minimum `text-white/70`
- [ ] NEVER `text-white/50` or lower on dark
- [ ] Overlays: desktop 85%+, mobile 90%+

### 2. Section Transitions
- [ ] No abrupt background changes between sections
- [ ] Gradient bridges where dark→light or light→dark
- [ ] Alternating rhythm: dark→light→dark→light is intentional
- [ ] Marquee/transition bands don't look buggy

### 3. Card Balance
- [ ] Cards respond to content height (not forced)
- [ ] Internal spacing: `space-y-2` between elements
- [ ] Padding: `p-4 sm:p-5` (not excessive)
- [ ] No "big box with little content" feeling
- [ ] Consistent across equivalent cards

### 4. Mobile Responsive (test at 360px, 390px, 480px, 768px)
- [ ] No horizontal overflow
- [ ] Buttons centered and full-width on mobile
- [ ] Text wraps properly (no overflow)
- [ ] Grids stack to single column
- [ ] Footer stacks to single column
- [ ] Header doesn't break (logo/nav gap maintained)

### 5. Copy QA (Spanish)
- [ ] All tildes present: tecnológica, diagnóstico, metodología, operación, decisión, área, números, aún
- [ ] Question marks: ¿...? with opening mark
- [ ] Consistent terminology throughout
- [ ] No double spaces or trailing whitespace
- [ ] Metadata (title, description) also accented

### 6. Image Integration
- [ ] Images feel part of the composition, not "added after"
- [ ] Proper aspect ratios (not stretched/squished)
- [ ] WebP with JPG fallback
- [ ] Desktop + mobile variants
- [ ] `loading="lazy"` on all except hero
- [ ] Alt text descriptive and present

### 7. Accessibility
- [ ] Heading hierarchy: h1 → h2 → h3 (no skips)
- [ ] `aria-label` on all icon/social links
- [ ] `focus-visible` outline on interactive elements
- [ ] Touch targets: minimum 44x44px
- [ ] Links to nowhere: `aria-disabled="true"` or removed

### 8. Spacing Rhythm
- [ ] `.section-padding` applied consistently
- [ ] `container-custom` on all sections
- [ ] No rogue hardcoded paddings
- [ ] Hero has intentional different padding (tall viewport)
- [ ] No dead space between sections

### 9. Numbering Format
- [ ] Format: `01`, `02`, `03` (no space between digits)
- [ ] Consistent across: hero, IA, method, services, differential, editorial pages
- [ ] Not broken by CSS flex/gap

### 10. Footer
- [ ] Social icons: visible, clickable, with aria-label
- [ ] Icons open in new tab
- [ ] Focus ring visible
- [ ] No dead links (href="#")
- [ ] Legal links either real or removed

## Breakpoints to Test

| Breakpoint | Width | Device |
|------------|-------|--------|
| Desktop XL | 1440px | Large monitors |
| Desktop | 1280px | Standard laptop |
| Laptop | 1024px | Small laptop/iPad landscape |
| Tablet | 768px | iPad portrait |
| Mobile L | 480px | Large phones |
| Mobile | 390px | iPhone |
| Mobile S | 360px | Android small |

## Commands

```bash
# Build verification
npm run build

# Search for contrast issues
grep -rn "text-white/50\|text-white/40\|text-white/30" components/sections/

# Search for broken numbering
grep -rn ">0 <\|\"0 \"" components/ lib/

# Search for missing tildes
grep -rn "tecnologica\|diagnostico\|metodologia\|operacion\|decision\|area\|numeros" --include="*.ts" --include="*.tsx" lib/ app/

# Search for dead links
grep -rn "href=\"#\"" components/

# Check heading hierarchy
grep -rn "<h[1-6]" components/sections/
```

## Issue Report Format

```markdown
### Issue: [Short description]
- **File**: `path/to/file.tsx:47`
- **Severity**: High/Medium/Low
- **Current**: `text-white/50` (too low opacity)
- **Expected**: `text-white/80` (minimum for readability)
- **Fix**: Change opacity class
```

## Resources

- Content guard: Load `kadia-content-guard` first
- UI fixes: `kadia-ui-fix` for implementing corrections
- Assets: `public/assets/img/{desktop,mobile}/`
- Visual guide: `docs/VISUAL_ASSETS_GUIDE.md`
