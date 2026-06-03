# Sidebar Project Bloom — Design Spec

**Date:** 2026-05-29
**Branch:** enhancement/design-language

---

## Summary

Redesign the sidebar header to show a project color "bloom" (IntelliJ-inspired) behind the traffic lights, move the project dropdown into that header area, move the nav tab switcher below the header (centered), and remove the footer project switcher entirely.

---

## Layout Changes

### Before

```
┌─ Sidebar ──────────────────────────────┐
│ [header 44px]  [Workspaces│Files│Git ▶]│  ← nav tabs aligned right/left
│ [SidebarTabs content]                  │
│ [footer] Projects / crowbar  ⚙         │  ← project switcher footer
└────────────────────────────────────────┘
```

### After

```
┌─ Sidebar ──────────────────────────────┐
│ [header 44px] ░░bloom░░  crowbar ▾    │  ← bloom bg + project dropdown
│    [Workspaces │ Files │ Git]          │  ← nav tabs centered, no separator
│ [SidebarTabs content]                  │
└────────────────────────────────────────┘
```

- No border between header and nav row.
- No footer. `SidebarFooter` and `SidebarProjectSwitcher` removed from `IDEShell`.
- Settings access moves to the settings dialog (already accessible elsewhere).

---

## Bloom Effect

**Style:** Left-to-right gradient wash, confined to the 44px header height only.

```
linear-gradient(90deg,
  hsla(<hue>, 40%, 60%, 0.35)   0%,
  hsla(<hue>, 40%, 60%, 0.08)  60%,
  transparent                  100%
)
```

**Placement:** Absolute-positioned layer behind all header content, `z-index: 0`. Header content sits at `z-index: 1`.

**Sidebar position aware:**
- Sidebar on left → gradient goes left-to-right (bloom behind traffic lights, project name on right).
- Sidebar on right → gradient goes right-to-left (bloom behind traffic lights on right, project name on left).

---

## Color Algorithm

```ts
function projectNameToHue(name: string): number {
  let hash = 0
  for (let i = 0; i < name.length; i++) {
    hash = (hash * 31 + name.charCodeAt(i)) >>> 0
  }
  return hash % 360
}
```

- Input: `activeProject?.name ?? ''`
- Output: integer hue 0–359
- No project → hue 0 (renders as near-invisible neutral, effectively no bloom)

The fixed saturation (40%) and lightness (60%) keep the color recognizable but subtle enough to work on both dark and light themes. The 0.35 alpha at the strong end keeps it from washing out text.

---

## New Component: `SidebarProjectHeader`

**File:** `web/src/components/layout/sidebar-project-header.tsx`

**Props:**
```ts
interface SidebarProjectHeaderProps {
  onSettingsClick?: () => void
}
```

Reads from stores internally:
- `useProjectStore` for `projects`, `activeProjectId`, `setActiveProject`
- `useSettingsStore` for `sidebarPosition`

**Rendered structure:**
```tsx
<div className="relative flex h-[44px] items-center overflow-hidden px-3" data-tauri-drag-region>
  {/* Bloom layer */}
  <div className="absolute inset-0 z-0 pointer-events-none" style={{ background: bloomGradient }} />

  {/* Traffic lights space (macOS) — real lights are rendered by the OS/Tauri frame */}
  {IS_MAC && <div className="relative z-10 w-[52px] shrink-0" />}

  {/* Project dropdown */}
  <DropdownMenu>
    <DropdownMenuTrigger className="... ml-auto z-10">
      {activeProject?.name ?? 'Select project'}
      <ChevronDown />
    </DropdownMenuTrigger>
    <DropdownMenuContent>
      {projects.map(...)}
      <DropdownMenuSeparator />
      <DropdownMenuItem>Manage projects…</DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>
</div>
```

When `sidebarPosition === 'right'`, the gradient direction flips and the spacer/dropdown swap sides.

---

## Nav Row Changes

`SidebarNavIcons` moves out of the header and into its own row, always horizontally centered:

```tsx
<div className="flex h-8 shrink-0 items-center justify-center">
  <SidebarNavIcons />
</div>
```

The existing left/right alignment logic inside `SidebarNavIcons` (the `ml-auto`/`mr-auto` wrapper) is removed — centering is owned by the parent row in `IDEShell`.

---

## Files Changed

| File | Change |
|------|--------|
| `web/src/components/layout/sidebar-project-header.tsx` | **New** — bloom header component |
| `web/src/components/layout/IDEShell.tsx` | Replace header + footer layout; add nav row |
| `web/src/components/layout/sidebar-nav-icons.tsx` | Remove `ml-auto`/`mr-auto` wrapper positioning |
| `web/src/components/layout/sidebar-project-switcher.tsx` | **Delete** — replaced by `SidebarProjectHeader` |

---

## Out of Scope

- Light theme color tuning (hue algorithm works on both; fine-tuning per theme is a follow-up).
- Animated bloom transition when switching projects (can be added later with `transition: background`).
- Settings gear icon in the header (settings already accessible via keyboard shortcut and menu).
