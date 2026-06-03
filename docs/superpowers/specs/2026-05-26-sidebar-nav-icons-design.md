# Sidebar Nav Icons in the Titlebar Strip

**Date:** 2026-05-26  
**Branch:** enhancement/design-language  
**Status:** Approved

## Problem

The sidebar's "Workspaces | Files | Git" text tab bar is visually heavy and takes up a dedicated row of vertical space. The goal is to replace it with icon buttons placed in the existing 38px traffic-light strip — beside the macOS traffic lights — matching the visual language of modern dev tools.

## Design

### Layout & component boundaries

A full-width 38px strip visually spans the top of the window. It is **not** a new wrapper component. It is two flush-aligned, independently owned strips:

- **Sidebar panel** owns its 38px top strip: renders the traffic-lights drag region + nav icons.
- **Content panel** (pane tab bar) owns its 38px top strip: renders file tabs, unchanged.

The two strips share the same `background` colour and `border-bottom` colour so they read as one seamless bar. The sidebar's `border-right` is applied only on `sidebar-body` (below the strip) — **not** on the top strip — so no vertical line cuts through the unified bar.

The `ResizablePanelGroup`, `ResizableHandle`, and all content-panel internals are untouched.

### Nav icons

Three `@phosphor-icons/react` icons replace the `<TabsList>` inside `SidebarTabs`:

| Sidebar tab | Icon | Phosphor component |
|---|---|---|
| Workspaces | ⊞ squares grid | `SquaresFour` |
| Files | 📂 open folder | `FolderOpen` |
| Git | ⎇ branch | `GitBranch` |

**Inactive state:** `text-muted-foreground`; hover → `text-foreground`. No background.  
**Active state:** filled chip — `bg-accent` background, `text-foreground` icon (same pattern used elsewhere in the app for selected states).  
**Tooltips:** each icon has a Radix tooltip showing the tab name (Workspaces / Files / Git), since text labels are removed.

### Platform-adaptive positioning

Icons always anchor to the **sidebar edge**:

| Platform | Sidebar left | Sidebar right |
|---|---|---|
| **macOS** | Left-aligned, immediately after traffic lights (~80px left inset already occupied by native chrome). Strip height: 38px. | Right-aligned, 8px right padding (native chrome is left on macOS — no conflict). |
| **Windows / Linux** | Left-aligned, 8px left padding. Strip height: 28px. No conflict — native chrome is right. | Right-aligned, right inset of ~138px to clear native minimize/maximize/close buttons. Strip height: 28px. |

`IS_MAC`, `IS_WINDOWS`, `IS_LINUX` from `@/utils/platform` drive the strip height and icon insets. `sidebarPosition` from `useSettingsStore` drives left vs right anchoring.

The `data-tauri-drag-region` attribute remains on the full sidebar top strip so the window stays draggable on macOS. Tauri allows pointer events on interactive children (buttons) within a drag region — the icon buttons will receive clicks normally.

### File tab bar height

The pane `tab-bar` is currently `h-9` (36px). It must be changed to `h-[38px]` (macOS) / `h-[28px]` (Windows/Linux) to flush-align with the sidebar strip, matching the `titleBarHeight = IS_MAC ? 44 : 28` constant already in `SplitViewRoot`. A single Tailwind class change, conditioned on `IS_MAC`.

### Fullscreen pane overlay offset

`SplitViewRoot` hard-codes `titleBarHeight = IS_MAC ? 44 : 28` for the fullscreen-pane overlay `top` offset. This value already accounts for the macOS traffic-light strip and does not need to change — the sidebar nav icons live within that existing 38px budget.

## Components changed

| File | Change |
|---|---|
| `web/src/components/layout/IDEShell.tsx` | Replace blank `h-[38px] data-tauri-drag-region` div with a flex row that holds the drag region + `<SidebarNavIcons>`. Apply platform height (`h-[38px]` macOS, `h-[28px]` Win/Linux). |
| `web/src/components/layout/SidebarTabs.tsx` | Remove `<TabsList>` and the three `<TabsTrigger>` elements entirely. Keep `<TabsContent>` panels. The active tab is now driven by the icon buttons. |
| `web/src/features/tabs/components/tab-bar.tsx` | Change `h-9` → `h-[38px]` (macOS) / `h-[32px]` (Win/Linux) to flush-align with the sidebar strip. |
| **new** `web/src/components/layout/sidebar-nav-icons.tsx` | Three icon buttons (SquaresFour, FolderOpen, GitBranch). Reads `activeTab` from `useSidebarStore`; calls `setActiveTab` on click. Tooltip per icon. Platform-aware horizontal padding and anchoring based on `IS_MAC` / `sidebarPosition`. |

## Out of scope

- `SidebarHeader` (project selector, settings gear, user avatar) is unchanged.
- No changes to sidebar content panels (WorkspacesSidebarPanel, FileExplorerTree, GitView).
- No changes to the ResizablePanelGroup or content panel layout.
- No new icons beyond the three nav items.
- No animation on icon transitions (keep it simple).
