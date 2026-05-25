# Athas → shadcn/ui Token Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace all undeclared Athas custom CSS variables with canonical shadcn/ui tokens, declare new custom tokens for editor-specific variables, and update `ThemeDefinition` to have a strongly-typed token surface.

**Architecture:** Token declarations are centralized in `index.css` (`:root`, `.dark`, `@theme inline`). CSS call-sites are updated in-place — shadcn equivalents renamed, editor-only vars kept but now properly declared. `ThemeDefinition` gains a typed `ThemeTokens` interface so the ThemeRegistry can eventually apply themes as a single flat object write.

**Tech Stack:** CSS custom properties, Tailwind CSS 4, OKLCH color space, TypeScript interfaces.

---

## File Map

| File | Change type |
|---|---|
| `web/src/extensions/themes/types.ts` | Add `ThemeTokens` interface, add `tokens?` field to `ThemeDefinition` |
| `web/src/index.css` | Add ~40 new token declarations to `:root`, `.dark`, `@theme inline` |
| `web/src/features/athas-editor/styles/token-theme.css` | Strip hex fallbacks from all `var()` calls; rename `--text` → `--foreground`, `--hover` → `--accent` |
| `web/src/features/editor/styles/token-theme.css` | Identical changes to the file above |
| `web/src/features/athas-editor/styles/overlay-editor.css` | Rename Athas vars to shadcn canonical; strip hex fallbacks; remove dead rgba fallback lines |
| `web/src/features/athas-editor/styles/overlay-card.css` | Rename 3 vars; no fallbacks to strip |
| `web/src/features/file-explorer/styles/file-explorer-tree.css` | Rename Athas vars to shadcn canonical |
| `web/src/features/terminal/styles/terminal.css` | Strip 2 redundant `rgba()` lines only |

> **Note:** `features/editor/styles/overlay-editor.css` does not exist — the spec assumed it did. Only the athas-editor copy exists.

---

## Task 1: Add ThemeTokens type

**Files:**
- Modify: `web/src/extensions/themes/types.ts`

- [ ] **Step 1: Replace the file content**

Current content of `web/src/extensions/themes/types.ts`:
```ts
// Stub
export interface ThemeDefinition {
  id: string
  name: string
  type?: "light" | "dark"
  isDark: boolean
  description?: string
  category?: "System" | "Light" | "Dark" | "Colorful"
  icon?: React.ReactNode
  colors?: Record<string, string>
  variables?: Record<string, string>
  /** Athas alias for variables */
  cssVariables?: Record<string, string>
  syntaxTokens?: Record<string, unknown>
}
```

Replace the entire file with:
```ts
// Stub

/**
 * Strongly-typed map of all CSS design tokens.
 * camelCase keys map to kebab-case CSS variable names:
 *   syntaxKeyword → --syntax-keyword
 *   appScrollbarThumb → --app-scrollbar-thumb
 */
export interface ThemeTokens {
  // shadcn base tokens
  background: string
  foreground: string
  card: string
  cardForeground: string
  popover: string
  popoverForeground: string
  primary: string
  primaryForeground: string
  secondary: string
  secondaryForeground: string
  muted: string
  mutedForeground: string
  accent: string
  accentForeground: string
  destructive: string
  border: string
  input: string
  ring: string

  // custom tokens — no shadcn equivalent
  warning: string
  info: string
  editorFontFamily: string
  uiTextXs: string
  uiTextSm: string
  appScrollbarSize: string
  appScrollbarThumb: string
  appScrollbarThumbBorder: string
  appScrollbarThumbHover: string
  appScrollbarTrack: string
  appScrollbarRadius: string

  // syntax highlighting tokens (27)
  syntaxKeyword: string
  syntaxString: string
  syntaxNumber: string
  syntaxConstant: string
  syntaxComment: string
  syntaxVariable: string
  syntaxProperty: string
  syntaxType: string
  syntaxFunction: string
  syntaxOperator: string
  syntaxPunctuation: string
  syntaxTag: string
  syntaxAttribute: string
  syntaxBoolean: string
  syntaxNull: string
  syntaxRegex: string
  syntaxJsx: string
  syntaxJsxAttribute: string
  syntaxMarkdownHeading: string
  syntaxMarkdownBold: string
  syntaxMarkdownItalic: string
  syntaxMarkdownStrikethrough: string
  syntaxMarkdownLink: string
  syntaxMarkdownLinkText: string
  syntaxMarkdownCode: string
  syntaxMarkdownList: string
  syntaxMarkdownQuote: string
}

export interface ThemeDefinition {
  id: string
  name: string
  type?: "light" | "dark"
  isDark: boolean
  description?: string
  category?: "System" | "Light" | "Dark" | "Colorful"
  icon?: React.ReactNode
  colors?: Record<string, string>
  variables?: Record<string, string>
  /** @deprecated Use `tokens` instead */
  cssVariables?: Record<string, string>
  syntaxTokens?: Record<string, unknown>
  /** Strongly-typed token map — preferred over cssVariables */
  tokens?: ThemeTokens
}
```

- [ ] **Step 2: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

Expected: no errors (the new fields are all optional or additive).

- [ ] **Step 3: Commit**

```bash
git add web/src/extensions/themes/types.ts
git commit -m "feat(theme): add ThemeTokens typed interface to ThemeDefinition"
```

---

## Task 2: Declare new tokens in index.css

**Files:**
- Modify: `web/src/index.css`

- [ ] **Step 1: Add new custom tokens to `:root`**

Inside the `:root { ... }` block (after the existing `--sidebar-ring` line), add:

```css
    /* custom tokens */
    --warning: oklch(0.75 0.15 85);
    --info: oklch(0.65 0.15 250);
    --editor-font-family: ui-monospace, 'Cascadia Code', 'Fira Code', monospace;
    --ui-text-xs: 0.6875rem;
    --ui-text-sm: 0.75rem;
    --app-scrollbar-size: 11px;
    --app-scrollbar-thumb: oklch(0.55 0 0 / 42%);
    --app-scrollbar-thumb-border: 3px solid transparent;
    --app-scrollbar-thumb-hover: oklch(0.55 0 0 / 58%);
    --app-scrollbar-track: transparent;
    --app-scrollbar-radius: 999px;

    /* syntax highlighting — light mode */
    --syntax-keyword: oklch(0.45 0.18 310);
    --syntax-string: oklch(0.42 0.14 140);
    --syntax-number: oklch(0.50 0.15 45);
    --syntax-constant: oklch(0.48 0.12 215);
    --syntax-comment: oklch(0.52 0.01 255);
    --syntax-variable: oklch(0.45 0.15 15);
    --syntax-property: oklch(0.45 0.15 270);
    --syntax-type: oklch(0.52 0.16 95);
    --syntax-function: oklch(0.45 0.15 270);
    --syntax-operator: oklch(0.48 0.12 215);
    --syntax-punctuation: oklch(0.25 0 0);
    --syntax-tag: oklch(0.45 0.15 15);
    --syntax-attribute: oklch(0.45 0.18 310);
    --syntax-boolean: oklch(0.42 0.22 15);
    --syntax-null: oklch(0.42 0.22 15);
    --syntax-regex: oklch(0.48 0.12 215);
    --syntax-jsx: oklch(0.48 0.12 215);
    --syntax-jsx-attribute: oklch(0.45 0.18 310);
    --syntax-markdown-heading: oklch(0.45 0.15 270);
    --syntax-markdown-bold: oklch(0.50 0.15 45);
    --syntax-markdown-italic: oklch(0.45 0.18 310);
    --syntax-markdown-strikethrough: oklch(0.45 0.15 15);
    --syntax-markdown-link: oklch(0.48 0.12 215);
    --syntax-markdown-link-text: oklch(0.45 0.15 270);
    --syntax-markdown-code: oklch(0.42 0.14 140);
    --syntax-markdown-list: oklch(0.45 0.18 310);
    --syntax-markdown-quote: oklch(0.52 0.01 255);
```

- [ ] **Step 2: Add dark overrides to `.dark`**

Inside the `.dark { ... }` block (after the existing `--sidebar-ring` line), add:

```css
    /* custom tokens — dark overrides */
    --warning: oklch(0.80 0.16 85);
    --info: oklch(0.72 0.15 250);

    /* syntax highlighting — dark mode */
    --syntax-keyword: oklch(0.68 0.18 310);
    --syntax-string: oklch(0.85 0.14 130);
    --syntax-number: oklch(0.75 0.15 45);
    --syntax-constant: oklch(0.85 0.09 215);
    --syntax-comment: oklch(0.56 0.01 255);
    --syntax-variable: oklch(0.70 0.15 15);
    --syntax-property: oklch(0.72 0.15 270);
    --syntax-type: oklch(0.87 0.16 95);
    --syntax-function: oklch(0.72 0.15 270);
    --syntax-operator: oklch(0.85 0.09 215);
    --syntax-punctuation: oklch(0.90 0 0);
    --syntax-tag: oklch(0.70 0.15 15);
    --syntax-attribute: oklch(0.68 0.18 310);
    --syntax-boolean: oklch(0.65 0.22 15);
    --syntax-null: oklch(0.65 0.22 15);
    --syntax-regex: oklch(0.85 0.09 215);
    --syntax-jsx: oklch(0.85 0.09 215);
    --syntax-jsx-attribute: oklch(0.68 0.18 310);
    --syntax-markdown-heading: oklch(0.72 0.15 270);
    --syntax-markdown-bold: oklch(0.75 0.15 45);
    --syntax-markdown-italic: oklch(0.68 0.18 310);
    --syntax-markdown-strikethrough: oklch(0.70 0.15 15);
    --syntax-markdown-link: oklch(0.85 0.09 215);
    --syntax-markdown-link-text: oklch(0.72 0.15 270);
    --syntax-markdown-code: oklch(0.85 0.14 130);
    --syntax-markdown-list: oklch(0.68 0.18 310);
    --syntax-markdown-quote: oklch(0.56 0.01 255);
```

Note: tokens whose values are the same in light and dark (scrollbar, font, text sizes) do NOT need a `.dark` entry — only color tokens that change.

- [ ] **Step 3: Register new color tokens in `@theme inline`**

Inside the `@theme inline { ... }` block (after the existing `--radius-4xl` line), add:

```css
    --color-warning: var(--warning);
    --color-info: var(--info);
    --font-editor: var(--editor-font-family);
```

This gives Tailwind access to `text-warning`, `bg-warning`, `text-info`, `bg-info`, and `font-editor` utility classes.

- [ ] **Step 4: Verify the file builds**

```bash
cd web && npx vite build --mode development 2>&1 | tail -10
```

Expected: build succeeds with no CSS errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/index.css
git commit -m "feat(theme): declare new custom tokens and syntax palette in index.css"
```

---

## Task 3: Migrate both token-theme.css files

Both files are identical in content. Apply the same changes to each.

**Files:**
- Modify: `web/src/features/athas-editor/styles/token-theme.css`
- Modify: `web/src/features/editor/styles/token-theme.css`

- [ ] **Step 1: Write the final content for both files**

Replace the entire content of **both** files with:

```css
/* Token theme for Tree-sitter based syntax highlighting */
/* Uses --syntax-* variables declared in index.css */

/* Keywords */
.token-keyword {
  color: var(--syntax-keyword);
  font-weight: 500;
}

/* Strings */
.token-string {
  color: var(--syntax-string);
}

/* Numbers */
.token-number {
  color: var(--syntax-number);
}

/* Constants */
.token-constant {
  color: var(--syntax-constant);
}

/* Comments */
.token-comment {
  color: var(--syntax-comment);
  font-style: italic;
}

/* Variables */
.token-variable {
  color: var(--syntax-variable);
}

/* Properties */
.token-property {
  color: var(--syntax-property);
}

/* Types */
.token-type {
  color: var(--syntax-type);
}

/* Functions */
.token-function {
  color: var(--syntax-function);
}

/* Operators */
.token-operator {
  color: var(--syntax-operator);
}

/* Punctuation */
.token-punctuation {
  color: var(--syntax-punctuation);
}

/* Tags (HTML/XML) */
.token-tag {
  color: var(--syntax-tag);
}

/* Attributes */
.token-attribute {
  color: var(--syntax-attribute);
}

/* Default text */
.token-text {
  color: var(--foreground);
}

/* Identifiers */
.token-identifier {
  color: var(--foreground);
}

/* Booleans */
.token-boolean {
  color: var(--syntax-boolean);
}

/* Null/undefined */
.token-null {
  color: var(--syntax-null);
}

/* Regular expressions */
.token-regex {
  color: var(--syntax-regex);
}

/* JSX */
.token-jsx {
  color: var(--syntax-jsx);
}

.token-jsx-attribute {
  color: var(--syntax-jsx-attribute);
}

.token-jsx-text {
  color: var(--foreground);
}

/* Markdown */
.token-markdown-heading {
  color: var(--syntax-markdown-heading);
  font-weight: 700;
}

.token-markdown-bold {
  color: var(--syntax-markdown-bold);
  font-weight: 700;
}

.token-markdown-italic {
  color: var(--syntax-markdown-italic);
  font-style: italic;
}

.token-markdown-strikethrough {
  color: var(--syntax-markdown-strikethrough);
  text-decoration: line-through;
}

.token-markdown-link {
  color: var(--syntax-markdown-link);
  text-decoration: underline;
}

.token-markdown-link-text {
  color: var(--syntax-markdown-link-text);
}

.token-markdown-code {
  color: var(--syntax-markdown-code);
  background-color: color-mix(in srgb, var(--accent) 15%, transparent);
  border-radius: 3px;
  padding: 2px 4px;
}

.token-markdown-list {
  color: var(--syntax-markdown-list);
  font-weight: 600;
}

.token-markdown-quote {
  color: var(--syntax-markdown-quote);
  font-style: italic;
}
```

What changed vs the original:
- All `var(--syntax-*, #hexvalue)` → `var(--syntax-*)` (fallbacks removed, tokens now declared)
- `var(--text)` → `var(--foreground)` (3 occurrences: `.token-text`, `.token-identifier`, `.token-jsx-text`)
- `var(--syntax-punctuation, var(--text))` → `var(--syntax-punctuation)` (fallback was another undeclared var)
- `var(--hover, rgba(255, 255, 255, 0.1))` → `color-mix(in srgb, var(--accent) 15%, transparent)` in `.token-markdown-code`

- [ ] **Step 2: Verify no old variable names remain in these files**

```bash
grep -n '\(--text\b\|--hover\b\|#[0-9a-fA-F]\{3,6\}\)' \
  web/src/features/athas-editor/styles/token-theme.css \
  web/src/features/editor/styles/token-theme.css
```

Expected: no output (zero matches).

- [ ] **Step 3: Commit**

```bash
git add \
  web/src/features/athas-editor/styles/token-theme.css \
  web/src/features/editor/styles/token-theme.css
git commit -m "feat(theme): migrate token-theme.css — strip hex fallbacks, rename text/hover vars"
```

---

## Task 4: Migrate overlay-editor.css

**Files:**
- Modify: `web/src/features/athas-editor/styles/overlay-editor.css`

This file has the most changes. Apply each replacement in order.

- [ ] **Step 1: Update `.highlight-layer` default text color (line 39)**

Before:
```css
  color: var(--color-text, var(--text, #d4d4d4));
```
After:
```css
  color: var(--foreground);
```

- [ ] **Step 2: Update `.input-layer` caret color (line 83)**

Before:
```css
  caret-color: var(--text, #d4d4d4);
```
After:
```css
  caret-color: var(--foreground);
```

- [ ] **Step 3: Update Firefox scrollbar declaration (line 108)**

Before:
```css
    scrollbar-color: var(--app-scrollbar-thumb, rgba(128, 128, 128, 0.42)) transparent;
```
After:
```css
    scrollbar-color: var(--app-scrollbar-thumb) transparent;
```

- [ ] **Step 4: Update WebKit scrollbar size (lines 125–126)**

Before:
```css
  width: var(--app-scrollbar-size, 11px) !important;
  height: var(--app-scrollbar-size, 11px) !important;
```
After:
```css
  width: var(--app-scrollbar-size) !important;
  height: var(--app-scrollbar-size) !important;
```

- [ ] **Step 5: Update scrollbar thumb block (lines 138–145)**

Before:
```css
.input-layer::-webkit-scrollbar-thumb {
  -webkit-appearance: none !important;
  background-color: rgba(128, 128, 128, 0.42) !important;
  background-color: var(--app-scrollbar-thumb, rgba(128, 128, 128, 0.42)) !important;
  border: var(--app-scrollbar-thumb-border, 3px solid transparent) !important;
  border-radius: var(--app-scrollbar-radius, 999px) !important;
  background-clip: content-box !important;
  transition: background-color 0.18s ease;
  cursor: default;
}
```
After:
```css
.input-layer::-webkit-scrollbar-thumb {
  -webkit-appearance: none !important;
  background-color: var(--app-scrollbar-thumb) !important;
  border: var(--app-scrollbar-thumb-border) !important;
  border-radius: var(--app-scrollbar-radius) !important;
  background-clip: content-box !important;
  transition: background-color 0.18s ease;
  cursor: default;
}
```

- [ ] **Step 6: Update scrollbar thumb hover block (lines 148–155)**

Before:
```css
.input-layer:hover::-webkit-scrollbar-thumb,
.input-layer:focus::-webkit-scrollbar-thumb,
.input-layer:focus-within::-webkit-scrollbar-thumb {
  background-color: rgba(128, 128, 128, 0.58) !important;
  background-color: var(--app-scrollbar-thumb-hover, rgba(128, 128, 128, 0.58)) !important;
  border: 2px solid transparent !important;
  background-clip: content-box !important;
}
```
After:
```css
.input-layer:hover::-webkit-scrollbar-thumb,
.input-layer:focus::-webkit-scrollbar-thumb,
.input-layer:focus-within::-webkit-scrollbar-thumb {
  background-color: var(--app-scrollbar-thumb-hover) !important;
  border: 2px solid transparent !important;
  background-clip: content-box !important;
}
```

- [ ] **Step 7: Update scrollbar thumb:hover block (lines 157–162)**

Before:
```css
.input-layer::-webkit-scrollbar-thumb:hover {
  background-color: rgba(160, 160, 160, 0.7) !important;
  background-color: var(--app-scrollbar-thumb-hover, rgba(160, 160, 160, 0.7)) !important;
  border: 2px solid transparent !important;
  background-clip: content-box !important;
}
```
After:
```css
.input-layer::-webkit-scrollbar-thumb:hover {
  background-color: var(--app-scrollbar-thumb-hover) !important;
  border: 2px solid transparent !important;
  background-clip: content-box !important;
}
```

- [ ] **Step 8: Update native selection blocks (lines 171–186)**

Before:
```css
.input-layer.native-selection::selection {
  background: rgba(106, 155, 204, 0.42);
  background: color-mix(in srgb, var(--accent, #6a9bcc) 44%, transparent);
  color: transparent;
}

.input-layer.native-selection::-moz-selection {
  background: rgba(106, 155, 204, 0.42);
  background: color-mix(in srgb, var(--accent, #6a9bcc) 44%, transparent);
  color: transparent;
}
```
After:
```css
.input-layer.native-selection::selection {
  background: color-mix(in srgb, var(--accent) 44%, transparent);
  color: transparent;
}

.input-layer.native-selection::-moz-selection {
  background: color-mix(in srgb, var(--accent) 44%, transparent);
  color: transparent;
}
```

- [ ] **Step 9: Update `.editor-bracket-match` border (line ~198)**

Before:
```css
  border-bottom: 1px solid color-mix(in srgb, var(--accent, #6a9bcc) 72%, transparent);
```
After:
```css
  border-bottom: 1px solid color-mix(in srgb, var(--accent) 72%, transparent);
```

- [ ] **Step 10: Update `.editor-bracket-match-unmatched` border (line ~204)**

Before:
```css
  border-bottom-color: color-mix(in srgb, var(--color-error, #ef4444) 72%, transparent);
```
After:
```css
  border-bottom-color: color-mix(in srgb, var(--destructive) 72%, transparent);
```

- [ ] **Step 11: Update `.editor-word-highlight` background (line ~213)**

Before:
```css
  background: color-mix(in srgb, var(--color-text, #d4d4d4) 10%, transparent);
```
After:
```css
  background: color-mix(in srgb, var(--foreground) 10%, transparent);
```

- [ ] **Step 12: Update `.editor-word-highlight-current` border (line ~217)**

Before:
```css
  border: 1px solid color-mix(in srgb, var(--color-text, #d4d4d4) 18%, transparent);
```
After:
```css
  border: 1px solid color-mix(in srgb, var(--foreground) 18%, transparent);
```

- [ ] **Step 13: Update `.editor-current-line` background (line ~226)**

Before:
```css
  background: color-mix(in srgb, var(--color-text, #d4d4d4) 5%, transparent);
```
After:
```css
  background: color-mix(in srgb, var(--foreground) 5%, transparent);
```

- [ ] **Step 14: Update `.editor-indent-guide` background (line ~231)**

Before:
```css
  background: color-mix(in srgb, var(--color-border, #8a8f98) 42%, transparent);
```
After:
```css
  background: color-mix(in srgb, var(--border) 42%, transparent);
```

- [ ] **Step 15: Update `.editor-indent-guide-active` background (line ~234)**

Before:
```css
  background: color-mix(in srgb, var(--accent, #6a9bcc) 52%, transparent);
```
After:
```css
  background: color-mix(in srgb, var(--accent) 52%, transparent);
```

- [ ] **Step 16: Update `.editor-visible-whitespace::after` color (line ~248)**

Before:
```css
  color: color-mix(in srgb, var(--color-text-lighter, #8a8f98) 56%, transparent);
```
After:
```css
  color: color-mix(in srgb, var(--muted-foreground) 56%, transparent);
```

- [ ] **Step 17: Update `.inlay-hint` border, background, and color (lines ~276–279)**

Before:
```css
  border: 1px solid color-mix(in srgb, var(--color-border, #8a8f98) 42%, transparent);
  border-radius: 4px;
  background: color-mix(in srgb, var(--color-primary-bg, #ffffff) 94%, transparent);
  color: var(--color-text-lighter, #8a8f98);
```
After:
```css
  border: 1px solid color-mix(in srgb, var(--border) 42%, transparent);
  border-radius: 4px;
  background: color-mix(in srgb, var(--background) 94%, transparent);
  color: var(--muted-foreground);
```

- [ ] **Step 18: Update diagnostic decoration colors (lines ~293–303)**

Before:
```css
.diagnostic-error {
  text-decoration-color: var(--error, #f85149);
}

.diagnostic-warning {
  text-decoration-color: var(--warning, #d29922);
}

.diagnostic-info {
  text-decoration-color: var(--info, #58a6ff);
}
```
After:
```css
.diagnostic-error {
  text-decoration-color: var(--destructive);
}

.diagnostic-warning {
  text-decoration-color: var(--warning);
}

.diagnostic-info {
  text-decoration-color: var(--info);
}
```

- [ ] **Step 19: Update `.editor-selection-box` and remove its `@supports` fallback block (lines ~305–315)**

Before:
```css
.editor-selection-box {
  background-color: rgba(106, 155, 204, 0.42);
  background-color: color-mix(in srgb, var(--accent, #6a9bcc) 44%, transparent);
  border-radius: 4px;
}

@supports not (background-color: color-mix(in srgb, #6a9bcc 44%, transparent)) {
  .editor-selection-box {
    background-color: rgba(106, 155, 204, 0.42);
  }
}
```
After:
```css
.editor-selection-box {
  background-color: color-mix(in srgb, var(--accent) 44%, transparent);
  border-radius: 4px;
}
```

The `@supports not` block was a progressive enhancement for browsers without `color-mix` support. Since we no longer hardcode a hex color as the base value, and `color-mix` is globally supported at >93%, the fallback block is removed.

- [ ] **Step 20: Verify no Athas vars or hex fallbacks remain**

```bash
grep -n -- \
  '--color-text\|--color-border\|--color-error\|--color-text-lighter\|--color-primary-bg\|--accent,\|--error,\|--warning,\|--info,\|--text,' \
  web/src/features/athas-editor/styles/overlay-editor.css
```

Expected: no output.

```bash
grep -nE 'var\(--[^,)]+,\s*(rgba|#)' \
  web/src/features/athas-editor/styles/overlay-editor.css
```

Expected: no output.

- [ ] **Step 21: Commit**

```bash
git add web/src/features/athas-editor/styles/overlay-editor.css
git commit -m "feat(theme): migrate overlay-editor.css — rename Athas vars, strip hex fallbacks"
```

---

## Task 5: Migrate overlay-card.css

**Files:**
- Modify: `web/src/features/athas-editor/styles/overlay-card.css`

This file is small — 3 variable renames, no fallbacks to strip.

- [ ] **Step 1: Apply all three renames**

Replace the entire file with:

```css
/* Shared styling for all editor overlay cards (hover tooltip, git blame, etc.) */
.editor-overlay-card {
  border-radius: 12px;
  border: 1px solid color-mix(in srgb, var(--border) 85%, transparent);
  background: color-mix(in srgb, var(--card) 96%, transparent);
  box-shadow:
    0 10px 28px rgba(0, 0, 0, 0.18),
    inset 0 1px 0 color-mix(in srgb, var(--accent) 42%, transparent);
  overflow: hidden;
}

@supports ((-webkit-backdrop-filter: blur(10px)) or (backdrop-filter: blur(10px))) {
  .editor-overlay-card {
    -webkit-backdrop-filter: blur(10px);
    backdrop-filter: blur(10px);
    background: color-mix(in srgb, var(--card) 90%, transparent);
  }
}
```

Changes from original:
- `var(--color-border)` → `var(--border)`
- `var(--color-secondary-bg)` (×2) → `var(--card)`
- `var(--color-hover)` → `var(--accent)`

- [ ] **Step 2: Verify**

```bash
grep -n -- '--color-' web/src/features/athas-editor/styles/overlay-card.css
```

Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add web/src/features/athas-editor/styles/overlay-card.css
git commit -m "feat(theme): migrate overlay-card.css — rename Athas vars to shadcn canonical"
```

---

## Task 6: Migrate file-explorer-tree.css

**Files:**
- Modify: `web/src/features/file-explorer/styles/file-explorer-tree.css`

- [ ] **Step 1: Update font-family (line 15)**

Before:
```css
  font-family: var(--app-font-family);
```
After:
```css
  font-family: var(--font-sans);
```

- [ ] **Step 2: Update `.bg-selected` border color (line ~51)**

Before:
```css
  border-color: color-mix(in srgb, var(--color-border) 78%, transparent) !important;
```
After:
```css
  border-color: color-mix(in srgb, var(--border) 78%, transparent) !important;
```

- [ ] **Step 3: Update focus-visible border color (line ~56)**

Before:
```css
  border-color: color-mix(in srgb, var(--color-accent) 42%, var(--color-border)) !important;
```
After:
```css
  border-color: color-mix(in srgb, var(--accent) 42%, var(--border)) !important;
```

- [ ] **Step 4: Update `.file-tree-item` local vars (lines ~61–63)**

Before:
```css
  --file-tree-hover-bg: color-mix(in srgb, var(--color-hover) 68%, transparent);
  --file-tree-guide-icon-offset: 7px;
  --tree-guide-color: color-mix(in srgb, var(--color-text-lighter) 18%, transparent);
```
After:
```css
  --file-tree-hover-bg: color-mix(in srgb, var(--accent) 68%, transparent);
  --file-tree-guide-icon-offset: 7px;
  --tree-guide-color: color-mix(in srgb, var(--muted-foreground) 18%, transparent);
```

- [ ] **Step 5: Update selected item background (line ~81)**

Before:
```css
  background-color: var(--color-selected);
```
After:
```css
  background-color: var(--accent);
```

- [ ] **Step 6: Update search highlight background (line ~100)**

Before:
```css
  background-color: color-mix(in srgb, var(--color-accent) 30%, transparent);
```
After:
```css
  background-color: color-mix(in srgb, var(--accent) 30%, transparent);
```

- [ ] **Step 7: Update search match background (line ~106)**

Before:
```css
  background-color: color-mix(in srgb, var(--color-accent) 7%, transparent) !important;
```
After:
```css
  background-color: color-mix(in srgb, var(--accent) 7%, transparent) !important;
```

- [ ] **Step 8: Update sticky ancestor stack (lines ~153–154)**

Before:
```css
  background-color: var(--color-primary-bg);
  box-shadow: 0 -1px 0 0 var(--color-primary-bg);
```
After:
```css
  background-color: var(--background);
  box-shadow: 0 -1px 0 0 var(--background);
```

- [ ] **Step 9: Update sticky ancestor stack ::after border (line ~163)**

Before:
```css
  background-color: color-mix(in srgb, var(--color-border) 58%, transparent);
```
After:
```css
  background-color: color-mix(in srgb, var(--border) 58%, transparent);
```

- [ ] **Step 10: Verify no Athas vars remain**

```bash
grep -n -- '--color-hover\|--color-selected\|--color-border\|--color-accent\|--color-text\|--color-primary-bg\|--app-font-family' \
  web/src/features/file-explorer/styles/file-explorer-tree.css
```

Expected: no output.

- [ ] **Step 11: Commit**

```bash
git add web/src/features/file-explorer/styles/file-explorer-tree.css
git commit -m "feat(theme): migrate file-explorer-tree.css — rename Athas vars to shadcn canonical"
```

---

## Task 7: Migrate terminal.css

**Files:**
- Modify: `web/src/features/terminal/styles/terminal.css`

This file already uses `--app-scrollbar-*` vars without fallbacks everywhere except two `rgba()` progressive-enhancement lines that preceded them. Strip those two lines.

- [ ] **Step 1: Remove redundant rgba fallback in thumb block (line 59)**

Before:
```css
.xterm-container .xterm-viewport::-webkit-scrollbar-thumb {
  -webkit-appearance: none !important;
  min-height: 36px !important;
  border: var(--app-scrollbar-thumb-border) !important;
  border-radius: var(--app-scrollbar-radius) !important;
  background-color: rgba(120, 120, 120, 0.42) !important;
  background-color: var(--app-scrollbar-thumb) !important;
  background-clip: content-box !important;
}
```
After:
```css
.xterm-container .xterm-viewport::-webkit-scrollbar-thumb {
  -webkit-appearance: none !important;
  min-height: 36px !important;
  border: var(--app-scrollbar-thumb-border) !important;
  border-radius: var(--app-scrollbar-radius) !important;
  background-color: var(--app-scrollbar-thumb) !important;
  background-clip: content-box !important;
}
```

- [ ] **Step 2: Remove redundant rgba fallback in thumb:hover block (line 65)**

Before:
```css
.xterm-container .xterm-viewport::-webkit-scrollbar-thumb:hover {
  background-color: rgba(105, 105, 105, 0.62) !important;
  background-color: var(--app-scrollbar-thumb-hover) !important;
  background-clip: content-box !important;
}
```
After:
```css
.xterm-container .xterm-viewport::-webkit-scrollbar-thumb:hover {
  background-color: var(--app-scrollbar-thumb-hover) !important;
  background-clip: content-box !important;
}
```

- [ ] **Step 3: Verify no rgba fallback lines remain**

```bash
grep -n 'rgba' web/src/features/terminal/styles/terminal.css
```

Expected: no output.

- [ ] **Step 4: Commit**

```bash
git add web/src/features/terminal/styles/terminal.css
git commit -m "feat(theme): migrate terminal.css — strip redundant rgba scrollbar fallbacks"
```

---

## Task 8: Verification pass

- [ ] **Step 1: Grep for all old Athas variable names — must be zero**

```bash
grep -rn -- \
  '--color-text\|--color-hover\|--color-selected\|--color-primary-bg\|--color-secondary-bg\|--color-error\|--color-accent\|--color-border\|--color-text-light\|--app-font-family' \
  web/src/
```

Expected: no output.

- [ ] **Step 2: Grep for hex fallbacks in var() calls — must be zero**

```bash
grep -rnE 'var\(--[^,)]+,\s*#' web/src/
```

Expected: no output.

- [ ] **Step 3: Grep for bare undeclared `--text` and `--error` usage — must be zero**

```bash
grep -rnE 'var\(--text[,)]|var\(--error[,)]|var\(--hover[,)]' web/src/
```

Expected: no output.

- [ ] **Step 4: Build check**

```bash
cd web && npx vite build --mode development 2>&1 | tail -5
```

Expected: `✓ built in` with no errors.

- [ ] **Step 5: Visual check — dark mode**

Start the dev server (`cd web && npm run dev`), open the app in a browser, confirm:
- Editor syntax highlighting renders with colored tokens (not invisible/white)
- File tree hover and selected states show the accent color
- Editor overlay cards (hover tooltips) have the correct backdrop
- Scrollbars visible in both the editor and terminal panels

- [ ] **Step 6: Visual check — light mode**

Toggle to light mode, confirm:
- Syntax highlighting uses darker, readable colors (not the dark-mode palette)
- All backgrounds, borders, and text are legible
- No invisible elements (any invisible text = missing token declaration)

- [ ] **Step 7: Final commit if any fixups were needed**

```bash
git add -A
git commit -m "fix(theme): visual fixups from verification pass"
```
```
