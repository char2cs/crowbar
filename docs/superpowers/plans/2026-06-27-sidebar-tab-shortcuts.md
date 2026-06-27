# Sidebar Tab Keyboard Shortcuts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `mod+1/2/3/4` keyboard shortcuts that switch the active sidebar tab, configurable via Settings > Keybindings.

**Architecture:** Four new commands registered in the keymap registry under the existing `'Navigation'` category; a single focused hook reads their effective chords and dispatches to the sidebar store on keydown; the hook is mounted in `WorkspaceViewInner` alongside the existing pane/save keyboard hooks.

**Tech Stack:** React, Zustand (`useSidebarStore`), Vitest + @testing-library/react

## Global Constraints

- Test files in `web/src/__tests__/` mirroring `web/src/` structure; use `@/` imports inside tests
- Component files use kebab-case filenames
- Store access inside event handlers must use `.getState()`, never the hook on the render path
- `liveEditable: true` on all four commands so they appear in Settings > Keybindings
- `e.repeat` guard required — consistent with `useWorkspaceSwitcherKeyboard` pattern
- Hook mounted in `WorkspaceViewInner` (the inner function in `workspace-view.tsx` at line 59)

---

### Task 1: Register four sidebar tab commands in the keymap registry

**Files:**
- Modify: `web/src/features/keymaps/registry.ts`

**Interfaces:**
- Produces:
  - `SIDEBAR_TAB_WORKSPACES = 'navigation.sidebarWorkspaces'` (exported const)
  - `SIDEBAR_TAB_CHATS = 'navigation.sidebarChats'` (exported const)
  - `SIDEBAR_TAB_FILES = 'navigation.sidebarFiles'` (exported const)
  - `SIDEBAR_TAB_GIT = 'navigation.sidebarGit'` (exported const)
  - Four entries added to `COMMANDS` array with `defaultChord: 'mod+1'` through `'mod+4'`

- [ ] **Step 1: Add the four constants and COMMANDS entries**

Open `web/src/features/keymaps/registry.ts`. After the `OPEN_WORKSPACE_SWITCHER` constant line, add:

```ts
export const SIDEBAR_TAB_WORKSPACES = 'navigation.sidebarWorkspaces'
export const SIDEBAR_TAB_CHATS = 'navigation.sidebarChats'
export const SIDEBAR_TAB_FILES = 'navigation.sidebarFiles'
export const SIDEBAR_TAB_GIT = 'navigation.sidebarGit'
```

Then inside `COMMANDS`, after the existing Navigation section entry for `OPEN_WORKSPACE_SWITCHER`, add:

```ts
  {
    id: SIDEBAR_TAB_WORKSPACES,
    label: 'Sidebar: Workspaces',
    category: 'Navigation',
    defaultChord: 'mod+1',
    liveEditable: true,
  },
  {
    id: SIDEBAR_TAB_CHATS,
    label: 'Sidebar: Chats',
    category: 'Navigation',
    defaultChord: 'mod+2',
    liveEditable: true,
  },
  {
    id: SIDEBAR_TAB_FILES,
    label: 'Sidebar: Files',
    category: 'Navigation',
    defaultChord: 'mod+3',
    liveEditable: true,
  },
  {
    id: SIDEBAR_TAB_GIT,
    label: 'Sidebar: Git',
    category: 'Navigation',
    defaultChord: 'mod+4',
    liveEditable: true,
  },
```

- [ ] **Step 2: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

Expected: no new errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/features/keymaps/registry.ts
git commit -m "feat(keymaps): register sidebar tab navigation commands (mod+1–4)"
```

---

### Task 2: Create `useSidebarTabKeyboard` hook with tests

**Files:**
- Create: `web/src/features/keymaps/hooks/use-sidebar-tab-keyboard.ts`
- Create: `web/src/__tests__/features/keymaps/hooks/use-sidebar-tab-keyboard.test.ts`

**Interfaces:**
- Consumes (from Task 1):
  - `SIDEBAR_TAB_WORKSPACES`, `SIDEBAR_TAB_CHATS`, `SIDEBAR_TAB_FILES`, `SIDEBAR_TAB_GIT` from `@/features/keymaps/registry`
  - `useEffectiveChordMap` from `@/features/keymaps/hooks/use-effective-keymap`
  - `eventMatchesChord` from `@/features/keymaps/utils/chord`
  - `useSidebarStore` from `@/lib/store/sidebar` (`.getState().setActiveTab` only — never on render path)
- Produces: `useSidebarTabKeyboard(): void`

- [ ] **Step 1: Write the failing tests**

Create `web/src/__tests__/features/keymaps/hooks/use-sidebar-tab-keyboard.test.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useSidebarTabKeyboard } from '@/features/keymaps/hooks/use-sidebar-tab-keyboard'

vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  IS_MAC: false,
}))

vi.mock('@/features/keymaps/hooks/use-effective-keymap', () => ({
  useEffectiveChordMap: () => ({
    'navigation.sidebarWorkspaces': 'mod+1',
    'navigation.sidebarChats': 'mod+2',
    'navigation.sidebarFiles': 'mod+3',
    'navigation.sidebarGit': 'mod+4',
  }),
}))

const setActiveTab = vi.fn()
vi.mock('@/lib/store/sidebar', () => ({
  useSidebarStore: {
    getState: () => ({ setActiveTab }),
  },
}))

function dispatchKeydown(init: KeyboardEventInit): KeyboardEvent {
  const event = new KeyboardEvent('keydown', { cancelable: true, bubbles: true, ...init })
  window.dispatchEvent(event)
  return event
}

describe('useSidebarTabKeyboard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('switches to workspaces on Ctrl+1', () => {
    renderHook(() => useSidebarTabKeyboard())
    const event = dispatchKeydown({ key: '1', ctrlKey: true })
    expect(setActiveTab).toHaveBeenCalledWith('workspaces')
    expect(event.defaultPrevented).toBe(true)
  })

  it('switches to chats on Ctrl+2', () => {
    renderHook(() => useSidebarTabKeyboard())
    const event = dispatchKeydown({ key: '2', ctrlKey: true })
    expect(setActiveTab).toHaveBeenCalledWith('chats')
    expect(event.defaultPrevented).toBe(true)
  })

  it('switches to files on Ctrl+3', () => {
    renderHook(() => useSidebarTabKeyboard())
    const event = dispatchKeydown({ key: '3', ctrlKey: true })
    expect(setActiveTab).toHaveBeenCalledWith('files')
    expect(event.defaultPrevented).toBe(true)
  })

  it('switches to git on Ctrl+4', () => {
    renderHook(() => useSidebarTabKeyboard())
    const event = dispatchKeydown({ key: '4', ctrlKey: true })
    expect(setActiveTab).toHaveBeenCalledWith('git')
    expect(event.defaultPrevented).toBe(true)
  })

  it('does not fire on plain number key', () => {
    renderHook(() => useSidebarTabKeyboard())
    dispatchKeydown({ key: '1' })
    expect(setActiveTab).not.toHaveBeenCalled()
  })

  it('does not fire on repeat keydown', () => {
    renderHook(() => useSidebarTabKeyboard())
    dispatchKeydown({ key: '1', ctrlKey: true, repeat: true })
    expect(setActiveTab).not.toHaveBeenCalled()
  })

  it('removes the listener on unmount', () => {
    const { unmount } = renderHook(() => useSidebarTabKeyboard())
    unmount()
    dispatchKeydown({ key: '1', ctrlKey: true })
    expect(setActiveTab).not.toHaveBeenCalled()
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd web && npx vitest run src/__tests__/features/keymaps/hooks/use-sidebar-tab-keyboard.test.ts 2>&1 | tail -10
```

Expected: FAIL — "Cannot find module '@/features/keymaps/hooks/use-sidebar-tab-keyboard'"

- [ ] **Step 3: Create the hook**

Create `web/src/features/keymaps/hooks/use-sidebar-tab-keyboard.ts`:

```ts
import { useEffect } from 'react'
import { useEffectiveChordMap } from '@/features/keymaps/hooks/use-effective-keymap'
import { eventMatchesChord } from '@/features/keymaps/utils/chord'
import {
  SIDEBAR_TAB_WORKSPACES,
  SIDEBAR_TAB_CHATS,
  SIDEBAR_TAB_FILES,
  SIDEBAR_TAB_GIT,
} from '@/features/keymaps/registry'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { SidebarTab } from '@/lib/store/sidebar'

const SIDEBAR_TAB_COMMANDS: Array<[string, SidebarTab]> = [
  [SIDEBAR_TAB_WORKSPACES, 'workspaces'],
  [SIDEBAR_TAB_CHATS, 'chats'],
  [SIDEBAR_TAB_FILES, 'files'],
  [SIDEBAR_TAB_GIT, 'git'],
]

export function useSidebarTabKeyboard(): void {
  const chordMap = useEffectiveChordMap()

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.repeat) return
      for (const [commandId, tab] of SIDEBAR_TAB_COMMANDS) {
        const chord = chordMap[commandId]
        if (chord && eventMatchesChord(e, chord)) {
          e.preventDefault()
          useSidebarStore.getState().setActiveTab(tab)
          return
        }
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [chordMap])
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd web && npx vitest run src/__tests__/features/keymaps/hooks/use-sidebar-tab-keyboard.test.ts 2>&1 | tail -10
```

Expected: 7 tests pass.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/keymaps/hooks/use-sidebar-tab-keyboard.ts \
        web/src/__tests__/features/keymaps/hooks/use-sidebar-tab-keyboard.test.ts
git commit -m "feat(keymaps): useSidebarTabKeyboard hook (mod+1–4 → sidebar tabs)"
```

---

### Task 3: Mount the hook in WorkspaceViewInner

**Files:**
- Modify: `web/src/features/workspace/components/workspace-view.tsx`

**Interfaces:**
- Consumes (from Task 2): `useSidebarTabKeyboard` from `@/features/keymaps/hooks/use-sidebar-tab-keyboard`

The hook is called inside `WorkspaceViewInner` (line 59 of `workspace-view.tsx`) — the same component that already calls `useSaveKeyboard()` and `usePaneKeyboard()`.

- [ ] **Step 1: Add the import and hook call**

Open `web/src/features/workspace/components/workspace-view.tsx`.

Add the import after the existing keyboard hook imports (after line 9):

```ts
import { useSidebarTabKeyboard } from '@/features/keymaps/hooks/use-sidebar-tab-keyboard'
```

Inside `WorkspaceViewInner`, add `useSidebarTabKeyboard()` after `usePaneKeyboard()`:

```ts
function WorkspaceViewInner({ wsId }: Pick<WorkspaceViewProps, 'wsId'>) {
  useWorkspaceEffects(wsId)
  useSaveKeyboard()
  usePaneKeyboard()
  useSidebarTabKeyboard()
  return <WorkspaceLayoutRoot />
}
```

- [ ] **Step 2: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

Expected: no new errors.

- [ ] **Step 3: Run full test suite**

```bash
cd web && npx vitest run 2>&1 | tail -15
```

Expected: all new tests pass, no regressions beyond the pre-existing 34 failures.

- [ ] **Step 4: Commit**

```bash
git add web/src/features/workspace/components/workspace-view.tsx
git commit -m "feat(workspace): mount useSidebarTabKeyboard — mod+1-4 switch sidebar tabs"
```
