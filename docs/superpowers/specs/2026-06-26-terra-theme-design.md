# Terra Theme Design

**Date:** 2026-06-26
**Status:** Approved

## Summary

Add a "Terra" color theme to Crowbar — an earthy, nature-inspired palette based on deep forest greens, warm cream, and copper accents. It appears as a second option in the Color Theme selector in Settings alongside the existing "Crowbar" theme. Both dark and light mode variants are supported.

## Palette

| Name | Hex | Role |
|---|---|---|
| Onyx | `#0F150A` | Darkest background (dark mode) |
| Evergreen | `#19220F` | Surface / sidebar background |
| Black Forest | `#283618` | Elevated surfaces, muted tones |
| Olive Leaf | `#606C38` | **Primary** — buttons, focus rings, active states |
| Cornsilk | `#FEFAE0` | Foreground text (dark) / background (light) |
| Copperwood | `#BC6C25` | **Secondary/accent** — hovers, highlights, secondary actions |

## Color Mapping

### Dark mode — `[data-theme="terra"].dark`

| CSS variable | Value | Notes |
|---|---|---|
| `--background` | `#0F150A` | Onyx |
| `--foreground` | `#FEFAE0` | Cornsilk |
| `--card` | `#19220F` | Evergreen |
| `--card-foreground` | `#FEFAE0` | |
| `--popover` | `#19220F` | Evergreen |
| `--popover-foreground` | `#FEFAE0` | |
| `--primary` | `#606C38` | Olive Leaf |
| `--primary-foreground` | `#FEFAE0` | Cornsilk |
| `--secondary` | `#BC6C25` | Copperwood |
| `--secondary-foreground` | `#FEFAE0` | |
| `--accent` | `#BC6C25` | Copperwood |
| `--accent-foreground` | `#FEFAE0` | |
| `--muted` | `oklch(1 0 0 / 6%)` | Light wash on dark |
| `--muted-foreground` | `#7a8a5a` | Lightened Black Forest |
| `--border` | `oklch(1 0 0 / 8%)` | |
| `--input` | `oklch(1 0 0 / 10%)` | |
| `--ring` | `#606C38` | Olive Leaf |
| `--sidebar` | `oklch(0.16 0.034 140 / 70%)` | Evergreen @ 70% opacity |
| `--sidebar-foreground` | `#FEFAE0` | |
| `--sidebar-primary` | `#606C38` | Olive Leaf |
| `--sidebar-primary-foreground` | `#FEFAE0` | |
| `--sidebar-accent` | `oklch(1 0 0 / 4%)` | |
| `--sidebar-accent-foreground` | `#FEFAE0` | |
| `--sidebar-border` | `oklch(1 0 0 / 5%)` | |
| `--sidebar-ring` | `#606C38` | Olive Leaf |
| `--code` | `#19220F` | Evergreen |
| `--code-foreground` | `#FEFAE0` | |
| `--code-highlight` | `oklch(1 0 0 / 4%)` | |
| `--chrome-bg` | `oklch(0 0 0 / 65%)` | Same as Crowbar dark |
| `--editor-selection` | `oklch(0.55 0.08 120 / 0.30)` | Green-tinted selection wash |

Semantic colors (`--destructive`, `--success`, `--warning`, `--info`) inherit the `.dark` defaults unchanged — they are universal and not theme-specific.

Syntax highlighting inherits the existing "Palette A" from the `.dark` block — no new syntax colors needed.

### Light mode — `[data-theme="terra"]:not(.dark)`

| CSS variable | Value | Notes |
|---|---|---|
| `--background` | `#FEFAE0` | Cornsilk |
| `--foreground` | `#0F150A` | Onyx |
| `--card` | `#ffffff` | Warm white |
| `--card-foreground` | `#0F150A` | |
| `--popover` | `#ffffff` | |
| `--popover-foreground` | `#0F150A` | |
| `--primary` | `#606C38` | Olive Leaf |
| `--primary-foreground` | `#FEFAE0` | Cornsilk |
| `--secondary` | `#BC6C25` | Copperwood |
| `--secondary-foreground` | `#FEFAE0` | |
| `--accent` | `#BC6C25` | Copperwood |
| `--accent-foreground` | `#FEFAE0` | |
| `--muted` | `oklch(0 0 0 / 5%)` | Dark wash on light |
| `--muted-foreground` | `#283618` | Black Forest |
| `--border` | `oklch(0 0 0 / 8%)` | |
| `--input` | `oklch(0 0 0 / 10%)` | |
| `--ring` | `#606C38` | Olive Leaf |
| `--sidebar` | `#19220F` | Evergreen — intentionally dark in light mode |
| `--sidebar-foreground` | `#FEFAE0` | Cornsilk |
| `--sidebar-primary` | `#606C38` | Olive Leaf |
| `--sidebar-primary-foreground` | `#FEFAE0` | |
| `--sidebar-accent` | `oklch(1 0 0 / 6%)` | Light wash on dark sidebar |
| `--sidebar-accent-foreground` | `#FEFAE0` | |
| `--sidebar-border` | `oklch(1 0 0 / 5%)` | |
| `--sidebar-ring` | `#606C38` | Olive Leaf |
| `--code` | `#ffffff` | |
| `--code-foreground` | `#0F150A` | |
| `--code-highlight` | `oklch(0 0 0 / 4%)` | |
| `--chrome-bg` | `oklch(0.97 0.018 120 / 0%)` | Warm green-tinted chrome |
| `--editor-selection` | `oklch(0.62 0.08 120 / 0.28)` | Green-tinted selection wash |

**Notable decision:** The sidebar stays dark green (`#19220F` Evergreen) in light mode. This creates a strong cream-body / dark-green-sidebar contrast that makes the layout feel intentional and distinctive.

Semantic colors (`--destructive`, `--success`, `--warning`, `--info`) inherit `:root` defaults unchanged.

## File Structure

### New file: `web/src/styles/terra.css`

Contains both theme blocks (`[data-theme="terra"].dark` and `[data-theme="terra"]:not(.dark)`). Imported once in the main CSS entry point alongside `theme.css`.

### Edit: `web/src/extensions/themes/theme-registry.ts`

Add one entry to `BUILTIN_THEMES`:

```ts
{
  id: 'terra',
  name: 'Terra',
  isDark: true,
  type: 'dark',
  category: 'Dark',
}
```

### Edit: main CSS entry point

One import line: `@import './terra.css';`

## What is NOT changing

- Theme switcher UI — already exists, reads `themeRegistry.getAllThemes()` dynamically
- Persistence / bootstrap cache — already wired up
- `.dark` class toggling — already handled by Theme Mode setting
- Syntax highlighting — inherits existing "Palette A" tokens
- Semantic colors (destructive, success, warning, info) — universal, not themed
