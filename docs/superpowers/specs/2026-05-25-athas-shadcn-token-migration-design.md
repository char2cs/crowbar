# Design: Athas → shadcn/ui Token Migration

**Date:** 2026-05-25
**Branch:** feature/agentic-ide-gordon
**Status:** Approved

---

## Problem

The codebase has two parallel CSS variable systems:

1. **shadcn/ui canonical tokens** — declared in `index.css` using OKLCH, consumed by shadcn components (`--background`, `--foreground`, `--accent`, `--border`, `--destructive`, etc.)
2. **Athas custom tokens** — consumed in editor/feature CSS files but **never declared anywhere**, relying entirely on hardcoded hex fallbacks inside each `var()` call (`--color-text`, `--color-hover`, `--color-selected`, `--color-primary-bg`, etc.)

This causes three problems:
- shadcn components and editor components use different vocabulary for the same semantic concepts
- Themes cannot be applied — the ThemeRegistry has no surface to write to for editor colors
- The hardcoded hex fallbacks are all dark-mode values, making real light-mode support impossible

---

## Goals

1. **Eliminate parallel systems** — one canonical token per semantic concept
2. **shadcn component parity** — editor/UI components speak the same token language as shadcn components
3. **Theme switching readiness** — `ThemeDefinition` covers the full token surface; `ThemeRegistry.applyTheme()` writes one flat object

---

## Approach: Hybrid Rename + New Custom Tokens

- Variables with a clean 1:1 shadcn equivalent → **rename call sites** to use the canonical shadcn name directly
- Variables with no shadcn equivalent → **declare as new custom tokens** in `index.css`, registered in `@theme inline`
- All new token values use **OKLCH color space**, consistent with existing shadcn declarations
- All hardcoded hex fallbacks in `var()` calls are **removed** — once tokens are declared, fallbacks are redundant and hide missing declarations

---

## Variable Mapping

### Clean 1:1 Renames (no new tokens introduced)

| Athas variable(s) | → shadcn canonical | Notes |
|---|---|---|
| `--color-text`, `--text` | `--foreground` | Exact semantic match |
| `--color-text-light`, `--color-text-lighter` | `--muted-foreground` | Use `color-mix()` opacity for lighter variant |
| `--color-primary-bg` | `--background` | Page/editor background |
| `--color-secondary-bg` | `--card` | Secondary surface |
| `--color-hover`, `--color-selected`, `--color-accent`, `--accent` | `--accent` | shadcn `--accent` is the hover/interactive bg token |
| `--color-border` | `--border` | Exact match |
| `--color-error`, `--error` | `--destructive` | Exact match |
| `--app-font-family` | `--font-sans` | Already in `@theme inline` |

### New Custom Tokens (declared in `index.css`)

| New token | Light (`:root`) | Dark (`.dark`) | Purpose |
|---|---|---|---|
| `--warning` | `oklch(0.75 0.15 85)` | `oklch(0.80 0.16 85)` | Diagnostic warning underline |
| `--info` | `oklch(0.65 0.15 250)` | `oklch(0.72 0.15 250)` | Diagnostic info underline |
| `--editor-font-family` | `ui-monospace, 'Cascadia Code', 'Fira Code', monospace` | same | Editor-specific font |
| `--ui-text-xs` | `0.6875rem` | same | Component font-size token |
| `--ui-text-sm` | `0.75rem` | same | Component font-size token |
| `--app-scrollbar-size` | `11px` | same | Scrollbar width/height |
| `--app-scrollbar-thumb` | `oklch(0.55 0 0 / 42%)` | same | Scrollbar thumb color |
| `--app-scrollbar-thumb-border` | `3px solid transparent` | same | Thumb border (clip trick) |
| `--app-scrollbar-thumb-hover` | `oklch(0.55 0 0 / 58%)` | same | Hovered thumb color |
| `--app-scrollbar-track` | `transparent` | same | Track background |
| `--app-scrollbar-radius` | `999px` | same | Thumb border-radius |

### Syntax Highlighting Tokens (`--syntax-*`)

All declared in `:root` (light) and `.dark` (dark), using OKLCH. Dark values are perceptual conversions of the existing Material Ocean hex fallbacks. Light values reduce lightness while preserving hue and chroma.

| Token | Dark OKLCH | Light OKLCH |
|---|---|---|
| `--syntax-keyword` | `oklch(0.68 0.18 310)` | `oklch(0.45 0.18 310)` |
| `--syntax-string` | `oklch(0.85 0.14 130)` | `oklch(0.42 0.14 140)` |
| `--syntax-number` | `oklch(0.75 0.15 45)` | `oklch(0.50 0.15 45)` |
| `--syntax-constant` | `oklch(0.85 0.09 215)` | `oklch(0.48 0.12 215)` |
| `--syntax-comment` | `oklch(0.56 0.01 255)` | `oklch(0.52 0.01 255)` |
| `--syntax-variable` | `oklch(0.70 0.15 15)` | `oklch(0.45 0.15 15)` |
| `--syntax-property` | `oklch(0.72 0.15 270)` | `oklch(0.45 0.15 270)` |
| `--syntax-type` | `oklch(0.87 0.16 95)` | `oklch(0.52 0.16 95)` |
| `--syntax-function` | `oklch(0.72 0.15 270)` | `oklch(0.45 0.15 270)` |
| `--syntax-operator` | `oklch(0.85 0.09 215)` | `oklch(0.48 0.12 215)` |
| `--syntax-punctuation` | `oklch(0.90 0 0)` | `oklch(0.25 0 0)` |
| `--syntax-tag` | `oklch(0.70 0.15 15)` | `oklch(0.45 0.15 15)` |
| `--syntax-attribute` | `oklch(0.68 0.18 310)` | `oklch(0.45 0.18 310)` |
| `--syntax-boolean` | `oklch(0.65 0.22 15)` | `oklch(0.42 0.22 15)` |
| `--syntax-null` | `oklch(0.65 0.22 15)` | `oklch(0.42 0.22 15)` |
| `--syntax-regex` | `oklch(0.85 0.09 215)` | `oklch(0.48 0.12 215)` |
| `--syntax-jsx` | `oklch(0.85 0.09 215)` | `oklch(0.48 0.12 215)` |
| `--syntax-jsx-attribute` | `oklch(0.68 0.18 310)` | `oklch(0.45 0.18 310)` |
| `--syntax-markdown-heading` | `oklch(0.72 0.15 270)` | `oklch(0.45 0.15 270)` |
| `--syntax-markdown-bold` | `oklch(0.75 0.15 45)` | `oklch(0.50 0.15 45)` |
| `--syntax-markdown-italic` | `oklch(0.68 0.18 310)` | `oklch(0.45 0.18 310)` |
| `--syntax-markdown-strikethrough` | `oklch(0.70 0.15 15)` | `oklch(0.45 0.15 15)` |
| `--syntax-markdown-link` | `oklch(0.85 0.09 215)` | `oklch(0.48 0.12 215)` |
| `--syntax-markdown-link-text` | `oklch(0.72 0.15 270)` | `oklch(0.45 0.15 270)` |
| `--syntax-markdown-code` | `oklch(0.85 0.14 130)` | `oklch(0.42 0.14 140)` |
| `--syntax-markdown-list` | `oklch(0.68 0.18 310)` | `oklch(0.45 0.18 310)` |
| `--syntax-markdown-quote` | `oklch(0.56 0.01 255)` | `oklch(0.52 0.01 255)` |

### Variables Left In Place

`--editor-padding-top/right/bottom/left` — layout geometry, not theme colors. Remain declared locally in `athas-editor/styles/overlay-editor.css` and `editor/styles/overlay-editor.css`.

---

## File Scope

### `web/src/index.css`
- Add all new custom token declarations to `:root` and `.dark` blocks
- Add new tokens to `@theme inline` for Tailwind utility access
- No removals, no structural changes

### CSS files with call-site renames

| File | Changes |
|---|---|
| `features/athas-editor/styles/overlay-editor.css` | `--color-text` → `--foreground`, `--color-border` → `--border`, `--color-text-lighter` → `--muted-foreground`, `--color-error` → `--destructive`, `--accent` → `--accent`, `--color-primary-bg` → `--background`; strip hex fallbacks |
| `features/athas-editor/styles/overlay-card.css` | `--color-border` → `--border`, `--color-secondary-bg` → `--card`, `--color-hover` → `--accent`; strip hex fallbacks |
| `features/athas-editor/styles/token-theme.css` | `--text` → `--foreground`, `--hover` fallback → `--accent`; strip all hex fallbacks from `var()` calls |
| `features/editor/styles/overlay-editor.css` | Same as athas-editor copy |
| `features/editor/styles/token-theme.css` | Same as athas-editor copy |
| `features/file-explorer/styles/file-explorer-tree.css` | `--color-hover` → `--accent`, `--color-selected` → `--accent`, `--color-border` → `--border`, `--color-accent` → `--accent`, `--color-text-lighter` → `--muted-foreground`, `--color-primary-bg` → `--background`, `--app-font-family` → `--font-sans` |
| `features/terminal/styles/terminal.css` | No renames needed — only references `--app-scrollbar-*` tokens (kept as custom tokens). Strip the two hardcoded hex fallbacks on lines 59 and 65. |

### `web/src/extensions/themes/types.ts`
Add `ThemeTokens` interface covering the full token surface (shadcn base + all custom tokens). Update `ThemeDefinition` to include `tokens: ThemeTokens`. The stub `ThemeRegistry` method signatures remain unchanged.

```ts
export interface ThemeTokens {
  // shadcn base
  background: string; foreground: string;
  card: string; cardForeground: string;
  popover: string; popoverForeground: string;
  primary: string; primaryForeground: string;
  secondary: string; secondaryForeground: string;
  muted: string; mutedForeground: string;
  accent: string; accentForeground: string;
  destructive: string; border: string; input: string; ring: string;

  // custom
  warning: string; info: string;
  editorFontFamily: string;
  uiTextXs: string; uiTextSm: string;
  appScrollbarSize: string; appScrollbarThumb: string;
  appScrollbarThumbBorder: string; appScrollbarThumbHover: string;
  appScrollbarTrack: string; appScrollbarRadius: string;

  // syntax (27 tokens)
  syntaxKeyword: string; syntaxString: string; syntaxNumber: string;
  syntaxConstant: string; syntaxComment: string; syntaxVariable: string;
  syntaxProperty: string; syntaxType: string; syntaxFunction: string;
  syntaxOperator: string; syntaxPunctuation: string; syntaxTag: string;
  syntaxAttribute: string; syntaxBoolean: string; syntaxNull: string;
  syntaxRegex: string; syntaxJsx: string; syntaxJsxAttribute: string;
  syntaxMarkdownHeading: string; syntaxMarkdownBold: string;
  syntaxMarkdownItalic: string; syntaxMarkdownStrikethrough: string;
  syntaxMarkdownLink: string; syntaxMarkdownLinkText: string;
  syntaxMarkdownCode: string; syntaxMarkdownList: string;
  syntaxMarkdownQuote: string;
}

export interface ThemeDefinition {
  id: string;
  name: string;
  type: 'light' | 'dark';
  tokens: ThemeTokens;
}
```

### `ThemeRegistry.applyTheme()` — future implementation shape
```ts
applyTheme(themeId: string): void {
  const theme = this.getTheme(themeId);
  if (!theme) return;
  const root = document.documentElement;
  root.classList.toggle('dark', theme.type === 'dark');
  for (const [key, value] of Object.entries(theme.tokens)) {
    root.style.setProperty(cssVarName(key), value);
    // cssVarName: syntaxKeyword → --syntax-keyword
  }
}
```

---

## Color Space

All new token values use **OKLCH** throughout. No hex, no hsl, no rgb.

Rationale:
- Consistent with all existing shadcn token declarations in `index.css`
- OKLCH is perceptually uniform — adjusting lightness doesn't shift perceived hue
- Enables clean light/dark derivation by scaling the L channel

---

## Verification

No unit tests required — this is a CSS variable migration. Verified visually and by grep:

1. **Light mode visual pass** — load app, toggle light mode, confirm editor/file-tree/terminal render correctly with no invisible or wrong-colored elements
2. **Dark mode visual pass** — same in dark mode
3. **Old variable grep** — must return zero results:
   ```sh
   grep -r -- '--color-text\|--color-hover\|--color-selected\|--color-primary-bg\|--color-secondary-bg\|--color-error\|--color-accent\|--app-font-family\|--color-border\|--color-text-light\|--color-secondary-bg' web/src
   ```
4. **Hex fallback grep** — must return zero results:
   ```sh
   grep -rE 'var\(--[^,)]+,\s*#' web/src
   ```

---

## Out of Scope

- Implementing `ThemeRegistry` beyond updating the type definitions
- Building theme presets (One Light, Dracula, etc.)
- Migrating any React component inline styles
- Changes to Tailwind config or `components.json`
