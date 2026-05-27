# Design: `--chrome-bg` Theme Token

**Date:** 2026-05-26  
**Branch:** enhancement/design-language  
**Status:** Approved

---

## Problem

The tab bar background and sidebar background are both "chrome" surfaces — fixed UI framing that sits at the edges of the editor. They should share a single, theme-overridable transparency value. Currently:

- Tab bar uses `bg-background` (fully opaque, no token)
- Sidebar uses `bg-primary-bg/95` (hardcoded Tailwind opacity on a runtime-injected variable)

Neither is connected to the shadcn/ui theme system, so there is no single knob a Crowbar theme can turn to control chrome-surface transparency.

---

## Goal

Add a single CSS design token — `--chrome-bg` — to the shadcn/ui theme in `index.css`. Both the tab bar and sidebar will reference this token. Any Crowbar theme can override it with any color + alpha value it chooses.

---

## Token Definition

### `index.css` — `@theme inline` block

```css
--color-chrome-bg: var(--chrome-bg);
```

This exposes `bg-chrome-bg` as a Tailwind utility class.

### `index.css` — `:root` (light mode)

```css
--chrome-bg: oklch(1 0 0 / 50%);
```

White at 50% opacity.

### `index.css` — `.dark`

```css
--chrome-bg: oklch(0.148 0.004 228.8 / 75%);
```

Dark blue-grey (matches `--background` in dark mode) at 75% opacity.

---

## Component Changes

### Tab bar — `web/src/features/tabs/components/tab-bar.tsx`

The main container div (currently `bg-background`) becomes:

```diff
- bg-background
+ bg-chrome-bg backdrop-blur-sm
```

`backdrop-blur-sm` is added to match the sidebar, so both chrome surfaces behave consistently when editor content scrolls behind them.

### Sidebar — `web/src/components/ui/sidebar.tsx`

`SidebarHeader` and `SidebarFooter` (currently `bg-primary-bg/95`) become:

```diff
- bg-primary-bg/95 backdrop-blur-sm   /* SidebarHeader */
- bg-primary-bg/95                    /* SidebarFooter */
+ bg-chrome-bg backdrop-blur-sm
+ bg-chrome-bg
```

This moves the sidebar off the runtime-injected `--primary-bg` variable and onto the shared shadcn/ui token. The colors are near-identical in the default dark theme; the switch also corrects the layering.

---

## Theme Override Point

Any Crowbar theme (present or future) can override `--chrome-bg` at any scope:

```css
/* In appearance-bootstrap.ts or a CSS theme file */
--chrome-bg: oklch(0.18 0.01 45 / 80%);   /* warm tinted sidebar */
--chrome-bg: oklch(0.12 0 0 / 60%);        /* deep glass */
```

Themes control both the base color and the alpha in a single value — no separate opacity knob needed.

---

## Out of Scope

- Other semi-transparent surfaces (popovers, tooltips, dropdowns) — they have their own tokens and are not chrome.
- Changing the blur radius — `backdrop-blur-sm` is a fixed design decision, not a token.
- Light-mode sidebar — the sidebar is not currently visible in light mode; if it becomes so, the `:root` value of `--chrome-bg` will apply automatically.

---

## Files Changed

| File | Change |
|------|--------|
| `web/src/index.css` | Add `--chrome-bg` to `:root`, `.dark`, and `@theme inline` |
| `web/src/features/tabs/components/tab-bar.tsx` | `bg-background` → `bg-chrome-bg backdrop-blur-sm` |
| `web/src/components/ui/sidebar.tsx` | `bg-primary-bg/95 [backdrop-blur-sm]` → `bg-chrome-bg [backdrop-blur-sm]` |
