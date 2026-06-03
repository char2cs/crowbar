# IDE Tab Bar — Pill Restyle Design

**Date:** 2026-05-26  
**Branch:** enhancement/design-language  
**Scope:** Pure visual restyle — zero behavior changes

---

## Overview

Redesign the IDE tab bar (`tab-bar.tsx` + `tab-bar-item.tsx`) so tabs use a pill/capsule shape matching the reference design. All existing behavior (DnD reorder, keyboard navigation, context menu, pinning, preview tabs, dirty indicator, split-pane actions) is preserved unchanged.

---

## Visual Design

### Active tab
- Shape: `rounded-full` pill
- Background: `bg-muted` (adapts to light/dark via shadcn token)
- Label: `text-foreground`, `font-medium`
- Icon: existing per-type icon, `text-muted-foreground`
- Close button: 14×14 `rounded-full` ghost circle, sits inside the pill on the right
  - At rest: transparent background, `text-muted-foreground`, slightly dimmed (`opacity-60`)
  - On hover: `bg-foreground/10` circular fill, `text-foreground`, full opacity
  - Hidden on inactive tabs until that tab is hovered (existing behaviour preserved)

### Inactive tab
- Shape: no background, no border, no underline
- Label: `text-muted-foreground`, dimmed
- Icon: existing per-type icon, `text-muted-foreground`, dimmed
- No close button visible at rest; appears on hover

### Nav buttons (← →) and action button (+)
- Shape: 20×20 `rounded-full` (perfect circle)
- At rest: transparent background, `text-muted-foreground`, `opacity-55`
- On hover: `bg-muted` fill, full opacity
- Disabled (← at history start, → at history end): `opacity-20`, no hover effect
- No border, no outline — ghost treatment throughout

### Tab bar container
- Background: `bg-background` (unchanged)
- Height: `h-7` (28px, unchanged)
- Gap between tabs: `gap-1` (unchanged)
- Padding: `px-1.5 py-0.5` (unchanged)

---

## Token Map

All values come from shadcn/ui CSS variables. No hardcoded hex or oklch values in component code.

| Element | Tailwind class | Token |
|---|---|---|
| Active pill background | `bg-muted` | `--muted` |
| Active tab label | `text-foreground` | `--foreground` |
| Inactive tab label / icons | `text-muted-foreground` | `--muted-foreground` |
| Nav/action button hover bg | `bg-muted` | `--muted` |
| Close button hover bg | `bg-foreground/10` | `--foreground` at 10% |
| Tab bar background | `bg-background` | `--background` |

---

## Files Changed

### `web/src/features/tabs/components/tab-bar-item.tsx`

1. **`Tab` className** — replace flat `bg-muted/80` active state with full pill:
   - All states: add `rounded-full`
   - Active: `bg-muted pl-2 pr-5` (the `pr-5` reserves space for the absolutely-positioned close button circle)
   - Inactive: `bg-transparent pl-2 pr-2`

2. **Close/pin `Button`** — stays absolutely positioned as a sibling of `Tab` (existing structure preserved to avoid invalid nested-button HTML that breaks DnD). Style changes only:
   - Keep `absolute top-1/2 right-1 -translate-y-1/2`
   - Change `rounded-sm` → `rounded-full`
   - Change `h-4 min-w-4` → `h-3.5 min-w-3.5` (14px circle)
   - Change `px-0` → `p-0`
   - Add `hover:bg-foreground/10` for the circular highlight
   - Keep `opacity-0 group-hover/tab:opacity-100` for inactive tabs; set `opacity-60` for active at rest (was implicitly 100% via the `opacity-100` branch)

### `web/src/features/tabs/components/tab-bar.tsx`

3. **Back button** — change `rounded-md` → `rounded-full`
4. **Forward button** — change `rounded-md` → `rounded-full`
5. **+ (new tab) `DropdownMenuTrigger`** — change `rounded-md` → `rounded-full`
6. **Close Split `Button`** — change `rounded-md` → `rounded-full`

---

## What Does NOT Change

- All DnD drag-and-drop reorder logic
- Keyboard navigation (Arrow keys, Home, End, Delete, Backspace, Enter)
- Context menu (right-click)
- Pin / unpin behaviour
- Preview tab italics
- Dirty dot indicator (unsaved changes)
- Scroll behaviour (horizontal overflow, wheel scroll)
- `SortableEditorTab` wrapper
- `DragOverlay` preview
- All store interactions and event handlers
- `tab-drag-preview.tsx`, `tab-context-menu.tsx` — untouched

---

## Out of Scope

- Sidebar navigation tabs (`SidebarTabs.tsx`) — separate decision
- `IDETabBar.tsx` header bar — separate decision
- Terminal tab bar (`terminal-tab-bar.tsx`) — separate decision
- Any new tab features or behavioral changes
