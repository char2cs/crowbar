# Cross/UI Design Language Adoption

**Date:** 2026-05-28
**Branch:** enhancement/design-language
**Status:** Approved

## Goal

Adopt Cross/UI's visual design language across the entire Crowbar web app — fonts, color tokens, component internals — using Cross/UI's own components installed via their shadcn registry. The editor's visual layer is kept separate and architecturally isolated from the UI theme.

## Context

- Cross/UI is a shadcn-style library (source installed locally, you own the code) built on `@base-ui/react` v1.5 — already a Crowbar dependency.
- Their font is **Cal Sans UI** (variable, body/UI) and **Cal Sans Regular** (headings) — both downloaded to `web/public/fonts/`.
- Their theme is a standard CSS variable system compatible with Tailwind v4's `@theme inline`.
- This branch already has `button.tsx` adapted to Cross/UI internals and the five semantic color tokens added to `index.css`. That work is consistent with this spec and stays.
- The editor uses JetBrains Mono and has ~30 syntax highlight tokens. These move into `editor-theme.css` as part of this spec — they are not touched visually, only relocated.

## Theme Architecture

The current `web/src/index.css` mixes UI theme tokens, editor tokens, font imports, and Tailwind config into one file. This spec splits it into three files:

```
web/src/index.css          — imports only: Tailwind, tw-animate-css, theme.css, editor-theme.css
web/src/styles/theme.css   — Cross/UI design tokens (colors, fonts, radius, sidebar)
web/src/styles/editor-theme.css  — editor tokens that reference theme.css vars + standalone syntax tokens
```

### `theme.css`

Contains everything Cross/UI owns:

- `@font-face` declarations for Cal Sans UI and Cal Sans Regular (served from `web/public/fonts/`)
- `@theme inline` block mapping Tailwind color utilities to CSS vars — updates `--font-sans` to `'CalSansUI'` and `--font-heading` to `'CalSans'`, removes old IBM Plex Sans references
- `:root` block with Cross/UI light-mode values (exact values from `https://coss.com/ui/docs/styling`)
- `.dark` block with Cross/UI dark-mode values

Cross/UI's exact token values:

**`:root` (light)**
```css
--radius: .625rem;
--background: var(--color-white);
--foreground: var(--color-neutral-800);
--card: var(--color-white);
--card-foreground: var(--color-neutral-800);
--popover: var(--color-white);
--popover-foreground: var(--color-neutral-800);
--primary: var(--color-neutral-800);
--primary-foreground: var(--color-neutral-50);
--secondary: #0000000a;
--secondary-foreground: var(--color-neutral-800);
--muted: #0000000a;
--muted-foreground: #686868;
--accent: #0000000a;
--accent-foreground: var(--color-neutral-800);
--destructive: var(--color-red-500);
--destructive-foreground: var(--color-red-700);
--info: var(--color-blue-500);
--info-foreground: var(--color-blue-700);
--success: var(--color-emerald-500);
--success-foreground: var(--color-emerald-700);
--warning: var(--color-amber-500);
--warning-foreground: var(--color-amber-700);
--border: #00000014;
--input: #0000001a;
--ring: var(--color-neutral-400);
--sidebar: var(--color-neutral-50);
--sidebar-foreground: #262626;
--sidebar-primary: var(--color-neutral-800);
--sidebar-primary-foreground: var(--color-neutral-50);
--sidebar-accent: #0000000a;
--sidebar-accent-foreground: var(--color-neutral-800);
--sidebar-border: #0000000f;
--sidebar-ring: var(--color-neutral-400);
--code: var(--color-white);
--code-foreground: var(--foreground);
--code-highlight: #0000000a;
```

**`.dark`**
```css
--background: #141414;
--foreground: var(--color-neutral-100);
--card: var(--background);
--card-foreground: var(--color-neutral-100);
--popover: var(--background);
--popover-foreground: var(--color-neutral-100);
--primary: var(--color-neutral-100);
--primary-foreground: var(--color-neutral-800);
--secondary: #ffffff0a;
--secondary-foreground: var(--color-neutral-100);
--muted: #ffffff0a;
--muted-foreground: #818181;
--accent: #ffffff0a;
--accent-foreground: var(--color-neutral-100);
--destructive: #fb414a;
--destructive-foreground: var(--color-red-400);
--info: var(--color-blue-500);
--info-foreground: var(--color-blue-400);
--success: var(--color-emerald-500);
--success-foreground: var(--color-emerald-400);
--warning: var(--color-amber-500);
--warning-foreground: var(--color-amber-400);
--border: #ffffff0f;
--input: #ffffff14;
--ring: var(--color-neutral-500);
--sidebar: #111;
--sidebar-foreground: #f5f5f5;
--sidebar-primary: var(--color-neutral-100);
--sidebar-primary-foreground: var(--color-neutral-800);
--sidebar-accent: #ffffff0a;
--sidebar-accent-foreground: var(--color-neutral-100);
--sidebar-border: #ffffff0d;
--sidebar-ring: var(--color-neutral-400);
--code: var(--background);
--code-foreground: var(--foreground);
--code-highlight: #ffffff0a;
```

Crowbar-specific tokens that move into `theme.css` (not in Cross/UI, but still UI-layer):
- `--chrome-bg` — app-specific glass chrome
- `--app-ui-scale`, `--ui-text-xs/sm/base` — UI scaling system
- `--app-scrollbar-*` — scrollbar sizing and colors

### `editor-theme.css`

Contains only editor-layer tokens. Structural tokens reference `theme.css` vars so the editor automatically tracks theme changes. Syntax tokens are standalone (no Cross/UI counterpart).

```css
@import "@fontsource-variable/jetbrains-mono";

:root {
  --editor-font-family: ui-monospace, 'JetBrains Mono Variable', monospace;

  /* Syntax highlighting — light. Values copied verbatim from current index.css --syntax-* block. */
  --syntax-keyword: oklch(0.45 0.18 310);
  --syntax-string: oklch(0.42 0.14 140);
  --syntax-number: oklch(0.50 0.15 45);
  --syntax-constant: oklch(0.48 0.12 215);
  --syntax-comment: oklch(0.52 0.01 255);
  --syntax-variable: oklch(0.45 0.15 15);
  --syntax-property: oklch(0.45 0.15 270);
  --syntax-type: oklch(0.52 0.16 95);
  --syntax-function: oklch(0.45 0.15 220);
  --syntax-operator: oklch(0.48 0.12 215);
  --syntax-punctuation: oklch(0.25 0 0);
  --syntax-tag: oklch(0.45 0.15 15);
  --syntax-attribute: oklch(0.45 0.18 310);
  --syntax-boolean: oklch(0.42 0.22 15);
  --syntax-null: oklch(0.42 0.22 15);
  --syntax-regex: oklch(0.48 0.12 215);
  --syntax-jsx: oklch(0.48 0.12 215);
  --syntax-jsx-attribute: var(--syntax-attribute);
  --syntax-markdown-heading: oklch(0.45 0.15 270);
  --syntax-markdown-bold: oklch(0.50 0.15 45);
  --syntax-markdown-italic: oklch(0.45 0.18 310);
  --syntax-markdown-strikethrough: oklch(0.45 0.15 15);
  --syntax-markdown-link: oklch(0.48 0.12 215);
  --syntax-markdown-link-text: oklch(0.45 0.15 270);
  --syntax-markdown-code: oklch(0.42 0.14 140);
  --syntax-markdown-list: oklch(0.45 0.18 310);
  --syntax-markdown-quote: oklch(0.52 0.01 255);
}

.dark {
  /* Syntax highlighting — dark overrides. Values copied verbatim from current index.css .dark --syntax-* block. */
  --syntax-keyword: oklch(0.68 0.18 310);
  --syntax-string: oklch(0.85 0.14 130);
  --syntax-number: oklch(0.75 0.15 45);
  --syntax-constant: oklch(0.85 0.09 215);
  --syntax-comment: oklch(0.62 0.01 255);
  --syntax-variable: oklch(0.70 0.15 15);
  --syntax-property: oklch(0.72 0.15 270);
  --syntax-type: oklch(0.87 0.16 95);
  --syntax-function: oklch(0.72 0.15 220);
  --syntax-operator: oklch(0.85 0.09 215);
  --syntax-punctuation: oklch(0.90 0 0);
  --syntax-tag: oklch(0.70 0.15 15);
  --syntax-attribute: oklch(0.68 0.18 310);
  --syntax-boolean: oklch(0.65 0.22 15);
  --syntax-null: oklch(0.65 0.22 15);
  --syntax-regex: oklch(0.85 0.09 215);
  --syntax-jsx: oklch(0.85 0.09 215);
  --syntax-jsx-attribute: var(--syntax-attribute);
  --syntax-markdown-heading: oklch(0.72 0.15 270);
  --syntax-markdown-bold: oklch(0.75 0.15 45);
  --syntax-markdown-italic: oklch(0.68 0.18 310);
  --syntax-markdown-strikethrough: oklch(0.70 0.15 15);
  --syntax-markdown-link: oklch(0.85 0.09 215);
  --syntax-markdown-link-text: oklch(0.72 0.15 270);
  --syntax-markdown-code: oklch(0.85 0.14 130);
  --syntax-markdown-list: oklch(0.68 0.18 310);
  --syntax-markdown-quote: oklch(0.56 0.01 255);
}
```

### `index.css`

After the split, becomes import-only:

```css
@import "tailwindcss";
@import "tw-animate-css";
@import "./styles/theme.css";
@import "./styles/editor-theme.css";
```

## Phase 2 — Component Migration

Install each Cross/UI component via their shadcn registry CLI, then graft Crowbar-specific extensions back. Run `vitest` after each component and fix any assertion failures before proceeding.

Registry URL: `https://coss.com/ui/r` (verify exact CLI invocation from `https://coss.com/ui/docs/get-started` before implementation — the pattern is `pnpm dlx shadcn@latest add <component> --registry-url https://coss.com/ui/r`).

### Group A — Install as-is (no Crowbar API to preserve)

accordion, alert, alert-dialog, autocomplete, avatar, badge, breadcrumb, calendar, card, checkbox, checkbox-group, collapsible, command, combobox, drawer, empty, field, fieldset, form, frame, group, hover-card, kbd, menu, meter, number-field, otp-field, pagination, popover, preview-card, progress, radio-group, scroll-area, separator, sheet, skeleton, slider, spinner, table, toggle, toggle-group, toolbar

### Group B — Install + graft Crowbar extensions

| Component | Crowbar extension to preserve |
|---|---|
| `button.tsx` | `active`, `loading`, `tooltip`, `compact`, `shortcut`, `commandId`, `tooltipSide` props — **already done on this branch** |
| `tabs.tsx` | Standalone `Tab` component used in `tab-bar-item.tsx` and `terminal-tab-bar-item.tsx` |
| `input.tsx` | `leftIcon` prop, variant compat |
| `switch.tsx` | Size variants, `onChange` |
| `tooltip.tsx` | Compound `<Tooltip content="..." />` API |
| `select.tsx` | Existing call sites using Crowbar-specific props |
| `dialog.tsx` | Existing call sites |
| `dropdown-menu.tsx` | Existing call sites |
| `context-menu.tsx` | Existing call sites |
| `textarea.tsx` | Existing call sites |
| `label.tsx` | Existing call sites |
| `resizable.tsx` | Existing call sites |
| `sidebar.tsx` | Existing call sites |
| `sonner.tsx` | App-specific toast config |

### Group C — Keep as-is (no Cross/UI counterpart)

`pane.tsx`, `sidebar-tree.tsx`, `tree-row.tsx`, `number-input.tsx`, `search.tsx`, `keybinding.tsx`, `primitive-dialog-service.tsx`, `dropdown.tsx`, `toast.tsx`

## Phase 3 — App Sweep

After Phase 2, grep `web/src/` for raw color values outside the two theme files:

```bash
grep -rn 'oklch\|#[0-9a-fA-F]\{3,6\}\b\|hsl(' web/src \
  --include='*.tsx' --include='*.ts' --include='*.css' \
  --exclude-dir='__tests__' \
  | grep -v 'styles/theme.css\|styles/editor-theme.css\|token-theme.css\|monaco-editor.css'
```

Each match is reviewed and replaced with the appropriate `var(--token)` reference. The output of this grep must be empty before Phase 3 is considered complete.

## Out of Scope

These files are not touched by this spec:

| File | Reason |
|---|---|
| `web/src/features/editor/styles/token-theme.css` | Monaco syntax theme — fully editor-layer |
| `web/src/features/editor/styles/monaco-editor.css` | Monaco chrome styling |
| `web/src/features/editor/styles/overlay-card.css` | Editor overlay |
| `web/src/features/terminal/styles/terminal.css` | xterm terminal styles |
| `web/src/features/editor/completion/completion-dropdown.css` | Editor completion UI |
| `web/src/features/editor/lsp/hover-tooltip.css` | LSP hover UI |
| `web/src/features/editor/markdown/styles.css` | Markdown rendering |

## Constraints

- All existing Crowbar call sites continue to compile without changes after each component migration.
- The `Tab` standalone component must remain exported from `tabs.tsx`.
- `--editor-font-family` always resolves to JetBrains Mono — never Cal Sans UI.
- No raw hex, oklch, or hsl values outside `theme.css` and `editor-theme.css` when Phase 3 is complete.
- The two theme files have a strict one-way dependency: `editor-theme.css` may reference vars declared in `theme.css`, but `theme.css` must never reference vars from `editor-theme.css`.
