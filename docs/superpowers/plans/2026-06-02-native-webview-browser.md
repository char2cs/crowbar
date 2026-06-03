# Native Webview Browser Pane — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the `<iframe>`-based web viewer with per-buffer native Tauri child webviews so sites that set `X-Frame-Options: SAMEORIGIN` load correctly.

**Architecture:** Each browser buffer gets its own Tauri `Webview` child of the main window. React's `WebViewerPane` renders a transparent anchor `<div>` whose pixel geometry is synced to the native webview via the `crowbar-bridge.ts` abstraction layer. Navigation events flow back via Tauri events to update the address bar and back/forward buttons.

**Tech Stack:** React 18, Zustand, Tauri v2 (`tauri::Webview`, `add_child`, `eval`), Vitest + Testing Library, Rust (tokio, serde_json)

---

## File Map

| Status | Path | What changes |
|---|---|---|
| Modify | `web/src/features/web-viewer/stores/web-viewer-navigation-store.ts` | Full rewrite: per-buffer state + `registerBuffer` / `updateNavState` / `removeBuffer` actions |
| Modify | `web/src/lib/crowbar-bridge.ts` | Add `isTauri()` helper + 6 `browserPane*` bridge functions |
| Create | `web/src/features/web-viewer/hooks/use-browser-pane-anchor.ts` | ResizeObserver hook that syncs anchor rect → bridge |
| Create | `web/src/features/web-viewer/components/browser-pane-event-listener.tsx` | Root-level component: listens for `browser-pane-navigated` Tauri events → store |
| Modify | `web/src/features/workspace/components/WorkspaceView.tsx` | Mount `BrowserPaneEventListener` inside `WorkspaceViewInner` |
| Modify | `web/src/features/web-viewer/components/web-viewer.tsx` | Replace iframe with anchor div; wire nav store + bridge; add non-Tauri fallback |
| Create | `desktop/src-tauri/src/browser_pane.rs` | `BrowserPaneManager` state + 7 Tauri commands |
| Modify | `desktop/src-tauri/src/lib.rs` | Register `mod browser_pane` + all commands |
| Create | `web/src/__tests__/features/web-viewer/stores/web-viewer-navigation-store.test.ts` | Store unit tests |
| Create | `web/src/__tests__/lib/crowbar-bridge-browser-pane.test.ts` | Bridge stub tests |
| Create | `web/src/__tests__/features/web-viewer/hooks/use-browser-pane-anchor.test.ts` | Hook unit tests |
| Create | `web/src/__tests__/features/web-viewer/components/web-viewer.test.tsx` | Component render tests |

---

## Task 1: Rewrite the navigation store

**Files:**
- Modify: `web/src/features/web-viewer/stores/web-viewer-navigation-store.ts`
- Create: `web/src/__tests__/features/web-viewer/stores/web-viewer-navigation-store.test.ts`

The existing store is a stub. This task replaces it with a working per-buffer registry. Each entry holds `url`, `canGoBack`, `canGoForward`, and stable `goBack` / `goForward` / `reload` functions (which call bridge functions). Two separate actions keep the two concerns separate: `registerBuffer` sets up functions, `updateNavState` updates URL and flags from Tauri events.

The `goBack`, `goForward`, and `reload` functions in each entry are what `useJumpNavigation` (in `features/tabs/hooks/use-jump-navigation.ts`) calls when the user clicks the back/forward buttons in the tab bar. They must be present in the store entry — not in component state — so the tab bar can reach them.

- [ ] **Step 1.1: Write the failing tests**

Create `web/src/__tests__/features/web-viewer/stores/web-viewer-navigation-store.test.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'

// Mock the bridge BEFORE importing the store (the store imports bridge at module level)
vi.mock('@/lib/crowbar-bridge', () => ({
  browserPaneGoBack: vi.fn().mockResolvedValue(undefined),
  browserPaneGoForward: vi.fn().mockResolvedValue(undefined),
  browserPaneReload: vi.fn().mockResolvedValue(undefined),
}))

import {
  useWebViewerNavigationStore,
} from '@/features/web-viewer/stores/web-viewer-navigation-store'
import { browserPaneGoBack, browserPaneGoForward, browserPaneReload } from '@/lib/crowbar-bridge'

beforeEach(() => {
  useWebViewerNavigationStore.setState({ navigationByBufferId: {} })
  vi.clearAllMocks()
})

describe('registerBuffer', () => {
  it('creates an entry with initial url and false flags', () => {
    useWebViewerNavigationStore.getState().registerBuffer('buf-1', 'https://example.com')
    const entry = useWebViewerNavigationStore.getState().navigationByBufferId['buf-1']
    expect(entry.url).toBe('https://example.com')
    expect(entry.canGoBack).toBe(false)
    expect(entry.canGoForward).toBe(false)
  })

  it('goBack calls browserPaneGoBack with the bufferId', () => {
    useWebViewerNavigationStore.getState().registerBuffer('buf-1', 'https://example.com')
    useWebViewerNavigationStore.getState().navigationByBufferId['buf-1'].goBack()
    expect(browserPaneGoBack).toHaveBeenCalledWith('buf-1')
  })

  it('goForward calls browserPaneGoForward with the bufferId', () => {
    useWebViewerNavigationStore.getState().registerBuffer('buf-1', 'https://example.com')
    useWebViewerNavigationStore.getState().navigationByBufferId['buf-1'].goForward()
    expect(browserPaneGoForward).toHaveBeenCalledWith('buf-1')
  })

  it('reload calls browserPaneReload with the bufferId', () => {
    useWebViewerNavigationStore.getState().registerBuffer('buf-1', 'https://example.com')
    useWebViewerNavigationStore.getState().navigationByBufferId['buf-1'].reload()
    expect(browserPaneReload).toHaveBeenCalledWith('buf-1')
  })
})

describe('updateNavState', () => {
  it('updates url, canGoBack, canGoForward without touching functions', () => {
    useWebViewerNavigationStore.getState().registerBuffer('buf-1', 'https://example.com')
    useWebViewerNavigationStore.getState().updateNavState('buf-1', {
      url: 'https://new.example.com',
      canGoBack: true,
      canGoForward: false,
    })
    const entry = useWebViewerNavigationStore.getState().navigationByBufferId['buf-1']
    expect(entry.url).toBe('https://new.example.com')
    expect(entry.canGoBack).toBe(true)
    expect(entry.canGoForward).toBe(false)
    // goBack function still works
    entry.goBack()
    expect(browserPaneGoBack).toHaveBeenCalledWith('buf-1')
  })

  it('creates an entry if buffer was not registered', () => {
    useWebViewerNavigationStore.getState().updateNavState('buf-x', {
      url: 'https://surprise.com',
      canGoBack: false,
      canGoForward: true,
    })
    const entry = useWebViewerNavigationStore.getState().navigationByBufferId['buf-x']
    expect(entry.url).toBe('https://surprise.com')
  })
})

describe('removeBuffer', () => {
  it('deletes the buffer entry', () => {
    useWebViewerNavigationStore.getState().registerBuffer('buf-1', 'https://example.com')
    useWebViewerNavigationStore.getState().removeBuffer('buf-1')
    expect(useWebViewerNavigationStore.getState().navigationByBufferId['buf-1']).toBeUndefined()
  })
})
```

- [ ] **Step 1.2: Run tests to confirm they fail**

```bash
cd web && npm run test -- --run src/__tests__/features/web-viewer/stores/web-viewer-navigation-store.test.ts
```

Expected: FAIL — `registerBuffer`, `updateNavState`, `removeBuffer` are not defined.

- [ ] **Step 1.3: Rewrite the navigation store**

Replace `web/src/features/web-viewer/stores/web-viewer-navigation-store.ts` entirely:

```ts
import { create } from 'zustand'
import {
  browserPaneGoBack,
  browserPaneGoForward,
  browserPaneReload,
} from '@/lib/crowbar-bridge'

export interface WebViewerNavEntry {
  url: string
  canGoBack: boolean
  canGoForward: boolean
  goBack: () => void
  goForward: () => void
  reload: () => void
}

interface WebViewerNavState {
  navigationByBufferId: Record<string, WebViewerNavEntry>
  registerBuffer: (bufferId: string, initialUrl: string) => void
  updateNavState: (
    bufferId: string,
    state: { url: string; canGoBack: boolean; canGoForward: boolean },
  ) => void
  removeBuffer: (bufferId: string) => void
}

export const useWebViewerNavigationStore = create<WebViewerNavState>((set, get) => ({
  navigationByBufferId: {},

  registerBuffer(bufferId, initialUrl) {
    set(state => ({
      navigationByBufferId: {
        ...state.navigationByBufferId,
        [bufferId]: {
          url: initialUrl,
          canGoBack: false,
          canGoForward: false,
          goBack: () => void browserPaneGoBack(bufferId),
          goForward: () => void browserPaneGoForward(bufferId),
          reload: () => void browserPaneReload(bufferId),
        },
      },
    }))
  },

  updateNavState(bufferId, { url, canGoBack, canGoForward }) {
    const existing = get().navigationByBufferId[bufferId]
    set(state => ({
      navigationByBufferId: {
        ...state.navigationByBufferId,
        [bufferId]: {
          url,
          canGoBack,
          canGoForward,
          goBack: existing?.goBack ?? (() => void browserPaneGoBack(bufferId)),
          goForward: existing?.goForward ?? (() => void browserPaneGoForward(bufferId)),
          reload: existing?.reload ?? (() => void browserPaneReload(bufferId)),
        },
      },
    }))
  },

  removeBuffer(bufferId) {
    set(state => {
      const next = { ...state.navigationByBufferId }
      delete next[bufferId]
      return { navigationByBufferId: next }
    })
  },
}))
```

- [ ] **Step 1.4: Run tests to confirm they pass**

```bash
cd web && npm run test -- --run src/__tests__/features/web-viewer/stores/web-viewer-navigation-store.test.ts
```

Expected: all 7 tests PASS.

- [ ] **Step 1.5: Commit**

```bash
git add web/src/features/web-viewer/stores/web-viewer-navigation-store.ts \
        web/src/__tests__/features/web-viewer/stores/web-viewer-navigation-store.test.ts
git commit -m "feat(web-viewer): rewrite navigation store with per-buffer actions"
```

---

## Task 2: Add browser pane bridge functions

**Files:**
- Modify: `web/src/lib/crowbar-bridge.ts`
- Create: `web/src/__tests__/lib/crowbar-bridge-browser-pane.test.ts`

Adds `isTauri()` detection and six `browserPane*` functions. In non-Tauri environments all functions are stubs that resolve immediately. In Tauri, they call `@tauri-apps/api/core`'s `invoke`. The `@tauri-apps/api` package is imported lazily inside the `isTauri` branch — this keeps the bundle clean for non-Tauri builds.

- [ ] **Step 2.1: Write the failing tests**

Create `web/src/__tests__/lib/crowbar-bridge-browser-pane.test.ts`:

```ts
import { describe, it, expect, beforeEach } from 'vitest'

// Ensure we're in non-Tauri environment (jsdom has no __TAURI_INTERNALS__)
// These tests verify the stub paths.

beforeEach(() => {
  // Make sure __TAURI_INTERNALS__ is absent
  delete (window as unknown as Record<string, unknown>)['__TAURI_INTERNALS__']
})

describe('isTauri (non-Tauri env)', () => {
  it('returns false when __TAURI_INTERNALS__ is absent', async () => {
    const { isTauri } = await import('@/lib/crowbar-bridge')
    expect(isTauri()).toBe(false)
  })
})

describe('browserPane stubs (non-Tauri env)', () => {
  it('browserPaneSync resolves without error', async () => {
    const { browserPaneSync } = await import('@/lib/crowbar-bridge')
    await expect(
      browserPaneSync('buf-1', { x: 0, y: 0, width: 800, height: 600 }, true),
    ).resolves.toBeUndefined()
  })

  it('browserPaneNavigate resolves without error', async () => {
    const { browserPaneNavigate } = await import('@/lib/crowbar-bridge')
    await expect(browserPaneNavigate('buf-1', 'https://example.com')).resolves.toBeUndefined()
  })

  it('browserPaneGoBack resolves without error', async () => {
    const { browserPaneGoBack } = await import('@/lib/crowbar-bridge')
    await expect(browserPaneGoBack('buf-1')).resolves.toBeUndefined()
  })

  it('browserPaneGoForward resolves without error', async () => {
    const { browserPaneGoForward } = await import('@/lib/crowbar-bridge')
    await expect(browserPaneGoForward('buf-1')).resolves.toBeUndefined()
  })

  it('browserPaneReload resolves without error', async () => {
    const { browserPaneReload } = await import('@/lib/crowbar-bridge')
    await expect(browserPaneReload('buf-1')).resolves.toBeUndefined()
  })

  it('browserPaneClose resolves without error', async () => {
    const { browserPaneClose } = await import('@/lib/crowbar-bridge')
    await expect(browserPaneClose('buf-1')).resolves.toBeUndefined()
  })
})
```

- [ ] **Step 2.2: Run tests to confirm they fail**

```bash
cd web && npm run test -- --run src/__tests__/lib/crowbar-bridge-browser-pane.test.ts
```

Expected: FAIL — `isTauri`, `browserPaneSync`, etc. are not exported.

- [ ] **Step 2.3: Add bridge functions to crowbar-bridge.ts**

Append to the bottom of `web/src/lib/crowbar-bridge.ts`:

```ts
// ── Browser Pane (native child webview) ──────────────────────────────────────

export function isTauri(): boolean {
  return typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window
}

async function tauriInvoke(cmd: string, args?: Record<string, unknown>): Promise<void> {
  const { invoke } = await import('@tauri-apps/api/core')
  await invoke(cmd, args)
}

export async function browserPaneSync(
  bufferId: string,
  rect: { x: number; y: number; width: number; height: number },
  visible: boolean,
): Promise<void> {
  if (!isTauri()) return
  await tauriInvoke('browser_pane_sync', {
    bufferId,
    x: rect.x,
    y: rect.y,
    width: rect.width,
    height: rect.height,
    visible,
  })
}

export async function browserPaneNavigate(bufferId: string, url: string): Promise<void> {
  if (!isTauri()) return
  await tauriInvoke('browser_pane_navigate', { bufferId, url })
}

export async function browserPaneGoBack(bufferId: string): Promise<void> {
  if (!isTauri()) return
  await tauriInvoke('browser_pane_go_back', { bufferId })
}

export async function browserPaneGoForward(bufferId: string): Promise<void> {
  if (!isTauri()) return
  await tauriInvoke('browser_pane_go_forward', { bufferId })
}

export async function browserPaneReload(bufferId: string): Promise<void> {
  if (!isTauri()) return
  await tauriInvoke('browser_pane_reload', { bufferId })
}

export async function browserPaneClose(bufferId: string): Promise<void> {
  if (!isTauri()) return
  await tauriInvoke('browser_pane_close', { bufferId })
}
```

- [ ] **Step 2.4: Run tests to confirm they pass**

```bash
cd web && npm run test -- --run src/__tests__/lib/crowbar-bridge-browser-pane.test.ts
```

Expected: all 7 tests PASS.

- [ ] **Step 2.5: Commit**

```bash
git add web/src/lib/crowbar-bridge.ts \
        web/src/__tests__/lib/crowbar-bridge-browser-pane.test.ts
git commit -m "feat(bridge): add browser pane bridge functions and isTauri helper"
```

---

## Task 3: Create the `use-browser-pane-anchor` hook

**Files:**
- Create: `web/src/features/web-viewer/hooks/use-browser-pane-anchor.ts`
- Create: `web/src/__tests__/features/web-viewer/hooks/use-browser-pane-anchor.test.ts`

The hook owns the native webview's lifecycle for one browser buffer. It attaches a `ResizeObserver` to the anchor div, debounces via `requestAnimationFrame`, and calls `browserPaneSync` whenever geometry changes. On unmount it calls `browserPaneClose`. A separate `useEffect` re-syncs whenever `isVisible` changes (tab switch).

- [ ] **Step 3.1: Write the failing tests**

Create `web/src/__tests__/features/web-viewer/hooks/use-browser-pane-anchor.test.ts`:

```ts
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { createRef } from 'react'

vi.mock('@/lib/crowbar-bridge', () => ({
  isTauri: vi.fn(),
  browserPaneSync: vi.fn().mockResolvedValue(undefined),
  browserPaneClose: vi.fn().mockResolvedValue(undefined),
}))

import { isTauri, browserPaneSync, browserPaneClose } from '@/lib/crowbar-bridge'
import { useBrowserPaneAnchor } from '@/features/web-viewer/hooks/use-browser-pane-anchor'

// Minimal ResizeObserver mock
let resizeCallback: ResizeObserverCallback | null = null
class MockResizeObserver {
  constructor(cb: ResizeObserverCallback) { resizeCallback = cb }
  observe() {}
  disconnect() { resizeCallback = null }
}

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', MockResizeObserver)
  vi.clearAllMocks()
  // Mock requestAnimationFrame to run synchronously
  vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => { cb(0); return 0 })
  vi.stubGlobal('cancelAnimationFrame', vi.fn())
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('useBrowserPaneAnchor — non-Tauri env', () => {
  it('returns isTauri=false and does not call browserPaneSync', () => {
    ;(isTauri as ReturnType<typeof vi.fn>).mockReturnValue(false)
    const ref = createRef<HTMLDivElement>()
    const { result } = renderHook(() =>
      useBrowserPaneAnchor({ bufferId: 'b1', isVisible: true, anchorRef: ref }),
    )
    expect(result.current.isTauri).toBe(false)
    expect(browserPaneSync).not.toHaveBeenCalled()
  })
})

describe('useBrowserPaneAnchor — Tauri env', () => {
  beforeEach(() => {
    ;(isTauri as ReturnType<typeof vi.fn>).mockReturnValue(true)
  })

  it('returns isTauri=true', () => {
    const ref = createRef<HTMLDivElement>()
    const { result } = renderHook(() =>
      useBrowserPaneAnchor({ bufferId: 'b1', isVisible: true, anchorRef: ref }),
    )
    expect(result.current.isTauri).toBe(true)
  })

  it('calls browserPaneSync on mount with visible=true', () => {
    const div = document.createElement('div')
    div.getBoundingClientRect = () =>
      ({ x: 10, y: 20, width: 400, height: 300 } as DOMRect)
    const ref = { current: div }
    renderHook(() =>
      useBrowserPaneAnchor({ bufferId: 'b1', isVisible: true, anchorRef: ref }),
    )
    expect(browserPaneSync).toHaveBeenCalledWith(
      'b1',
      { x: 10, y: 20, width: 400, height: 300 },
      true,
    )
  })

  it('calls browserPaneSync with visible=false when isVisible changes to false', () => {
    const div = document.createElement('div')
    div.getBoundingClientRect = () =>
      ({ x: 0, y: 0, width: 100, height: 100 } as DOMRect)
    const ref = { current: div }
    const { rerender } = renderHook(
      ({ visible }: { visible: boolean }) =>
        useBrowserPaneAnchor({ bufferId: 'b1', isVisible: visible, anchorRef: ref }),
      { initialProps: { visible: true } },
    )
    vi.clearAllMocks()
    rerender({ visible: false })
    expect(browserPaneSync).toHaveBeenCalledWith('b1', expect.any(Object), false)
  })

  it('calls browserPaneClose on unmount', () => {
    const ref = createRef<HTMLDivElement>()
    const { unmount } = renderHook(() =>
      useBrowserPaneAnchor({ bufferId: 'b1', isVisible: true, anchorRef: ref }),
    )
    unmount()
    expect(browserPaneClose).toHaveBeenCalledWith('b1')
  })
})
```

- [ ] **Step 3.2: Run tests to confirm they fail**

```bash
cd web && npm run test -- --run src/__tests__/features/web-viewer/hooks/use-browser-pane-anchor.test.ts
```

Expected: FAIL — `useBrowserPaneAnchor` does not exist.

- [ ] **Step 3.3: Create the hook**

Create `web/src/features/web-viewer/hooks/use-browser-pane-anchor.ts`:

```ts
import { type RefObject, useEffect, useRef } from 'react'
import { isTauri, browserPaneSync, browserPaneClose } from '@/lib/crowbar-bridge'

interface Options {
  bufferId: string
  isVisible: boolean
  anchorRef: RefObject<HTMLDivElement | null>
}

export function useBrowserPaneAnchor({ bufferId, isVisible, anchorRef }: Options): {
  isTauri: boolean
} {
  const isTauriEnv = useRef(isTauri())

  // Main effect: set up ResizeObserver + window resize listener.
  // Runs once per bufferId (bufferId is stable for the lifetime of a pane).
  useEffect(() => {
    if (!isTauriEnv.current) return

    let rafId: number | null = null

    function sync() {
      const el = anchorRef.current
      if (!el) return
      const r = el.getBoundingClientRect()
      void browserPaneSync(bufferId, { x: r.x, y: r.y, width: r.width, height: r.height }, isVisible)
    }

    function scheduleSync() {
      if (rafId !== null) cancelAnimationFrame(rafId)
      rafId = requestAnimationFrame(() => {
        rafId = null
        sync()
      })
    }

    const ro = new ResizeObserver(scheduleSync)
    if (anchorRef.current) ro.observe(anchorRef.current)
    window.addEventListener('resize', scheduleSync)

    // Initial sync
    scheduleSync()

    return () => {
      if (rafId !== null) cancelAnimationFrame(rafId)
      ro.disconnect()
      window.removeEventListener('resize', scheduleSync)
      void browserPaneClose(bufferId)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [bufferId])

  // Separate effect for isVisible: re-sync the visible flag without re-mounting
  // the observer. Uses the latest anchorRef position.
  useEffect(() => {
    if (!isTauriEnv.current) return
    const el = anchorRef.current
    if (!el) return
    const r = el.getBoundingClientRect()
    void browserPaneSync(bufferId, { x: r.x, y: r.y, width: r.width, height: r.height }, isVisible)
  }, [bufferId, isVisible, anchorRef])

  return { isTauri: isTauriEnv.current }
}
```

- [ ] **Step 3.4: Run tests to confirm they pass**

```bash
cd web && npm run test -- --run src/__tests__/features/web-viewer/hooks/use-browser-pane-anchor.test.ts
```

Expected: all 5 tests PASS.

- [ ] **Step 3.5: Commit**

```bash
git add web/src/features/web-viewer/hooks/use-browser-pane-anchor.ts \
        web/src/__tests__/features/web-viewer/hooks/use-browser-pane-anchor.test.ts
git commit -m "feat(web-viewer): add use-browser-pane-anchor hook"
```

---

## Task 4: Create `BrowserPaneEventListener` component

**Files:**
- Create: `web/src/features/web-viewer/components/browser-pane-event-listener.tsx`

This component has no visible output. It mounts once at the app root and subscribes to the `browser-pane-navigated` Tauri event, writing each payload into the navigation store. It uses a dynamic import of `@tauri-apps/api/event` so it doesn't break non-Tauri builds (the `isTauri()` guard runs first).

- [ ] **Step 4.1: Create the component**

Create `web/src/features/web-viewer/components/browser-pane-event-listener.tsx`:

```tsx
import { useEffect } from 'react'
import { isTauri } from '@/lib/crowbar-bridge'
import { useWebViewerNavigationStore } from '@/features/web-viewer/stores/web-viewer-navigation-store'

interface BrowserPaneNavigatedPayload {
  bufferId: string
  url: string
  canGoBack: boolean
  canGoForward: boolean
}

export function BrowserPaneEventListener() {
  useEffect(() => {
    if (!isTauri()) return

    let unlisten: (() => void) | null = null

    import('@tauri-apps/api/event').then(({ listen }) => {
      listen<BrowserPaneNavigatedPayload>('browser-pane-navigated', event => {
        const { bufferId, url, canGoBack, canGoForward } = event.payload
        useWebViewerNavigationStore
          .getState()
          .updateNavState(bufferId, { url, canGoBack, canGoForward })
      }).then(fn => {
        unlisten = fn
      })
    })

    return () => {
      unlisten?.()
    }
  }, [])

  return null
}
```

- [ ] **Step 4.2: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit 2>&1 | grep browser-pane-event-listener
```

Expected: no output (no errors for this file).

- [ ] **Step 4.3: Commit**

```bash
git add web/src/features/web-viewer/components/browser-pane-event-listener.tsx
git commit -m "feat(web-viewer): add BrowserPaneEventListener for Tauri nav events"
```

---

## Task 5: Mount `BrowserPaneEventListener` at the app root

**Files:**
- Modify: `web/src/features/workspace/components/WorkspaceView.tsx`

`WorkspaceViewInner` is the right mount point — it renders after hydration is complete, meaning the workspace store context is available. `BrowserPaneEventListener` doesn't use the workspace store, but mounting it here avoids creating a separate global component. It only needs to be mounted once per workspace instance, which is correct since each workspace has its own pane layout.

- [ ] **Step 5.1: Add the import and component to WorkspaceView.tsx**

In `web/src/features/workspace/components/WorkspaceView.tsx`, change `WorkspaceViewInner`:

```tsx
// Add import at top of file (alongside existing imports):
import { BrowserPaneEventListener } from '@/features/web-viewer/components/browser-pane-event-listener'

// Change WorkspaceViewInner from:
function WorkspaceViewInner({ wsId }: Pick<WorkspaceViewProps, 'wsId'>) {
  useWorkspaceEffects(wsId)
  return <WorkspaceLayoutRoot />
}

// To:
function WorkspaceViewInner({ wsId }: Pick<WorkspaceViewProps, 'wsId'>) {
  useWorkspaceEffects(wsId)
  return (
    <>
      <BrowserPaneEventListener />
      <WorkspaceLayoutRoot />
    </>
  )
}
```

- [ ] **Step 5.2: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit 2>&1 | grep WorkspaceView
```

Expected: no output.

- [ ] **Step 5.3: Commit**

```bash
git add web/src/features/workspace/components/WorkspaceView.tsx
git commit -m "feat(workspace): mount BrowserPaneEventListener at workspace root"
```

---

## Task 6: Rewrite `web-viewer.tsx`

**Files:**
- Modify: `web/src/features/web-viewer/components/web-viewer.tsx`
- Create: `web/src/__tests__/features/web-viewer/components/web-viewer.test.tsx`

The iframe is replaced with a transparent anchor `<div>` whose geometry drives the native webview. The nav bar is unchanged in appearance. When `isTauri` is false, a "requires desktop app" message is shown instead of the anchor. The address bar reads from the navigation store so it updates as the user navigates inside the native webview.

On mount the component:
1. Registers itself in the nav store (`registerBuffer`).
2. Navigates the native webview to the initial URL (`browserPaneNavigate`).

On unmount it removes itself from the nav store (the hook handles `browserPaneClose`).

- [ ] **Step 6.1: Write the failing tests**

Create `web/src/__tests__/features/web-viewer/components/web-viewer.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import React from 'react'

vi.mock('@/lib/crowbar-bridge', () => ({
  isTauri: vi.fn(),
  browserPaneSync: vi.fn().mockResolvedValue(undefined),
  browserPaneClose: vi.fn().mockResolvedValue(undefined),
  browserPaneNavigate: vi.fn().mockResolvedValue(undefined),
  browserPaneReload: vi.fn().mockResolvedValue(undefined),
  browserPaneGoBack: vi.fn().mockResolvedValue(undefined),
  browserPaneGoForward: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/features/web-viewer/hooks/use-browser-pane-anchor', () => ({
  useBrowserPaneAnchor: vi.fn().mockReturnValue({ isTauri: false }),
}))

vi.mock('@/features/web-viewer/stores/web-viewer-navigation-store', () => ({
  useWebViewerNavigationStore: vi.fn().mockReturnValue(undefined),
}))

import { isTauri } from '@/lib/crowbar-bridge'
import { useBrowserPaneAnchor } from '@/features/web-viewer/hooks/use-browser-pane-anchor'
import { WebViewer } from '@/features/web-viewer/components/web-viewer'

beforeEach(() => {
  vi.clearAllMocks()
})

describe('WebViewer — non-Tauri', () => {
  it('shows the requires-desktop message', () => {
    ;(isTauri as ReturnType<typeof vi.fn>).mockReturnValue(false)
    ;(useBrowserPaneAnchor as ReturnType<typeof vi.fn>).mockReturnValue({ isTauri: false })
    render(<WebViewer bufferId="b1" url="https://example.com" />)
    expect(screen.getByText(/requires the desktop app/i)).toBeInTheDocument()
  })

  it('renders the nav bar even in non-Tauri mode', () => {
    ;(isTauri as ReturnType<typeof vi.fn>).mockReturnValue(false)
    ;(useBrowserPaneAnchor as ReturnType<typeof vi.fn>).mockReturnValue({ isTauri: false })
    render(<WebViewer bufferId="b1" url="https://example.com" />)
    expect(screen.getByRole('textbox')).toBeInTheDocument()
  })
})

describe('WebViewer — Tauri', () => {
  it('does not show the requires-desktop message', () => {
    ;(useBrowserPaneAnchor as ReturnType<typeof vi.fn>).mockReturnValue({ isTauri: true })
    render(<WebViewer bufferId="b1" url="https://example.com" />)
    expect(screen.queryByText(/requires the desktop app/i)).not.toBeInTheDocument()
  })

  it('renders an anchor div (data-anchor attribute)', () => {
    ;(useBrowserPaneAnchor as ReturnType<typeof vi.fn>).mockReturnValue({ isTauri: true })
    render(<WebViewer bufferId="b1" url="https://example.com" />)
    expect(document.querySelector('[data-browser-anchor]')).toBeInTheDocument()
  })
})
```

- [ ] **Step 6.2: Run tests to confirm they fail**

```bash
cd web && npm run test -- --run src/__tests__/features/web-viewer/components/web-viewer.test.tsx
```

Expected: FAIL — imports succeed but assertions about "requires desktop app" and `data-browser-anchor` fail (old iframe implementation).

- [ ] **Step 6.3: Rewrite web-viewer.tsx**

Replace `web/src/features/web-viewer/components/web-viewer.tsx` entirely:

```tsx
import { useState, useRef, useCallback, useEffect } from 'react'
import {
  ArrowClockwise as RotateCw,
  GlobeHemisphereWest as Globe,
} from '@phosphor-icons/react'
import { cn } from '@/utils/cn'
import { browserPaneNavigate, browserPaneReload } from '@/lib/crowbar-bridge'
import { useBrowserPaneAnchor } from '@/features/web-viewer/hooks/use-browser-pane-anchor'
import { useWebViewerNavigationStore } from '@/features/web-viewer/stores/web-viewer-navigation-store'

export interface WebViewerProps {
  url?: string
  bufferId?: string
  profileKey?: string
  history?: string[]
  historyIndex?: number
  isActive?: boolean
  isVisible?: boolean
  [key: string]: unknown
}

function normalizeUrl(raw: string): string {
  const trimmed = raw.trim()
  if (!trimmed) return 'about:blank'
  if (trimmed.startsWith('about:')) return trimmed
  if (/^https?:\/\//i.test(trimmed)) return trimmed
  if (/^[^\s/]+\.[^\s/]+/.test(trimmed)) return `https://${trimmed}`
  return `https://www.google.com/search?q=${encodeURIComponent(trimmed)}`
}

export function WebViewer({
  url: initialUrl = 'about:blank',
  bufferId = '',
  isActive,
  isVisible = true,
}: WebViewerProps) {
  const anchorRef = useRef<HTMLDivElement>(null)
  const { isTauri } = useBrowserPaneAnchor({ bufferId, isVisible, anchorRef })

  const navEntry = useWebViewerNavigationStore(state =>
    bufferId ? state.navigationByBufferId[bufferId] : undefined,
  )

  const { registerBuffer, removeBuffer } = useWebViewerNavigationStore.getState()

  // Register this buffer in the nav store; initial URL drives the address bar
  useEffect(() => {
    if (!bufferId) return
    registerBuffer(bufferId, normalizeUrl(initialUrl))
    return () => removeBuffer(bufferId)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [bufferId])

  // Navigate the native webview to the initial URL once on mount
  const didNavigate = useRef(false)
  useEffect(() => {
    if (!isTauri || !bufferId || didNavigate.current) return
    const url = normalizeUrl(initialUrl)
    if (url !== 'about:blank') {
      didNavigate.current = true
      void browserPaneNavigate(bufferId, url)
    }
  }, [isTauri, bufferId, initialUrl])

  // Address bar follows the nav store url; falls back to normalized initial url
  const [inputValue, setInputValue] = useState(() => normalizeUrl(initialUrl))
  useEffect(() => {
    if (navEntry?.url) setInputValue(navEntry.url)
  }, [navEntry?.url])

  const handleSubmit = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault()
      const url = normalizeUrl(inputValue)
      setInputValue(url)
      if (bufferId) void browserPaneNavigate(bufferId, url)
    },
    [bufferId, inputValue],
  )

  const handleReload = useCallback(() => {
    if (bufferId) void browserPaneReload(bufferId)
  }, [bufferId])

  return (
    <div className={cn('flex h-full flex-col overflow-hidden', !isActive && 'pointer-events-none')}>
      {/* Navigation bar */}
      <form
        onSubmit={handleSubmit}
        className="flex shrink-0 items-center gap-1 border-b border-border bg-card px-2 py-1.5"
      >
        <button
          type="button"
          title="Reload"
          onClick={handleReload}
          className="flex items-center justify-center rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
        >
          <RotateCw size={14} />
        </button>

        <div className="flex min-w-0 flex-1 items-center gap-1.5 rounded-md bg-background px-2 py-1 ring-1 ring-border focus-within:ring-primary">
          <Globe size={12} className="shrink-0 text-muted-foreground" />
          <input
            type="text"
            value={inputValue}
            onChange={e => setInputValue(e.target.value)}
            onFocus={e => e.target.select()}
            placeholder="Enter URL or search…"
            className="min-w-0 flex-1 bg-transparent text-xs text-foreground outline-none placeholder:text-muted-foreground"
            spellCheck={false}
            autoCorrect="off"
            autoCapitalize="off"
          />
        </div>
      </form>

      {/* Content */}
      {!isTauri ? (
        <div className="flex flex-1 items-center justify-center text-muted-foreground ui-text-sm">
          This feature requires the desktop app
        </div>
      ) : (
        <div
          ref={anchorRef}
          data-browser-anchor
          className="min-h-0 flex-1"
        />
      )}
    </div>
  )
}

export default WebViewer
```

- [ ] **Step 6.4: Run tests to confirm they pass**

```bash
cd web && npm run test -- --run src/__tests__/features/web-viewer/components/web-viewer.test.tsx
```

Expected: all 4 tests PASS.

- [ ] **Step 6.5: Run the full test suite to check for regressions**

```bash
cd web && npm run test
```

Expected: all previously passing tests still pass.

- [ ] **Step 6.6: Commit**

```bash
git add web/src/features/web-viewer/components/web-viewer.tsx \
        web/src/__tests__/features/web-viewer/components/web-viewer.test.tsx
git commit -m "feat(web-viewer): replace iframe with native webview anchor"
```

---

## Task 7: Write the Rust `browser_pane` module

**Files:**
- Create: `desktop/src-tauri/src/browser_pane.rs`

This module owns the `BrowserPaneManager` Tauri state and exposes seven commands. The manager stores one `tauri::Webview` per `buffer_id`. `browser_pane_sync` creates the webview on first call and repositions/shows/hides it on subsequent calls. A JS init script is injected on creation to track SPA navigation and emit `browser-pane-navigated` events back to the main window.

**Note on Tauri v2 API:** This code uses `window.add_child`, `webview.set_bounds`, `webview.show`/`hide`, and `webview.navigate`. Verify exact method names against the installed `tauri` crate version — `cargo doc --open` or the Tauri v2 API reference at https://docs.rs/tauri/2.

- [ ] **Step 7.1: Create `desktop/src-tauri/src/browser_pane.rs`**

```rust
use std::collections::HashMap;
use std::sync::Mutex;

use tauri::{AppHandle, Emitter, LogicalPosition, LogicalSize, Manager, Rect, State};
use tauri::webview::{WebviewBuilder, WebviewUrl};

// Injected into every child webview.
// __CROWBAR_BID__ is replaced with the actual bufferId before injection.
const NAV_SCRIPT_TEMPLATE: &str = r#"
(function() {
  var B = '__CROWBAR_BID__';
  var sess = sessionStorage;
  var idx = parseInt(sess.getItem('__ci_' + B) || '0');
  var max = parseInt(sess.getItem('__cm_' + B) || '0');

  function save() {
    sess.setItem('__ci_' + B, String(idx));
    sess.setItem('__cm_' + B, String(max));
  }

  function emit(url) {
    if (!window.__TAURI_INTERNALS__) return;
    window.__TAURI_INTERNALS__.invoke('browser_pane_nav_event', {
      bufferId: B,
      url: url,
      canGoBack: idx > 0,
      canGoForward: idx < max,
    }).catch(function(){});
  }

  // Wrap pushState (SPA forward navigation)
  var origPush = history.pushState.bind(history);
  history.pushState = function(state, title, url) {
    idx += 1;
    max = Math.max(max, idx);
    origPush({ __ci: idx, __cm: max }, title, url);
    save();
    emit(String(url || location.href));
  };

  // Wrap replaceState (URL update without nav stack change)
  var origReplace = history.replaceState.bind(history);
  history.replaceState = function(state, title, url) {
    origReplace({ __ci: idx, __cm: max }, title, url);
    save();
    emit(String(url || location.href));
  };

  // popstate = back or forward
  window.addEventListener('popstate', function(e) {
    var s = history.state;
    if (s && typeof s.__ci === 'number') {
      idx = s.__ci;
      max = Math.max(max, typeof s.__cm === 'number' ? s.__cm : idx);
    } else {
      idx = Math.max(0, idx - 1);
    }
    save();
    emit(location.href);
  });

  // Full page load (cross-document navigation or initial load)
  window.addEventListener('load', function() {
    emit(location.href);
  });
})();
"#;

pub struct BrowserPaneManager {
    panes: Mutex<HashMap<String, tauri::Webview>>,
}

impl BrowserPaneManager {
    pub fn new() -> Self {
        Self {
            panes: Mutex::new(HashMap::new()),
        }
    }
}

fn make_init_script(buffer_id: &str) -> String {
    NAV_SCRIPT_TEMPLATE.replace("__CROWBAR_BID__", buffer_id)
}

#[tauri::command]
pub async fn browser_pane_sync(
    app: AppHandle,
    state: State<'_, BrowserPaneManager>,
    buffer_id: String,
    x: f64,
    y: f64,
    width: f64,
    height: f64,
    visible: bool,
) -> Result<(), String> {
    let mut panes = state.panes.lock().map_err(|e| e.to_string())?;

    if let Some(webview) = panes.get(&buffer_id) {
        // Reposition and show/hide existing webview
        webview
            .set_bounds(Rect {
                position: tauri::Position::Logical(LogicalPosition::new(x, y)),
                size: tauri::Size::Logical(LogicalSize::new(width, height)),
            })
            .map_err(|e| e.to_string())?;
        if visible {
            webview.show().map_err(|e| e.to_string())?;
        } else {
            webview.hide().map_err(|e| e.to_string())?;
        }
    } else {
        // Create new child webview
        let main_window = app
            .get_webview_window("main")
            .ok_or_else(|| "main window not found".to_string())?;

        let label = format!("browser-pane-{}", buffer_id);
        let init_script = make_init_script(&buffer_id);

        let webview = main_window
            .add_child(
                WebviewBuilder::new(&label, WebviewUrl::External("about:blank".parse().unwrap()))
                    .initialization_script(&init_script),
                LogicalPosition::new(x, y),
                LogicalSize::new(width, height),
            )
            .map_err(|e| e.to_string())?;

        if !visible {
            webview.hide().map_err(|e| e.to_string())?;
        }

        panes.insert(buffer_id, webview);
    }

    Ok(())
}

#[tauri::command]
pub async fn browser_pane_navigate(
    state: State<'_, BrowserPaneManager>,
    buffer_id: String,
    url: String,
) -> Result<(), String> {
    let panes = state.panes.lock().map_err(|e| e.to_string())?;
    let webview = panes.get(&buffer_id).ok_or_else(|| format!("no pane for {buffer_id}"))?;
    // Escape single quotes in the URL to avoid JS injection
    let safe_url = url.replace('\'', "%27");
    webview
        .eval(&format!("location.href = '{safe_url}'"))
        .map_err(|e| e.to_string())
}

#[tauri::command]
pub async fn browser_pane_go_back(
    state: State<'_, BrowserPaneManager>,
    buffer_id: String,
) -> Result<(), String> {
    let panes = state.panes.lock().map_err(|e| e.to_string())?;
    let webview = panes.get(&buffer_id).ok_or_else(|| format!("no pane for {buffer_id}"))?;
    webview.eval("history.back()").map_err(|e| e.to_string())
}

#[tauri::command]
pub async fn browser_pane_go_forward(
    state: State<'_, BrowserPaneManager>,
    buffer_id: String,
) -> Result<(), String> {
    let panes = state.panes.lock().map_err(|e| e.to_string())?;
    let webview = panes.get(&buffer_id).ok_or_else(|| format!("no pane for {buffer_id}"))?;
    webview.eval("history.forward()").map_err(|e| e.to_string())
}

#[tauri::command]
pub async fn browser_pane_reload(
    state: State<'_, BrowserPaneManager>,
    buffer_id: String,
) -> Result<(), String> {
    let panes = state.panes.lock().map_err(|e| e.to_string())?;
    let webview = panes.get(&buffer_id).ok_or_else(|| format!("no pane for {buffer_id}"))?;
    webview.eval("location.reload()").map_err(|e| e.to_string())
}

#[tauri::command]
pub async fn browser_pane_close(
    state: State<'_, BrowserPaneManager>,
    buffer_id: String,
) -> Result<(), String> {
    let mut panes = state.panes.lock().map_err(|e| e.to_string())?;
    if let Some(webview) = panes.remove(&buffer_id) {
        webview.close().map_err(|e| e.to_string())?;
    }
    Ok(())
}

// Called by the injected JS in each child webview.
// Re-emits the event to the main window so React can update the address bar.
#[tauri::command]
pub async fn browser_pane_nav_event(
    app: AppHandle,
    buffer_id: String,
    url: String,
    can_go_back: bool,
    can_go_forward: bool,
) -> Result<(), String> {
    app.emit_to(
        tauri::EventTarget::WebviewWindow { label: "main".into() },
        "browser-pane-navigated",
        serde_json::json!({
            "bufferId": buffer_id,
            "url": url,
            "canGoBack": can_go_back,
            "canGoForward": can_go_forward,
        }),
    )
    .map_err(|e| e.to_string())
}
```

- [ ] **Step 7.2: Verify Rust compiles**

```bash
cd desktop && cargo build 2>&1 | head -40
```

Expected: may have errors — fix them before continuing. Common issues:
- `tauri::Rect`, `tauri::Position`, `tauri::Size` import paths may differ — check with `cargo doc --open` if needed
- `webview.show()` / `webview.hide()` may be `webview.set_visible(true/false)` in your Tauri version
- `window.add_child` signature may differ slightly; check `tauri::WebviewWindow` docs

Fix any compile errors in `browser_pane.rs` until `cargo build` succeeds.

- [ ] **Step 7.3: Commit**

```bash
git add desktop/src-tauri/src/browser_pane.rs
git commit -m "feat(desktop): add browser_pane Rust module with child webview commands"
```

---

## Task 8: Register the module and commands in `lib.rs`

**Files:**
- Modify: `desktop/src-tauri/src/lib.rs`

- [ ] **Step 8.1: Add `mod browser_pane` and register state + commands**

In `desktop/src-tauri/src/lib.rs`, make these three changes:

**Add module declaration** (alongside the existing `mod` lines at the top):
```rust
mod browser_pane;
```

**Add `.manage(...)` call** (in `builder.manage(sidecar::SidecarHandle::new())` chain — add another `.manage` line after it):
```rust
.manage(browser_pane::BrowserPaneManager::new())
```

**Add commands to `.invoke_handler`** — if there isn't one yet, add it before `.run(...)`:
```rust
.invoke_handler(tauri::generate_handler![
    browser_pane::browser_pane_sync,
    browser_pane::browser_pane_navigate,
    browser_pane::browser_pane_go_back,
    browser_pane::browser_pane_go_forward,
    browser_pane::browser_pane_reload,
    browser_pane::browser_pane_close,
    browser_pane::browser_pane_nav_event,
])
```

If `.invoke_handler` already exists in `lib.rs`, add the seven commands to the existing macro invocation rather than creating a second one.

- [ ] **Step 8.2: Verify the full desktop build**

```bash
cd desktop && cargo build 2>&1 | tail -20
```

Expected: `Finished dev [unoptimized + debuginfo]` with no errors.

- [ ] **Step 8.3: Run the app and manually verify**

```bash
cd desktop && cargo tauri dev
```

Open a web viewer tab and navigate to `https://google.com`. Expected:
- Google loads (no `X-Frame-Options` error)
- Address bar shows `https://www.google.com/`
- Clicking a link updates the address bar
- Back button in the tab bar activates after navigating away

- [ ] **Step 8.4: Commit**

```bash
git add desktop/src-tauri/src/lib.rs
git commit -m "feat(desktop): register browser_pane module and commands in lib.rs"
```

---

## Self-Review Notes

- **Spec coverage:** All seven spec requirements covered. Non-Tauri fallback ✓, per-buffer child webview ✓, ResizeObserver sync ✓, `canGoBack`/`canGoForward` ✓, address bar update on navigation ✓, cleanup on buffer close ✓.
- **Type consistency:** `WebViewerNavEntry` in store matches what `useJumpNavigation` reads (`canGoBack`, `canGoForward`, `goBack`, `goForward`). Bridge function signatures match between Task 2 (definition), Task 3 (hook callers), and Task 6 (component callers).
- **Rust command names:** JS side uses camelCase keys (`bufferId`, `canGoBack`) and Rust uses snake_case (`buffer_id`, `can_go_back`) — Tauri's `invoke` serialiser handles this conversion automatically.
- **`tauri::generate_handler!` uniqueness:** If `lib.rs` already has an `invoke_handler`, add to the existing macro. Two `invoke_handler` calls would silently shadow the first.
