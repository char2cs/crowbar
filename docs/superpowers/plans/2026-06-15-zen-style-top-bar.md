# Zen-style Sidebar Top Bar Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the sidebar header into a Zen-browser-style top bar (sidebar-toggle on the leading edge; back / forward / settings on the trailing edge, mirrored for a right-side sidebar), reusing Crowbar's existing editor jump navigation.

**Architecture:** The sidebar header lives *outside* the per-workspace `WorkspaceStoreContext.Provider`, so we add a small reactive `useActiveWorkspaceBuffer` hook (router `wsId` + workspace registry via `useSyncExternalStore`) and decouple `useJumpNavigation` from React context (use the registry's `getActiveWorkspaceStore()` accessor). The header then drives the existing editor + webviewer back/forward. Back/forward are removed from the tab bar, which keeps only the sidebar-toggle.

**Tech Stack:** React + TypeScript, Zustand (global + per-workspace registry stores), `useSyncExternalStore`, Vitest + @testing-library/react, Tailwind tokens, `@phosphor-icons/react`.

---

## File Structure

- **Create** `web/src/features/workspace/hooks/use-active-workspace-buffer.ts` — reactive accessor for the globally-active workspace's active buffer.
- **Create** `web/src/features/tabs/hooks/use-active-webviewer-navigation.ts` — derives jump-nav inputs (`usesWebViewerNavigation`, `activeWebViewerNavigation`) from the active buffer.
- **Modify** `web/src/features/tabs/hooks/use-jump-navigation.ts` — drop the `useWorkspaceStore()` React-context dependency (use `getActiveWorkspaceStore()`); subscribe to jump-list state for reactive `canGoBack/canGoForward`.
- **Rewrite** `web/src/components/layout/sidebar-project-header.tsx` — Zen-style top bar layout + wiring.
- **Modify** `web/src/features/tabs/components/tab-navigation-buttons.tsx` — remove back/forward; keep the sidebar-toggle only.
- **Modify** `web/src/features/tabs/components/tab-bar.tsx` — stop computing/passing jump-nav.
- **Tests:** `web/src/__tests__/features/workspace/hooks/use-active-workspace-buffer.test.ts`, `web/src/__tests__/features/tabs/hooks/use-active-webviewer-navigation.test.ts`, `web/src/__tests__/components/layout/sidebar-project-header.test.tsx`.

Verified facts used below:
- `getOrCreateWorkspaceStore(wsId)` / `getActiveWorkspaceStore()` (`features/workspace/stores/workspace-store-registry.ts`) return a vanilla Zustand `WorkspaceStore` (has `getState`/`subscribe`).
- Active buffer id: `state.paneActions.getActivePane()?.activeBufferId`; buffers: `state.buffers`.
- `PaneContent` type (`features/panes/types/pane-content.ts`), discriminated by `type` (e.g. `'webViewer'`, `'editor'`, `'terminal'`).
- `useWebViewerNavigationStore` (`features/web-viewer/stores/web-viewer-navigation-store.ts`): `navigationByBufferId[id]` → `{ url, canGoBack, canGoForward, goBack, goForward, reload }`.
- `useJumpListStore` (`features/editor/stores/jump-list-store.ts`) global, with `entries`, `currentIndex`, `actions` and `createSelectors` (so `useJumpListStore.use.entries()` etc.).
- `useJumpNavigation` returns `{ canGoBack, canGoForward, handleJumpBack, handleJumpForward }`.
- `useSidebar()` (`@/components/ui/sidebar`): `{ open, toggleSidebar }`.
- `useUIState().openSettingsDialog()` (`features/window/stores/ui-state-store.ts`).
- Icons used today: `SidebarSimple`, `ArrowLeft`, `ArrowRight`, `GearSix` from `@phosphor-icons/react`.
- The router `wsId`: `pathname.match(/\/workspaces\/([^/]+)/)?.[1]`.

---

## Task 1: `useActiveWorkspaceBuffer` hook

**Files:**
- Create: `web/src/features/workspace/hooks/use-active-workspace-buffer.ts`
- Test: `web/src/__tests__/features/workspace/hooks/use-active-workspace-buffer.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
// web/src/__tests__/features/workspace/hooks/use-active-workspace-buffer.test.ts
import { renderHook } from '@testing-library/react'
import { describe, it, expect, beforeEach, vi } from 'vitest'

const routerState = { wsId: 'ws1' }
vi.mock('@tanstack/react-router', () => ({
  useRouterState: ({ select }: { select: (s: unknown) => unknown }) =>
    select({ location: { pathname: `/workspaces/${routerState.wsId}` } }),
}))

const fakeStore = {
  state: {
    buffers: [{ id: 'b1', type: 'editor' }],
    paneActions: { getActivePane: () => ({ activeBufferId: 'b1' }) },
  },
  listeners: new Set<() => void>(),
  getState() {
    return this.state
  },
  subscribe(fn: () => void) {
    this.listeners.add(fn)
    return () => this.listeners.delete(fn)
  },
}
vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getOrCreateWorkspaceStore: () => fakeStore,
}))

import { useActiveWorkspaceBuffer } from '@/features/workspace/hooks/use-active-workspace-buffer'

beforeEach(() => {
  routerState.wsId = 'ws1'
  fakeStore.state = {
    buffers: [{ id: 'b1', type: 'editor' }],
    paneActions: { getActivePane: () => ({ activeBufferId: 'b1' }) },
  }
})

describe('useActiveWorkspaceBuffer', () => {
  it('returns the active buffer of the active workspace', () => {
    const { result } = renderHook(() => useActiveWorkspaceBuffer())
    expect(result.current).toEqual({ id: 'b1', type: 'editor' })
  })

  it('returns null when there is no workspace in the route', () => {
    routerState.wsId = ''
    const { result } = renderHook(() => useActiveWorkspaceBuffer())
    expect(result.current).toBeNull()
  })

  it('returns null when the active pane has no active buffer', () => {
    fakeStore.state = {
      buffers: [{ id: 'b1', type: 'editor' }],
      paneActions: { getActivePane: () => ({ activeBufferId: null }) },
    }
    const { result } = renderHook(() => useActiveWorkspaceBuffer())
    expect(result.current).toBeNull()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/__tests__/features/workspace/hooks/use-active-workspace-buffer.test.ts`
Expected: FAIL — cannot resolve the hook module.

- [ ] **Step 3: Write minimal implementation**

```ts
// web/src/features/workspace/hooks/use-active-workspace-buffer.ts
import { useCallback, useSyncExternalStore } from 'react'
import { useRouterState } from '@tanstack/react-router'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import type { PaneContent } from '@/features/panes/types/pane-content'

/**
 * Reactively returns the active buffer of the currently-active workspace.
 *
 * The sidebar header lives outside the per-workspace WorkspaceStoreContext
 * provider, so we cannot use `useWorkspaceStoreContext` here. Instead we read
 * the active workspace id from the router and subscribe to that workspace's
 * store from the registry via `useSyncExternalStore`.
 */
export function useActiveWorkspaceBuffer(): PaneContent | null {
  const wsId = useRouterState({
    select: (s) => s.location.pathname.match(/\/workspaces\/([^/]+)/)?.[1] ?? '',
  })

  const subscribe = useCallback(
    (onChange: () => void) => {
      if (!wsId) return () => {}
      return getOrCreateWorkspaceStore(wsId).subscribe(onChange)
    },
    [wsId],
  )

  const getSnapshot = useCallback((): PaneContent | null => {
    if (!wsId) return null
    const state = getOrCreateWorkspaceStore(wsId).getState()
    const activeBufferId = state.paneActions.getActivePane()?.activeBufferId
    if (!activeBufferId) return null
    return state.buffers.find((b) => b.id === activeBufferId) ?? null
  }, [wsId])

  return useSyncExternalStore(subscribe, getSnapshot)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/__tests__/features/workspace/hooks/use-active-workspace-buffer.test.ts`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/features/workspace/hooks/use-active-workspace-buffer.ts web/src/__tests__/features/workspace/hooks/use-active-workspace-buffer.test.ts
git commit -m "feat(workspace): reactive useActiveWorkspaceBuffer hook"
```

---

## Task 2: `useActiveWebViewerNavigation` hook

**Files:**
- Create: `web/src/features/tabs/hooks/use-active-webviewer-navigation.ts`
- Test: `web/src/__tests__/features/tabs/hooks/use-active-webviewer-navigation.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
// web/src/__tests__/features/tabs/hooks/use-active-webviewer-navigation.test.ts
import { renderHook } from '@testing-library/react'
import { describe, it, expect, beforeEach, vi } from 'vitest'

let activeBuffer: { id: string; type: string } | null = null
vi.mock('@/features/workspace/hooks/use-active-workspace-buffer', () => ({
  useActiveWorkspaceBuffer: () => activeBuffer,
}))

import { useActiveWebViewerNavigation } from '@/features/tabs/hooks/use-active-webviewer-navigation'
import { useWebViewerNavigationStore } from '@/features/web-viewer/stores/web-viewer-navigation-store'

beforeEach(() => {
  activeBuffer = null
  useWebViewerNavigationStore.setState({ navigationByBufferId: {} })
})

describe('useActiveWebViewerNavigation', () => {
  it('reports a non-webviewer active buffer as not using webviewer navigation', () => {
    activeBuffer = { id: 'b1', type: 'editor' }
    const { result } = renderHook(() => useActiveWebViewerNavigation())
    expect(result.current.usesWebViewerNavigation).toBe(false)
    expect(result.current.activeWebViewerNavigation).toBeUndefined()
  })

  it('returns the webviewer nav entry for a webViewer active buffer', () => {
    activeBuffer = { id: 'b2', type: 'webViewer' }
    useWebViewerNavigationStore.setState({
      navigationByBufferId: {
        b2: {
          url: 'https://x.com',
          canGoBack: true,
          canGoForward: false,
          goBack: () => {},
          goForward: () => {},
          reload: () => {},
        },
      },
    })
    const { result } = renderHook(() => useActiveWebViewerNavigation())
    expect(result.current.usesWebViewerNavigation).toBe(true)
    expect(result.current.activeWebViewerNavigation?.canGoBack).toBe(true)
  })

  it('returns undefined nav when there is no active buffer', () => {
    activeBuffer = null
    const { result } = renderHook(() => useActiveWebViewerNavigation())
    expect(result.current.usesWebViewerNavigation).toBe(false)
    expect(result.current.activeWebViewerNavigation).toBeUndefined()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/__tests__/features/tabs/hooks/use-active-webviewer-navigation.test.ts`
Expected: FAIL — cannot resolve the hook module.

- [ ] **Step 3: Write minimal implementation**

```ts
// web/src/features/tabs/hooks/use-active-webviewer-navigation.ts
import { useActiveWorkspaceBuffer } from '@/features/workspace/hooks/use-active-workspace-buffer'
import {
  useWebViewerNavigationStore,
  type WebViewerNavEntry,
} from '@/features/web-viewer/stores/web-viewer-navigation-store'

/**
 * Derives jump-navigation inputs for the globally-active buffer: whether it is
 * a web viewer, and its live navigation entry (if so). Feeds `useJumpNavigation`
 * from the global sidebar header.
 */
export function useActiveWebViewerNavigation(): {
  usesWebViewerNavigation: boolean
  activeWebViewerNavigation: WebViewerNavEntry | undefined
} {
  const activeBuffer = useActiveWorkspaceBuffer()
  const usesWebViewerNavigation = activeBuffer?.type === 'webViewer'
  const activeWebViewerNavigation = useWebViewerNavigationStore((s) =>
    usesWebViewerNavigation && activeBuffer ? s.navigationByBufferId[activeBuffer.id] : undefined,
  )
  return { usesWebViewerNavigation, activeWebViewerNavigation }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/__tests__/features/tabs/hooks/use-active-webviewer-navigation.test.ts`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/features/tabs/hooks/use-active-webviewer-navigation.ts web/src/__tests__/features/tabs/hooks/use-active-webviewer-navigation.test.ts
git commit -m "feat(tabs): useActiveWebViewerNavigation for global nav"
```

---

## Task 3: Decouple `useJumpNavigation` from React context + make it reactive

**Files:**
- Modify: `web/src/features/tabs/hooks/use-jump-navigation.ts`

Context: the hook currently calls `const workspaceStore = useWorkspaceStore()` (React context) and computes `canGoBack`/`canGoForward` from `jumpListActions.canGoBack()` without subscribing to the jump-list state. The tab bar is the only current caller and will stop using it (Task 6), so this change is safe.

- [ ] **Step 1: Replace the imports and the workspace-store access**

Open `web/src/features/tabs/hooks/use-jump-navigation.ts`.

Replace this import:

```ts
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
```

with:

```ts
import { getActiveWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
```

- [ ] **Step 2: Subscribe to jump-list state for reactive enable/disable**

Replace these lines:

```ts
  const jumpListActions = useJumpListStore.use.actions()
  const workspaceStore = useWorkspaceStore()

  const canGoBack = usesWebViewerNavigation
    ? Boolean(activeWebViewerNavigation?.canGoBack)
    : jumpListActions.canGoBack()

  const canGoForward = usesWebViewerNavigation
    ? Boolean(activeWebViewerNavigation?.canGoForward)
    : jumpListActions.canGoForward()
```

with:

```ts
  const jumpListActions = useJumpListStore.use.actions()
  // Subscribe to jump-list state so canGoBack/canGoForward stay reactive.
  useJumpListStore.use.entries()
  useJumpListStore.use.currentIndex()

  const canGoBack = usesWebViewerNavigation
    ? Boolean(activeWebViewerNavigation?.canGoBack)
    : jumpListActions.canGoBack()

  const canGoForward = usesWebViewerNavigation
    ? Boolean(activeWebViewerNavigation?.canGoForward)
    : jumpListActions.canGoForward()
```

- [ ] **Step 3: Use the registry accessor inside `handleJumpBack`**

Replace the body of `handleJumpBack`'s editor branch. Change:

```ts
    const wsState = workspaceStore.getState()
    const editorState = useEditorStateStore.getState()
```

to:

```ts
    const store = getActiveWorkspaceStore()
    if (!store) return
    const wsState = store.getState()
    const editorState = useEditorStateStore.getState()
```

Then update the `useCallback` dependency array for `handleJumpBack`: remove `workspaceStore`. It becomes:

```ts
  }, [activeWebViewerNavigation, jumpListActions, usesWebViewerNavigation])
```

(`handleJumpForward` does not reference the workspace store; leave it unchanged.)

- [ ] **Step 4: Typecheck**

Run: `cd web && npx tsc --noEmit`
Expected: no errors. (No other file should reference `useWorkspaceStore` via this hook.)

- [ ] **Step 5: Commit**

```bash
git add web/src/features/tabs/hooks/use-jump-navigation.ts
git commit -m "refactor(tabs): decouple useJumpNavigation from workspace React context"
```

---

## Task 4: Rewrite `SidebarProjectHeader` as the Zen-style top bar

**Files:**
- Rewrite: `web/src/components/layout/sidebar-project-header.tsx`
- Test: `web/src/__tests__/components/layout/sidebar-project-header.test.tsx`

- [ ] **Step 1: Write the failing test**

```tsx
// web/src/__tests__/components/layout/sidebar-project-header.test.tsx
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, beforeEach, vi } from 'vitest'

const toggleSidebar = vi.fn()
vi.mock('@/components/ui/sidebar', () => ({
  useSidebar: () => ({ open: true, toggleSidebar }),
}))

const jump = {
  canGoBack: true,
  canGoForward: false,
  handleJumpBack: vi.fn(),
  handleJumpForward: vi.fn(),
}
vi.mock('@/features/tabs/hooks/use-jump-navigation', () => ({
  useJumpNavigation: () => jump,
}))
vi.mock('@/features/tabs/hooks/use-active-webviewer-navigation', () => ({
  useActiveWebViewerNavigation: () => ({
    usesWebViewerNavigation: false,
    activeWebViewerNavigation: undefined,
  }),
}))

const openSettingsDialog = vi.fn()
vi.mock('@/features/window/stores/ui-state-store', () => ({
  useUIState: Object.assign(() => undefined, {
    getState: () => ({ openSettingsDialog }),
  }),
}))

let sidebarPosition: 'left' | 'right' = 'left'
vi.mock('@/features/settings/store', () => ({
  useSettingsStore: (sel: (s: unknown) => unknown) =>
    sel({ settings: { sidebarPosition } }),
}))

import { SidebarProjectHeader } from '@/components/layout/sidebar-project-header'

beforeEach(() => {
  sidebarPosition = 'left'
  toggleSidebar.mockClear()
  openSettingsDialog.mockClear()
  jump.handleJumpBack.mockClear()
  jump.handleJumpForward.mockClear()
})

describe('SidebarProjectHeader', () => {
  it('renders toggle, back, forward, and settings', () => {
    render(<SidebarProjectHeader />)
    expect(screen.getByRole('button', { name: /sidebar/i })).toBeTruthy()
    expect(screen.getByRole('button', { name: /go back/i })).toBeTruthy()
    expect(screen.getByRole('button', { name: /go forward/i })).toBeTruthy()
    expect(screen.getByRole('button', { name: /settings/i })).toBeTruthy()
  })

  it('toggles the sidebar', async () => {
    render(<SidebarProjectHeader />)
    await userEvent.click(screen.getByRole('button', { name: /sidebar/i }))
    expect(toggleSidebar).toHaveBeenCalledOnce()
  })

  it('opens settings', async () => {
    render(<SidebarProjectHeader />)
    await userEvent.click(screen.getByRole('button', { name: /settings/i }))
    expect(openSettingsDialog).toHaveBeenCalledOnce()
  })

  it('disables forward when canGoForward is false and runs back when enabled', async () => {
    render(<SidebarProjectHeader />)
    expect(screen.getByRole('button', { name: /go forward/i })).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: /go back/i }))
    expect(jump.handleJumpBack).toHaveBeenCalledOnce()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/__tests__/components/layout/sidebar-project-header.test.tsx`
Expected: FAIL — the current header has no back/forward/toggle buttons.

- [ ] **Step 3: Write the implementation**

```tsx
// web/src/components/layout/sidebar-project-header.tsx
import { ArrowLeft, ArrowRight, GearSix, SidebarSimple } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { useSidebar } from '@/components/ui/sidebar'
import { useSettingsStore } from '@/features/settings/store'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { useJumpNavigation } from '@/features/tabs/hooks/use-jump-navigation'
import { useActiveWebViewerNavigation } from '@/features/tabs/hooks/use-active-webviewer-navigation'
import { IS_MAC } from '@/utils/platform'
import { cn } from '@/utils/cn'

/**
 * Zen-style sidebar top bar: a sidebar-toggle on the leading edge and a
 * back / forward / settings cluster on the trailing edge. Mirrors when the
 * sidebar sits on the right. Back/forward reuse the editor jump navigation.
 */
export function SidebarProjectHeader() {
  const sidebarPosition = useSettingsStore((s) => s.settings.sidebarPosition)
  const isRight = sidebarPosition === 'right'
  const { open: sidebarOpen, toggleSidebar } = useSidebar()
  const { canGoBack, canGoForward, handleJumpBack, handleJumpForward } = useJumpNavigation(
    useActiveWebViewerNavigation(),
  )

  const toggle = (
    <Button
      onClick={toggleSidebar}
      variant="ghost"
      size="icon-sm"
      className={cn('shrink-0 text-muted-foreground', isRight && 'scale-x-[-1]')}
      tooltip={sidebarOpen ? 'Hide Sidebar' : 'Show Sidebar'}
      tooltipSide="bottom"
      aria-label={sidebarOpen ? 'Hide sidebar' : 'Show sidebar'}
    >
      <SidebarSimple size={16} />
    </Button>
  )

  const cluster = (
    <div className="flex shrink-0 items-center gap-0.5">
      <Button
        onClick={() => void handleJumpBack()}
        disabled={!canGoBack}
        variant="ghost"
        size="icon-sm"
        className="shrink-0 text-muted-foreground"
        tooltip="Go Back"
        tooltipSide="bottom"
        aria-label="Go back to previous location"
      >
        <ArrowLeft size={16} />
      </Button>
      <Button
        onClick={() => void handleJumpForward()}
        disabled={!canGoForward}
        variant="ghost"
        size="icon-sm"
        className="shrink-0 text-muted-foreground"
        tooltip="Go Forward"
        tooltipSide="bottom"
        aria-label="Go forward to next location"
      >
        <ArrowRight size={16} />
      </Button>
      <Button
        onClick={() => useUIState.getState().openSettingsDialog()}
        variant="ghost"
        size="icon-sm"
        className="shrink-0 text-muted-foreground"
        tooltip="Settings"
        tooltipSide="bottom"
        aria-label="Settings"
      >
        <GearSix size={16} />
      </Button>
    </div>
  )

  return (
    <div
      className={cn(
        'flex w-full flex-shrink-0 items-center gap-1 px-3',
        IS_MAC ? 'h-[44px]' : 'h-[34px]',
        isRight && 'flex-row-reverse',
      )}
      data-tauri-drag-region
    >
      {/* Reserve space for the macOS traffic lights on whichever side is
          top-left (only when the sidebar is on the left). */}
      {IS_MAC && !isRight && <div className="w-[52px] shrink-0" />}
      {toggle}
      <div className="flex-1" />
      {cluster}
    </div>
  )
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/__tests__/components/layout/sidebar-project-header.test.tsx`
Expected: PASS (4 tests).

- [ ] **Step 5: Typecheck**

Run: `cd web && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add web/src/components/layout/sidebar-project-header.tsx web/src/__tests__/components/layout/sidebar-project-header.test.tsx
git commit -m "feat(sidebar): zen-style top bar (toggle + back/forward + settings)"
```

---

## Task 5: Strip back/forward from `TabNavigationButtons`

**Files:**
- Modify: `web/src/features/tabs/components/tab-navigation-buttons.tsx`

- [ ] **Step 1: Replace the file with the toggle-only version**

```tsx
// web/src/features/tabs/components/tab-navigation-buttons.tsx
import { SidebarSimple as PanelLeftClose } from '@phosphor-icons/react'
import React from 'react'
import { Button } from '@/components/ui/button'
import { cn } from '@/utils/cn'

interface TabNavigationButtonsProps {
  isBottomPane: boolean
  sidebarOpen: boolean
  sidebarPosition: 'left' | 'right'
  onToggleSidebar: () => void
}

const TabNavigationButtons = React.memo(function TabNavigationButtons({
  isBottomPane,
  sidebarOpen,
  sidebarPosition,
  onToggleSidebar,
}: TabNavigationButtonsProps) {
  if (isBottomPane) return null
  return (
    <Button
      onClick={onToggleSidebar}
      variant="ghost"
      size="icon-xs"
      className={cn(
        'shrink-0 text-muted-foreground',
        sidebarPosition === 'right' && 'scale-x-[-1]',
      )}
      tooltip={sidebarOpen ? 'Hide Sidebar' : 'Show Sidebar'}
      tooltipSide="bottom"
      aria-label={sidebarOpen ? 'Hide sidebar' : 'Show sidebar'}
    >
      <PanelLeftClose size={14} />
    </Button>
  )
})

export default TabNavigationButtons
```

- [ ] **Step 2: Typecheck (expected to FAIL at the tab-bar call site)**

Run: `cd web && npx tsc --noEmit`
Expected: errors in `tab-bar.tsx` about removed props (`canGoBack`, `canGoForward`, `onJumpBack`, `onJumpForward`). This is fixed in Task 6.

- [ ] **Step 3: Commit (paired with Task 6)**

Do not commit yet — `tsc` is failing. Proceed directly to Task 6, then commit Tasks 5+6 together.

---

## Task 6: Remove jump-nav usage from `tab-bar.tsx`

**Files:**
- Modify: `web/src/features/tabs/components/tab-bar.tsx`

- [ ] **Step 1: Remove the jump-nav derivations**

In `web/src/features/tabs/components/tab-bar.tsx`, delete these lines (around 159-171):

```tsx
  const activeBuffer = useMemo(
    () => buffers.find((buffer) => buffer.id === activeBufferId) ?? null,
    [activeBufferId, buffers],
  )
  const activeWebViewerNavigation = useWebViewerNavigationStore((state) =>
    activeBuffer?.type === 'webViewer' ? state.navigationByBufferId[activeBuffer.id] : undefined,
  )
  const usesWebViewerNavigation = activeBuffer?.type === 'webViewer'
  const { canGoBack, canGoForward, handleJumpBack, handleJumpForward } = useJumpNavigation({
    usesWebViewerNavigation,
    activeWebViewerNavigation,
  })
```

IMPORTANT: `activeBuffer` may be used elsewhere in the file. Before deleting it, run `grep -n "activeBuffer" web/src/features/tabs/components/tab-bar.tsx`. If `activeBuffer` is referenced outside the deleted block, keep its `useMemo` declaration and only delete the three nav-related lines (`activeWebViewerNavigation`, `usesWebViewerNavigation`, the `useJumpNavigation` call).

- [ ] **Step 2: Remove now-unused imports**

Remove the `useJumpNavigation` import and the `useWebViewerNavigationStore` import **only if** no longer referenced (re-grep both after Step 1). Leave `useMemo` import if still used elsewhere.

- [ ] **Step 3: Update the `TabNavigationButtons` call site**

Find the `<TabNavigationButtons ... />` usage (around line 430) and remove the four removed props, keeping the rest:

```tsx
            <TabNavigationButtons
              isBottomPane={isBottomPane}
              sidebarOpen={sidebarOpen}
              sidebarPosition={sidebarPosition}
              onToggleSidebar={toggleSidebar}
            />
```

(Delete `canGoBack`, `canGoForward`, `onJumpBack`, `onJumpForward` props.)

- [ ] **Step 4: Typecheck + lint**

Run: `cd web && npx tsc --noEmit && npx eslint src/features/tabs/components/tab-bar.tsx src/features/tabs/components/tab-navigation-buttons.tsx`
Expected: no errors (no unused vars/imports).

- [ ] **Step 5: Commit (Tasks 5 + 6 together)**

```bash
git add web/src/features/tabs/components/tab-navigation-buttons.tsx web/src/features/tabs/components/tab-bar.tsx
git commit -m "refactor(tabs): move back/forward to sidebar top bar, keep tab-bar toggle"
```

---

## Task 7: Full verification

- [ ] **Step 1: Run the full web test suite**

Run: `cd web && npx vitest run`
Expected: all tests pass except the 3 known-pre-existing `semantic-tokens-provider.test.ts` failures (unrelated to this work).

- [ ] **Step 2: Typecheck**

Run: `cd web && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 3: Lint the changed files**

Run: `cd web && npx eslint src/components/layout/sidebar-project-header.tsx src/features/tabs/hooks/use-jump-navigation.ts src/features/tabs/hooks/use-active-webviewer-navigation.ts src/features/workspace/hooks/use-active-workspace-buffer.ts src/features/tabs/components/tab-navigation-buttons.tsx src/features/tabs/components/tab-bar.tsx`
Expected: no errors.

- [ ] **Step 4: Manual check in Tauri (verify live before claiming done)**

Confirm:
- Sidebar header shows: `[toggle] ··· [back] [forward] [settings]` (left sidebar), traffic lights clear of the toggle.
- Switch sidebar to the right (settings) → the bar mirrors: cluster on the left, toggle on the right; toggle icon flips.
- Back/forward enable/disable correctly while navigating editor locations; open a web viewer and confirm its back/forward drive the page.
- The tab bar no longer shows back/forward, but still shows the sidebar-toggle; collapsing the sidebar then reopening via the tab-bar toggle works.
- Settings button opens the settings dialog.

---

## Notes for the implementer

- Conventions: kebab-case files, PascalCase exports, narrow Zustand selectors in render, `getState()` only in handlers, `@/` imports in tests, CSS-variable tokens only.
- Do not change `tauri.conf.json` or any Rust — traffic lights stay top-left by design.
- `useSyncExternalStore` getSnapshot must return a stable reference when unchanged: `buffers.find(...)` returns the stored buffer object (stable under Zustand/immer structural sharing), and `null` when absent — do not wrap it in a new object.
- The `useJumpListStore.use.entries()` / `.use.currentIndex()` calls in Task 3 exist purely to subscribe for reactive enable/disable; keep them even though their return values are unused.
