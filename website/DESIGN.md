# Design — Takoform website

This is the locked visual system for the Takoform public website. It serves an
Experimental specification project, not a finished-standard marketing claim.
Every public page must expose project status, authority, and compatibility
boundaries before decorative proof or inventory counts.

## Genre

Modern-minimal, technical, and austere. The site is an instrument for reading
contracts and current evidence. It does not imitate an IDE, browser, cloud
dashboard, or standards body.

## Macrostructure family

- Overview: **Workbench**. Lifecycle facts, exact identities, and authority are
  the proof. Any code is shown as code, without fake window chrome.
- Documentation and specification: **Long Document**. Continuous reading,
  compact reference rows, and canonical-source links replace marketing cards.

## Theme

- Paper: `oklch(14% 0.012 25)`
- Raised paper: `oklch(18% 0.014 25)`
- Ink: `oklch(95% 0.006 25)`
- Secondary ink: `oklch(78% 0.008 25)`
- Rule: `oklch(32% 0.014 25)`
- Accent: `oklch(68% 0.18 25)`
- Focus: `oklch(82% 0.16 25)`

The aka-red accent is a signal, never a section fill. Functional state also
uses text or symbols so colour is never the only distinction.

## Typography

- Display: Iowan Old Style / Palatino / platform Japanese mincho, weight 700,
  roman. The site downloads no font and never blocks contract text on a third
  party.
- Body: the platform UI and Japanese sans stack.
- Code: the platform monospace stack.
- Scale: major-third, with display capped at `5.25rem`.
- Prose measure: `65ch`; technical rows may use the wider site grid.

## Spacing

A 4-point named scale is expressed through the VitePress spacing tokens used
in the theme CSS. Major sections vary their vertical rhythm; repeated equal
card padding is not part of this system.

## Motion

- No scroll reveal and no ambient animation.
- Links and buttons use one short colour/transform response.
- Focus rings are instant.
- Reduced motion removes spatial transitions.
- Navigation uses one dark, tight shadow token; content uses no shadows.

## Microinteraction stance

- Keyboard focus is always visible.
- Pointer hover exists only where the same affordance works by focus/tap.
- Touch targets are at least 44 CSS pixels.
- The language toggle changes content in place and preserves the selected
  language; it does not announce a success toast. The site is built with
  VitePress i18n: English at `/`, Japanese at `/ja/`.

## CTA voice

- Primary actions are compact accent-filled controls with specific verbs.
- In prose, use an underlined typographic link instead of a second button.
- Do not use “Learn more”, “Get started”, or marketing-only verbs where an
  exact destination name is available.

## Per-page allowances

- Overview may show real HCL and shell commands as bordered figures.
- Documentation and specification use typography only.
- No page may add fake chrome, screenshots without real evidence, invented
  metrics, testimonials, logos, gradients, or decorative illustration.

## What pages must share

- The Takoform wordmark, dark aka-red palette, fonts, navigation, footer, focus
  treatment, and status vocabulary.
- The facts: project `Experimental`; current Provider `3.0.0` is a
  Registry-published non-normative implementation of Host API
  `forms.takoform.com/v1`, mapping 31 typed resources across eight
  versionless families. The standalone Edge source publishes 16
  content-addressed Form packages. The current package envelope is
  `packages.forms.takoform.com/v1alpha5`. The `release/version.json`
  descriptor remains `candidate-only` source metadata after owner publication;
  the release identity ledger owns live distribution truth. Provider `2.1.1`
  and earlier lanes remain immutable Registry history. Host implementation
  source and live service availability are different evidence.

## What pages may differ on

- Overview may use a split proof column and the complete resource inventory.
- Documentation may prioritize provider usage and canonical generated docs.
- Specification may prioritize authority, lifecycle, and version boundaries.

## Exports

`.vitepress/theme/custom.css` is the executable source of truth. The equivalent
portable formats are recorded below for consumers; this VitePress site does not
execute Tailwind, DTCG, or shadcn configuration.

### Tailwind v4 `@theme`

```css
@theme {
  --color-paper: oklch(14% 0.012 25);
  --color-paper-2: oklch(18% 0.014 25);
  --color-paper-3: oklch(22% 0.014 25);
  --color-ink: oklch(95% 0.006 25);
  --color-ink-2: oklch(78% 0.008 25);
  --color-rule: oklch(32% 0.014 25);
  --color-rule-2: oklch(25% 0.014 25);
  --color-muted: oklch(68% 0.01 25);
  --color-neutral: oklch(60% 0.012 25);
  --color-accent: oklch(68% 0.18 25);
  --color-focus: oklch(82% 0.16 25);
  --font-display: "Iowan Old Style", "Palatino Linotype", "Yu Mincho", "Hiragino Mincho ProN", Georgia, serif;
  --font-body: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", "Noto Sans JP", "Hiragino Kaku Gothic ProN", "Yu Gothic", Meiryo, sans-serif;
  --font-mono: ui-monospace, "SFMono-Regular", Consolas, "Liberation Mono", monospace;
  --spacing-sm: 0.75rem;
  --spacing-md: 1rem;
  --spacing-lg: 1.5rem;
  --spacing-xl: 2.5rem;
  --ease-out: cubic-bezier(0.16, 1, 0.3, 1);
}
```

### DTCG `tokens.json`

```json
{
  "$schema": "https://design-tokens.github.io/community-group/format/",
  "color": {
    "paper": { "$value": "oklch(14% 0.012 25)", "$type": "color" },
    "ink": { "$value": "oklch(95% 0.006 25)", "$type": "color" },
    "accent": { "$value": "oklch(68% 0.18 25)", "$type": "color" },
    "focus": { "$value": "oklch(82% 0.16 25)", "$type": "color" }
  },
  "font": {
    "display": { "$value": "Iowan Old Style, Palatino Linotype, Yu Mincho, Hiragino Mincho ProN, Georgia, serif", "$type": "fontFamily" },
    "body": { "$value": "system-ui, -apple-system, BlinkMacSystemFont, Segoe UI, Noto Sans JP, Hiragino Kaku Gothic ProN, Yu Gothic, Meiryo, sans-serif", "$type": "fontFamily" },
    "mono": { "$value": "ui-monospace, SFMono-Regular, Consolas, Liberation Mono, monospace", "$type": "fontFamily" }
  },
  "space": {
    "md": { "$value": "1rem", "$type": "dimension" },
    "xl": { "$value": "2.5rem", "$type": "dimension" }
  }
}
```

### shadcn/ui CSS variables

```css
:root {
  --background: 14% 0.012 25;
  --foreground: 95% 0.006 25;
  --card: 18% 0.014 25;
  --card-foreground: 95% 0.006 25;
  --popover: 18% 0.014 25;
  --popover-foreground: 95% 0.006 25;
  --primary: 68% 0.18 25;
  --primary-foreground: 14% 0.012 25;
  --secondary: 22% 0.014 25;
  --secondary-foreground: 95% 0.006 25;
  --muted: 25% 0.014 25;
  --muted-foreground: 68% 0.01 25;
  --accent: 68% 0.18 25;
  --accent-foreground: 14% 0.012 25;
  --border: 32% 0.014 25;
  --input: 32% 0.014 25;
  --ring: 82% 0.16 25;
  --radius: 0.375rem;
}
```
