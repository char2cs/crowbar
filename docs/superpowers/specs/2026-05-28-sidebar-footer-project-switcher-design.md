# Sidebar Footer: Project Switcher

**Date:** 2026-05-28
**Branch:** enhancement/design-language

## Goal

Move the "Projects / {ActiveProject}" breadcrumb and settings gear from the top of the sidebar to the footer. Remove the MU user avatar and the blue logo square entirely.

## Current State

`SidebarHeader.tsx` renders a 48px strip at the top of the sidebar containing:
- Blue logo square (22×22px)
- "Projects" button → navigates to `/projects`
- "/" separator
- Active project dropdown (uses `useProjectStore`)
- Settings gear icon → opens `SettingsDialog`
- "MU" user avatar

`IDEShell.tsx` mounts `<SidebarHeader>` immediately below the titlebar/nav-icons row.

## Target State

```
┌─────────────────────────┐
│  [Workspaces][Files][Git]│  ← titlebar (unchanged)
├─────────────────────────┤
│                         │
│   SidebarTabs content   │  ← fills all remaining space
│                         │
├─────────────────────────┤
│  Projects / Rabbyte  ⚙  │  ← new footer (no logo, no MU)
└─────────────────────────┘
```

## Changes

### 1. Rename `SidebarHeader.tsx` → `sidebar-project-switcher.tsx`

- Export renamed: `SidebarHeader` → `SidebarProjectSwitcher`
- Remove the blue logo square `<div>` (first child)
- Remove the `userInitials` prop and the `<Avatar>` element
- Interface renamed: `SidebarHeaderProps` → `SidebarProjectSwitcherProps` (drop `userInitials`)
- Change the container div's `border-b` → `border-t` (it's now a footer, border belongs on top)
- All other logic unchanged: project dropdown, "Projects" button, settings gear

### 2. Update `IDEShell.tsx`

- Remove import of `SidebarHeader` from `./SidebarHeader`
- Add imports: `SidebarFooter` from `@/components/ui/sidebar`, `SidebarProjectSwitcher` from `./sidebar-project-switcher`
- Remove `<SidebarHeader userInitials="MU" ... />` from the top of the sidebar flex column
- Add at the bottom of the sidebar flex column (after `<SidebarTabs>`):
  ```tsx
  <SidebarFooter className="p-0">
    <SidebarProjectSwitcher
      onProjectsClick={...}
      onProjectSelect={...}
      onSettingsClick={...}
    />
  </SidebarFooter>
  ```
- `userInitials` prop removed from the call site

## Files Changed

| File | Action |
|------|--------|
| `web/src/components/layout/SidebarHeader.tsx` | Renamed → `sidebar-project-switcher.tsx`, stripped logo + MU |
| `web/src/components/layout/IDEShell.tsx` | Updated imports, removed header usage, added footer usage |

## Out of Scope

- No changes to `SidebarNavIcons`, `SidebarTabs`, or any other component
- No routing or settings logic changes
- No test changes (no existing tests for these components)
