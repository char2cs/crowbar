# Sidebar Tab Keyboard Shortcuts

**Date:** 2026-06-27
**Branch:** hardening/production-readiness

## Goal

Add `Cmd+1/2/3/4` (Mac) / `Ctrl+1/2/3/4` (Windows/Linux) as global keyboard shortcuts to switch between the four sidebar tabs. All four shortcuts must be live-editable through Settings > Keybindings.

## Tab Mapping

| Shortcut | Tab         | SidebarTab value |
|----------|-------------|------------------|
| `mod+1`  | Workspaces  | `'workspaces'`   |
| `mod+2`  | Chats       | `'chats'`        |
| `mod+3`  | Files       | `'files'`        |
| `mod+4`  | Git         | `'git'`          |

## Design

### 1. Registry — four new commands

In `features/keymaps/registry.ts`, export four constants and add entries to `COMMANDS` under the existing `'Navigation'` category:

```ts
export const SIDEBAR_TAB_WORKSPACES = 'navigation.sidebarWorkspaces'
export const SIDEBAR_TAB_CHATS      = 'navigation.sidebarChats'
export const SIDEBAR_TAB_FILES      = 'navigation.sidebarFiles'
export const SIDEBAR_TAB_GIT        = 'navigation.sidebarGit'
```

Each command: `liveEditable: true`, `category: 'Navigation'`, defaultChords `mod+1` through `mod+4`.

### 2. Keyboard hook — new file

Create `features/keymaps/hooks/use-sidebar-tab-keyboard.ts`:
- `useSidebarTabKeyboard(): void`
- Reads all four effective chords via `useEffectiveChordMap()`
- Single `useEffect` with one `keydown` listener on `window`
- On match, calls `useSidebarStore.getState().setActiveTab(tab)` — uses `.getState()` inside the event handler (not the render path), consistent with CLAUDE.md store rules
- Guards `e.repeat` — same pattern as `useWorkspaceSwitcherKeyboard`

### 3. Mount point

In `features/workspace/components/workspace-view.tsx`, call `useSidebarTabKeyboard()` alongside the existing `usePaneKeyboard()` and `useSaveKeyboard()`.

### 4. Settings UI

No change needed — `'Navigation'` is already first in `CATEGORY_ORDER`. The four commands appear automatically.

## Files changed

| File | Change |
|------|--------|
| `features/keymaps/registry.ts` | 4 constants + 4 `COMMANDS` entries |
| `features/keymaps/hooks/use-sidebar-tab-keyboard.ts` | New hook |
| `features/workspace/components/workspace-view.tsx` | Call `useSidebarTabKeyboard()` |
| `web/src/__tests__/features/keymaps/hooks/use-sidebar-tab-keyboard.test.ts` | New test file |

## Behaviour

- `mod+1–4` switch the active sidebar tab from anywhere in the workspace
- Shortcuts are live-editable and conflict-detected in Settings > Keybindings > Navigation
- Pressing a shortcut for the already-active tab is a no-op (idempotent store set)
- Holding the key down does not fire repeatedly (`e.repeat` guard)
