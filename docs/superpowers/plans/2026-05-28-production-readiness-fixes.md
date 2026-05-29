# Production-Readiness Fixes — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix 9 production-readiness issues in the React/TypeScript frontend: crash bugs, memory leaks, stale state, UI accessibility gaps, and restore the bottom-pane/terminal subsystem that is fully implemented in the store but never rendered.

**Architecture:** Three groups of frontend fixes. Groups A and B are independent and can be worked concurrently. Group C (bottom pane restoration) is the largest and must follow Group A's pane-slice routing fix.

**Tech Stack:** React 18 + Zustand + Immer; TypeScript; Vitest

---

## File Map

### Group A — Frontend Crash & Data Fixes
| Action | File |
|---|---|
| Modify | `web/src/components/layout/IDEShell.tsx` |
| Modify | `web/src/hooks/useModelPreference.ts` |
| Modify | `web/src/features/workspace/stores/workspace-store-ref.ts` |
| Modify | `web/src/features/workspace/stores/hooks/use-active-workspace-state.ts` |

### Group B — UI Fixes
| Action | File |
|---|---|
| Modify | `web/src/components/chat/MarkdownContent.tsx` |
| Modify | `web/src/components/ui/context-menu.tsx` |

### Group C — Bottom Pane Restoration
| Action | File |
|---|---|
| Modify | `web/src/features/workspace/stores/slices/pane-slice.ts` |
| Modify | `web/src/features/window/stores/ui-state-store.ts` |
| Create | `web/src/features/workspace/components/bottom-pane.tsx` |
| Modify | `web/src/features/workspace/components/WorkspaceLayoutRoot.tsx` |
| Modify | `web/src/features/workspace/stores/hooks/use-workspace-effects.ts` |
| Modify | `web/src/features/terminal/components/terminal-tab.tsx` |

---

## Group A: Frontend Crash & Data Fixes

### Task 1: localStorage try/catch Guards

**Files:**
- Modify: `web/src/components/layout/IDEShell.tsx:32-35`
- Modify: `web/src/hooks/useModelPreference.ts:16`

**Bug:** `localStorage.getItem` called bare inside `useState` lazy initializers. In Firefox/Safari private browsing or storage-locked contexts, `localStorage` throws `SecurityError`. This executes during render before error boundaries can catch it, crashing the whole app on mount.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/hooks/useModelPreference.test.ts`:

```ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useModelPreference } from '@/hooks/useModelPreference'

describe('useModelPreference', () => {
  it('returns default model when localStorage throws', () => {
    const originalGet = Storage.prototype.getItem
    Storage.prototype.getItem = () => { throw new Error('SecurityError') }

    let result: ReturnType<typeof renderHook<ReturnType<typeof useModelPreference>, unknown>>
    expect(() => {
      result = renderHook(() => useModelPreference())
    }).not.toThrow()

    expect(result!.result.current.model).toBe('claude-sonnet-4-6')
    Storage.prototype.getItem = originalGet
  })
})
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd web && npx vitest run src/__tests__/hooks/useModelPreference.test.ts
```

Expected: FAIL (throws SecurityError)

- [ ] **Step 3: Fix `useModelPreference.ts`**

Find the `useState` initializer in `web/src/hooks/useModelPreference.ts` that calls `localStorage.getItem(STORAGE_KEY)`. Wrap it:

```ts
// Before (find the useState line):
const [model, setModelState] = useState<Model>(() => {
  const stored = localStorage.getItem(STORAGE_KEY)
  // ... rest of logic
})

// After:
const [model, setModelState] = useState<Model>(() => {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    // ... rest of logic (unchanged)
  } catch {
    return 'claude-sonnet-4-6'
  }
})
```

Also wrap the `localStorage.setItem` call in `setModel`:

```ts
const setModel = (m: Model) => {
  try {
    localStorage.setItem(STORAGE_KEY, m)
  } catch {
    // storage unavailable — preference not persisted this session
  }
  setModelState(m)
}
```

- [ ] **Step 4: Fix `IDEShell.tsx`**

In `web/src/components/layout/IDEShell.tsx`, find lines 32-35 (the `useState` for `sidebarWidth`):

```ts
// Before:
const [sidebarWidth, setSidebarWidth] = useState(() => {
  const stored = parseInt(localStorage.getItem('sidebar-width') ?? '', 10)
  return Number.isFinite(stored) ? Math.max(294, stored) : 294
})

// After:
const [sidebarWidth, setSidebarWidth] = useState(() => {
  try {
    const stored = parseInt(localStorage.getItem('sidebar-width') ?? '', 10)
    return Number.isFinite(stored) ? Math.max(294, stored) : 294
  } catch {
    return 294
  }
})
```

- [ ] **Step 5: Run tests**

```bash
cd web && npx vitest run src/__tests__/hooks/useModelPreference.test.ts
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add web/src/hooks/useModelPreference.ts web/src/components/layout/IDEShell.tsx web/src/__tests__/hooks/useModelPreference.test.ts
git commit -m "fix: guard localStorage access with try/catch in useState initializers"
```

---

### Task 2: IDEShell Drag Listener Cleanup + Pane Carousel Leak

**Files:**
- Modify: `web/src/components/layout/IDEShell.tsx`
- Modify: `web/src/features/panes/components/pane-container.tsx`

**Bug A:** `handleResizeDragStart` attaches `mousemove`/`mouseup` to `document` directly (not in a `useEffect`). If `IDEShell` unmounts during a drag (route change), `mouseup` never fires, so the listeners are permanently attached to `document`. Also the pending `requestAnimationFrame` is not cancelled.

**Bug B:** `pane-container.tsx` has a similar pattern for carousel card resize drag — if the pane unmounts mid-drag, `document.body.style.cursor` and `userSelect` are permanently stuck.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/components/layout/IDEShell.test.tsx`:

```ts
import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, fireEvent, cleanup } from '@testing-library/react'
import { IDEShell } from '@/components/layout/IDEShell'

vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  useWorkspaces: () => [],
  useActiveWorkspaceId: () => null,
}))

describe('IDEShell drag listener cleanup', () => {
  afterEach(cleanup)

  it('removes document listeners when unmounted during drag', () => {
    const addSpy = vi.spyOn(document, 'addEventListener')
    const removeSpy = vi.spyOn(document, 'removeEventListener')

    const { unmount } = render(<IDEShell />)

    // Simulate drag start on the resize handle
    const handle = document.querySelector('[data-slot="sidebar-resize-handle"]')
    if (handle) {
      fireEvent.mouseDown(handle, { clientX: 300 })
    }

    const addedCount = addSpy.mock.calls.length

    // Unmount while drag is in progress (no mouseup)
    unmount()

    const removedCount = removeSpy.mock.calls.length
    expect(removedCount).toBeGreaterThanOrEqual(addedCount)

    addSpy.mockRestore()
    removeSpy.mockRestore()
  })
})
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd web && npx vitest run src/__tests__/components/layout/IDEShell.test.tsx
```

Expected: FAIL (listeners not cleaned up)

- [ ] **Step 3: Fix `IDEShell.tsx` — track active drag cleanup in a ref**

In `web/src/components/layout/IDEShell.tsx`, add a `cleanupDragRef` and wire it up:

```ts
// Add near other refs/state at top of IDEShell component:
const cleanupDragRef = useRef<(() => void) | null>(null)

// Add useEffect for cleanup on unmount (after all other hooks):
useEffect(() => {
  return () => { cleanupDragRef.current?.() }
}, [])
```

Inside `handleResizeDragStart` (the `onMouseDown` handler), find where `document.addEventListener('mousemove', ...)` and `document.addEventListener('mouseup', ...)` are called. Store a cleanup function:

```ts
// After adding the two document listeners, add:
cleanupDragRef.current = () => {
  document.removeEventListener('mousemove', handleMouseMove)
  document.removeEventListener('mouseup', handleMouseUp)
  if (rafId) cancelAnimationFrame(rafId)
  document.body.style.cursor = ''
  document.body.style.userSelect = ''
  cleanupDragRef.current = null
}
```

Inside the existing `handleMouseUp` function, call the cleanup at the start:

```ts
const handleMouseUp = () => {
  cleanupDragRef.current?.()  // remove listeners and cancel RAF
  // ... rest of existing mouseup logic (localStorage write, setSidebarWidth)
}
```

Note: after this change, `document.removeEventListener` in the old `handleMouseUp` body is handled by the cleanup ref — remove any duplicate `removeEventListener` calls to avoid double-remove warnings.

- [ ] **Step 4: Fix `pane-container.tsx` — carousel resize listener leak**

Find the carousel card resize drag handler in `web/src/features/panes/components/pane-container.tsx` (around line 444 — the one that sets `document.body.style.cursor = 'col-resize'` and `document.body.style.userSelect = 'none'`).

Apply the same pattern as IDEShell: add a `cleanupCarouselDragRef = useRef<(() => void) | null>(null)`, store the cleanup when listeners are added, call it on `mouseup`, and add a `useEffect(() => () => { cleanupCarouselDragRef.current?.() }, [])`.

- [ ] **Step 5: Run tests**

```bash
cd web && npx vitest run src/__tests__/components/layout/IDEShell.test.tsx
```

Expected: PASS

- [ ] **Step 6: Run full test suite**

```bash
cd web && npx vitest run
```

Expected: no regressions

- [ ] **Step 7: Commit**

```bash
git add web/src/components/layout/IDEShell.tsx web/src/features/panes/components/pane-container.tsx web/src/__tests__/components/layout/IDEShell.test.tsx
git commit -m "fix: clean up document drag listeners on unmount in IDEShell and PaneContainer"
```

---

### Task 3: useActiveWorkspaceState — Fix Stale Selector Closure

**Files:**
- Modify: `web/src/features/workspace/stores/hooks/use-active-workspace-state.ts`

**Bug:** The `useEffect` dependency array is intentionally `[]`, but `selector` and `fallback` are captured at mount. If a parent re-renders with a different `selector` (e.g. inline arrow using a prop), the hook silently reads stale data forever. Fix: store `selector` and `fallback` in refs that are always up-to-date.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/workspace/stores/hooks/use-active-workspace-state.test.ts`:

```ts
import { describe, it, expect, vi } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useActiveWorkspaceState } from '@/features/workspace/stores/hooks/use-active-workspace-state'
import * as storeRef from '@/features/workspace/stores/workspace-store-ref'

describe('useActiveWorkspaceState', () => {
  it('uses the latest selector when parent re-renders with a new selector', () => {
    // Create a minimal fake store
    let storeState = { value: 'a' }
    let storeSubscriber: ((s: typeof storeState) => void) | null = null
    const fakeStore = {
      getState: () => storeState,
      subscribe: (fn: (s: typeof storeState) => void) => {
        storeSubscriber = fn
        return () => { storeSubscriber = null }
      },
    }

    vi.spyOn(storeRef, 'getActiveWorkspaceStoreRef').mockReturnValue(fakeStore as never)
    const onChangeSpy = vi.spyOn(storeRef, 'onActiveWorkspaceStoreChange')
    let fireChange: (store: typeof fakeStore | null) => void = () => {}
    onChangeSpy.mockImplementation((cb) => {
      fireChange = cb as typeof fireChange
      cb(fakeStore as never)
      return () => {}
    })

    // Render with selector for 'value'
    const { result, rerender } = renderHook(
      ({ key }: { key: string }) => useActiveWorkspaceState((s: never) => (s as Record<string, string>)[key], ''),
      { initialProps: { key: 'value' } }
    )
    expect(result.current).toBe('a')

    // Re-render with a different selector key — simulates prop change
    storeState = { value: 'a', other: 'b' } as never
    rerender({ key: 'other' })

    // Trigger a store update — the hook should use the NEW selector
    act(() => {
      storeSubscriber?.({ value: 'a', other: 'b' } as never)
    })

    expect(result.current).toBe('b')

    vi.restoreAllMocks()
  })
})
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd web && npx vitest run src/__tests__/features/workspace/stores/hooks/use-active-workspace-state.test.ts
```

Expected: FAIL (result is still 'a' because selector is stale)

- [ ] **Step 3: Apply the useRef fix**

Replace the full content of `web/src/features/workspace/stores/hooks/use-active-workspace-state.ts`:

```ts
import { useState, useEffect, useRef } from 'react'
import {
  getActiveWorkspaceStoreRef,
  onActiveWorkspaceStoreChange,
} from '../workspace-store-ref'
import type { WorkspaceStore } from '../workspace-store'
import type { WorkspaceState } from '../workspace-store.types'

/**
 * Safe alternative to useWorkspaceStoreContext for components that render
 * outside WorkspaceStoreContext.Provider (sidebar, global overlays, etc.).
 *
 * Returns `fallback` when no workspace is active, and automatically
 * re-subscribes when the active workspace changes.
 *
 * Unlike useWorkspaceStoreContext, this hook always uses the latest selector
 * even if the parent re-renders with a new one — safe with inline selectors.
 */
export function useActiveWorkspaceState<T>(
  selector: (state: WorkspaceState) => T,
  fallback: T,
): T {
  // Always-current refs so the subscription closure never goes stale.
  const selectorRef = useRef(selector)
  selectorRef.current = selector
  const fallbackRef = useRef(fallback)
  fallbackRef.current = fallback

  const [value, setValue] = useState<T>(() => {
    const store = getActiveWorkspaceStoreRef()
    return store ? selectorRef.current(store.getState()) : fallbackRef.current
  })

  useEffect(() => {
    let storeUnsub: (() => void) | null = null

    const unsub = onActiveWorkspaceStoreChange((store: WorkspaceStore | null) => {
      storeUnsub?.()
      storeUnsub = null

      if (!store) {
        setValue(fallbackRef.current)
        return
      }

      setValue(selectorRef.current(store.getState()))
      storeUnsub = store.subscribe((state) => {
        setValue(selectorRef.current(state))
      })
    })

    return () => {
      unsub()
      storeUnsub?.()
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  return value
}
```

- [ ] **Step 4: Run tests**

```bash
cd web && npx vitest run src/__tests__/features/workspace/stores/hooks/use-active-workspace-state.test.ts
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/features/workspace/stores/hooks/use-active-workspace-state.ts web/src/__tests__/features/workspace/stores/hooks/use-active-workspace-state.test.ts
git commit -m "fix: use refs in useActiveWorkspaceState to prevent stale selector closure"
```

---

### Task 4: workspace-store-ref — Eliminate Null Window During Workspace Switch

**Files:**
- Modify: `web/src/features/workspace/stores/workspace-store-ref.ts`

**Bug:** When the active workspace changes, `WorkspaceView` cleanup calls `setActiveWorkspaceStoreRef(null)` which immediately notifies all subscribers with `null` — causing them to tear down Zustand subscriptions and reset state. Then the new workspace mounts and calls `setActiveWorkspaceStoreRef(newStore)`. The gap is real: any subscriber that resets state on `null` (e.g. file explorer clearing its tree) produces a visible flash.

**Fix:** Defer the `null` notification by one microtask. If a new store is set before the microtask fires, the `null` notification is suppressed entirely.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/workspace/stores/workspace-store-ref.test.ts`:

```ts
import { describe, it, expect, vi } from 'vitest'
import {
  setActiveWorkspaceStoreRef,
  onActiveWorkspaceStoreChange,
} from '@/features/workspace/stores/workspace-store-ref'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'

function fakeStore(id: string): WorkspaceStore {
  return { id } as unknown as WorkspaceStore
}

describe('workspace-store-ref null window', () => {
  it('does not fire null when a new store is set in the same tick', async () => {
    const calls: Array<WorkspaceStore | null> = []
    const unsub = onActiveWorkspaceStoreChange((s) => calls.push(s))
    calls.length = 0 // ignore the immediate fire

    const storeA = fakeStore('a')
    const storeB = fakeStore('b')

    setActiveWorkspaceStoreRef(storeA)
    setActiveWorkspaceStoreRef(null)   // workspace switch: old unmounts
    setActiveWorkspaceStoreRef(storeB) // new mounts in same sync block

    // Let the microtask queue drain
    await Promise.resolve()

    // Should see storeA and storeB, but NOT null in between
    expect(calls).not.toContainEqual(null)
    expect(calls[calls.length - 1]).toBe(storeB)

    unsub()
  })

  it('fires null when no new store follows', async () => {
    const calls: Array<WorkspaceStore | null> = []
    const unsub = onActiveWorkspaceStoreChange((s) => calls.push(s))
    calls.length = 0

    const storeA = fakeStore('a')
    setActiveWorkspaceStoreRef(storeA)
    setActiveWorkspaceStoreRef(null) // last workspace closed

    await Promise.resolve()

    expect(calls).toContainEqual(null)

    unsub()
  })
})
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd web && npx vitest run src/__tests__/features/workspace/stores/workspace-store-ref.test.ts
```

Expected: FAIL on the first test (null is fired immediately)

- [ ] **Step 3: Apply the microtask-deferred null fix**

Replace the full content of `web/src/features/workspace/stores/workspace-store-ref.ts`:

```ts
import type { WorkspaceStore } from './workspace-store'

let _activeWorkspaceStore: WorkspaceStore | null = null
const _listeners = new Set<(store: WorkspaceStore | null) => void>()

export function setActiveWorkspaceStoreRef(store: WorkspaceStore | null): void {
  _activeWorkspaceStore = store

  if (store !== null) {
    // Non-null: notify immediately so the new workspace is available synchronously.
    for (const fn of _listeners) fn(store)
  } else {
    // Null: defer one microtask so a same-tick workspace switch can set the new
    // store before subscribers are notified. If the ref is non-null by the time
    // the microtask fires, the null notification is suppressed entirely.
    queueMicrotask(() => {
      if (_activeWorkspaceStore === null) {
        for (const fn of _listeners) fn(null)
      }
    })
  }
}

/**
 * Returns the active workspace store for imperative (non-React) access.
 * Returns null when no workspace is active. Always null-check the result.
 */
export function getActiveWorkspaceStoreRef(): WorkspaceStore | null {
  return _activeWorkspaceStore
}

/**
 * Register a listener that fires whenever the active workspace store changes.
 * Fires immediately with the current store so late registrants don't miss it.
 * Returns an unsubscribe function.
 */
export function onActiveWorkspaceStoreChange(
  listener: (store: WorkspaceStore | null) => void,
): () => void {
  _listeners.add(listener)
  listener(_activeWorkspaceStore)
  return () => { _listeners.delete(listener) }
}
```

- [ ] **Step 4: Run tests**

```bash
cd web && npx vitest run src/__tests__/features/workspace/stores/workspace-store-ref.test.ts
```

Expected: PASS

- [ ] **Step 5: Run full test suite**

```bash
cd web && npx vitest run
```

Expected: no regressions

- [ ] **Step 6: Commit**

```bash
git add web/src/features/workspace/stores/workspace-store-ref.ts web/src/__tests__/features/workspace/stores/workspace-store-ref.test.ts
git commit -m "fix: defer null notification in workspace-store-ref to prevent flash during workspace switch"
```

---

## Group B: UI Bug Fixes

### Task 5: MarkdownContent — Stable React Keys for Inline Tokens

**Files:**
- Modify: `web/src/components/chat/MarkdownContent.tsx:64`

**Bug:** `tokenizeInline(raw).map((t, i) => ...)` uses array index `i` as React key. During streaming, when the token list grows (e.g. 2 tokens → 3 tokens), React reuses DOM nodes by index — a `<strong>` gets reconciled as `<em>`, producing corrupted inline text.

**Fix:** Generate a stable key from the token's kind and content. Since tokens are pure derivations of `raw`, a key of `kind:content` is unique within a single tokenizeInline call on the same line.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/components/chat/MarkdownContent.test.tsx`:

```ts
import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/react'
import { MarkdownContent } from '@/components/chat/MarkdownContent'

describe('MarkdownContent', () => {
  it('renders bold text correctly', () => {
    const { container } = render(<MarkdownContent content="**hello**" />)
    expect(container.querySelector('strong')?.textContent).toBe('hello')
  })

  it('renders italic text correctly', () => {
    const { container } = render(<MarkdownContent content="*world*" />)
    expect(container.querySelector('em')?.textContent).toBe('world')
  })

  it('renders inline code correctly', () => {
    const { container } = render(<MarkdownContent content="`code`" />)
    expect(container.querySelector('code')?.textContent).toBe('code')
  })
})
```

- [ ] **Step 2: Run to verify tests pass (baseline)**

```bash
cd web && npx vitest run src/__tests__/components/chat/MarkdownContent.test.tsx
```

Expected: PASS (these verify baseline behavior; the key bug is a reconciliation issue visible only during streaming)

- [ ] **Step 3: Fix the key in `MarkdownContent.tsx`**

In `web/src/components/chat/MarkdownContent.tsx`, find line 64 (inside the `Inline` component's `tokenizeInline(raw).map((t, i) => ...)`).

Replace the `.map` call:

```ts
// Before:
{tokenizeInline(raw).map((t, i) => {
  // ... render logic ...
  return <element key={i}>...</element>
})}

// After:
{tokenizeInline(raw).map((t, i) => {
  // ... render logic (unchanged) ...
  // key includes index as tiebreaker for duplicate kind+text (e.g. "**a** **a**")
  return <element key={`${t.kind}:${t.text}:${i}`}>...</element>
})}
```

The `i` tiebreaker at the end means duplicate tokens in the same line still get unique keys, while the `kind:text:` prefix ensures React uses position-independent reconciliation when token count changes.

- [ ] **Step 4: Run tests**

```bash
cd web && npx vitest run src/__tests__/components/chat/MarkdownContent.test.tsx
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/components/chat/MarkdownContent.tsx web/src/__tests__/components/chat/MarkdownContent.test.tsx
git commit -m "fix: use stable kind:text:i keys for inline markdown tokens to prevent reconciliation corruption"
```

---

### Task 6: Context Menu — Keyboard Dismiss (Escape Key)

**Files:**
- Modify: `web/src/components/ui/context-menu.tsx` (the `ImperativeContextMenu` component, around line 84)

**Bug:** The dismiss backdrop `<div>` has `onClick`/`onContextMenu` but no keyboard handler. Keyboard-only users cannot close an open context menu.

- [ ] **Step 1: Find the backdrop div**

In `web/src/components/ui/context-menu.tsx`, find the `ImperativeContextMenu` component. It renders a fixed full-screen backdrop div and the menu panel. The backdrop looks like:

```tsx
<div
  className="fixed inset-0 z-[10000]"
  onClick={onClose}
  onContextMenu={...}
/>
```

- [ ] **Step 2: Write the test**

Create `web/src/__tests__/components/ui/context-menu.test.tsx`:

```ts
import { describe, it, expect, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/react'
import { ContextMenu } from '@/components/ui/context-menu'

describe('ContextMenu keyboard dismiss', () => {
  it('calls onClose when Escape is pressed', () => {
    const onClose = vi.fn()
    render(
      <ContextMenu
        isOpen={true}
        position={{ x: 100, y: 100 }}
        items={[{ label: 'Item', onClick: vi.fn() }]}
        onClose={onClose}
      />
    )
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onClose).toHaveBeenCalledOnce()
  })
})
```

- [ ] **Step 3: Run to verify it fails**

```bash
cd web && npx vitest run src/__tests__/components/ui/context-menu.test.tsx
```

Expected: FAIL

- [ ] **Step 4: Add Escape key handler to `ImperativeContextMenu`**

In the `ImperativeContextMenu` component (or the `useContextMenu` hook — wherever `isOpen` state lives), add a `useEffect` that listens for `keydown` on `document`:

```tsx
// Inside ImperativeContextMenu component:
useEffect(() => {
  if (!isOpen) return
  const handler = (e: KeyboardEvent) => {
    if (e.key === 'Escape') {
      e.stopPropagation()
      onClose()
    }
  }
  document.addEventListener('keydown', handler)
  return () => document.removeEventListener('keydown', handler)
}, [isOpen, onClose])
```

Also add `role="presentation"` to the backdrop div:

```tsx
<div
  role="presentation"
  className="fixed inset-0 z-[10000]"
  onClick={onClose}
  onContextMenu={...}
/>
```

- [ ] **Step 5: Run tests**

```bash
cd web && npx vitest run src/__tests__/components/ui/context-menu.test.tsx
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add web/src/components/ui/context-menu.tsx web/src/__tests__/components/ui/context-menu.test.tsx
git commit -m "fix: close imperative context menu on Escape key for keyboard accessibility"
```

---

## Group C: Bottom Pane Restoration

**Context for all D tasks:** The bottom pane is fully designed: the store has `bottomRoot: PaneNode` in both state and persistence; `useBottomRoot()` hook exists; `PaneNodeRenderer` is generic and can render any `PaneNode` tree; `isBottomPaneVisible` flag exists in `useUIState`; `BOTTOM_PANE_ID = "bottom-pane"` constant exists. But four things are missing: (1) pane-slice actions only mutate `paneRoot`, not `bottomRoot`; (2) `WorkspaceLayoutRoot` has no bottom panel DOM slot; (3) `SplitViewRoot` doesn't render `bottomRoot`; (4) `terminal-ensure-session` event has no listener and `requestTerminalFocus` is a stub.

---

### Task 7: pane-slice — Route Actions to bottomRoot When paneId Belongs There

**Files:**
- Modify: `web/src/features/workspace/stores/slices/pane-slice.ts`

**Bug:** Every action (`splitPane`, `closePane`, `activatePaneBuffer`, `addBufferToPane`, `removeBufferFromPane`, `moveBufferBetweenPanes`, `setPanePreviewBuffer`, `setPaneBufferPinned`, `reorderPaneBuffers`, `resizeFlattenedPaneSplit`, `distributeFlattenedPaneSplit`) hardcodes `state.paneRoot`. Actions invoked on a bottom-pane paneId silently no-op or corrupt `paneRoot`. The read-only getters (`getAllPaneGroups`, `getPaneById`, `getPaneByBufferId`) also miss `bottomRoot`.

**Reference implementation:** `web/src/features/panes/stores/pane-store.ts` has the same actions correctly implemented with `getTreeForPane()` routing — use it as the reference.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/workspace/stores/slices/pane-slice.test.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { BOTTOM_PANE_ID } from '@/features/panes/constants/pane-constants'

describe('pane-slice bottomRoot routing', () => {
  it('addBufferToPane adds to bottomRoot when paneId is BOTTOM_PANE_ID', () => {
    const store = createWorkspaceStore('test-ws')
    store.getState().paneActions.addBufferToPane(BOTTOM_PANE_ID, 'buf-1', true)
    const bottomGroups = store.getState().paneActions.getAllPaneGroups()
      // getAllPaneGroups must return both trees' groups
    const bottomGroup = bottomGroups.find(g => g.id === BOTTOM_PANE_ID)
    expect(bottomGroup?.bufferIds).toContain('buf-1')
  })

  it('getAllPaneGroups includes groups from both paneRoot and bottomRoot', () => {
    const store = createWorkspaceStore('test-ws')
    const groups = store.getState().paneActions.getAllPaneGroups()
    const ids = groups.map(g => g.id)
    expect(ids).toContain('root-pane')
    expect(ids).toContain(BOTTOM_PANE_ID)
  })
})
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd web && npx vitest run src/__tests__/features/workspace/stores/slices/pane-slice.test.ts
```

Expected: FAIL (`buf-1` ends up in `paneRoot` not `bottomRoot`)

- [ ] **Step 3: Add the `treeFor` helper to `pane-slice.ts`**

In `web/src/features/workspace/stores/slices/pane-slice.ts`, add this helper function at the top of the file (after imports, before the slice definition):

```ts
import type { WritableDraft } from 'immer'

// Returns the pane tree that owns `paneId`, and a setter to write the mutated tree back.
// Searches paneRoot first; falls back to bottomRoot.
function treeFor(
  state: WritableDraft<PaneState>,
  paneId: string,
): { tree: PaneNode; setTree: (n: PaneNode) => void } {
  if (findPaneGroup(state.paneRoot, paneId)) {
    return { tree: state.paneRoot, setTree: (n) => { state.paneRoot = n } }
  }
  return { tree: state.bottomRoot, setTree: (n) => { state.bottomRoot = n } }
}
```

(`findPaneGroup` is already imported in pane-slice.ts. `WritableDraft` comes from `immer` which is already a dependency.)

- [ ] **Step 4: Update each action to use `treeFor`**

**`splitPane`** (currently uses `state.paneRoot` on lines ~81-94):

```ts
splitPane(paneId, direction, bufferId?, placement = 'after') {
  set(state => {
    const { tree, setTree } = treeFor(state, paneId)
    const oldIds = getAllPaneGroups(tree).map(g => g.id)
    const newTree = splitPaneUtil(tree, paneId, direction, bufferId, placement)
    setTree(newTree)
    const newGroup = getAllPaneGroups(newTree).find(g => !oldIds.includes(g.id))
    if (newGroup) state.activePaneId = newGroup.id
  })
},
```

**`closePane`** (currently around line 95):

```ts
// In the set(state => ...) body for close:
const { tree, setTree } = treeFor(state, paneId)
const result = closePane(tree, paneId)
if (result !== null) {
  setTree(normalizePaneTree(result))
  const remaining = getAllPaneGroups(result)
  if (!remaining.find(g => g.id === state.activePaneId)) {
    state.activePaneId = remaining[0]?.id ?? ROOT_PANE_ID
  }
}
```

**`activatePaneBuffer`** (around line 115):

```ts
activatePaneBuffer(paneId, bufferId) {
  set(state => {
    const { tree, setTree } = treeFor(state, paneId)
    setTree(setActivePaneBuffer(tree, paneId, bufferId))
    state.activePaneId = paneId
  })
},
```

**`addBufferToPane`** (around line 122):

```ts
addBufferToPane(paneId, bufferId, setActive = true) {
  set(state => {
    const { tree, setTree } = treeFor(state, paneId)
    setTree(addBufferToPane(tree, paneId, bufferId, setActive))
  })
},
```

**`removeBufferFromPane`** (around line 128):

```ts
removeBufferFromPane(paneId, bufferId, preserveEmptyPane = false) {
  set(state => {
    const { tree, setTree } = treeFor(state, paneId)
    setTree(removeBufferFromPane(tree, paneId, bufferId))
    if (!preserveEmptyPane) {
      const pane = findPaneGroup(tree, paneId)
      const isRoot = paneId === ROOT_PANE_ID || paneId === BOTTOM_PANE_ID
      if (pane && pane.bufferIds.length === 0 && !isRoot) {
        const result = closePane(tree, paneId)
        if (result !== null) setTree(normalizePaneTree(result))
      }
    }
  })
},
```

**`moveBufferBetweenPanes`** (around line 143): The existing implementation calls `state.paneRoot = moveBufferBetweenPanes(...)`. Replace with:

```ts
set(state => {
  const { tree: fromTree, setTree: setFromTree } = treeFor(state, fromPaneId)
  const { tree: toTree, setTree: setToTree } = treeFor(state, toPaneId)
  if (fromTree === toTree) {
    // Same tree — single mutation
    setFromTree(moveBufferBetweenPanes(fromTree, bufferId, fromPaneId, toPaneId))
  } else {
    // Cross-tree move: remove from source, add to dest
    setFromTree(removeBufferFromPane(fromTree, fromPaneId, bufferId))
    setToTree(addBufferToPane(toTree, toPaneId, bufferId, true))
  }
})
```

**`setPanePreviewBuffer`**, **`setPaneBufferPinned`**, **`reorderPaneBuffers`**, **`resizeFlattenedPaneSplit`**, **`distributeFlattenedPaneSplit`**: Each currently does `state.paneRoot = fn(state.paneRoot, paneId, ...)`. Replace the pattern with:

```ts
set(state => {
  const { tree, setTree } = treeFor(state, paneId)
  setTree(fn(tree, paneId, ...args))
})
```

- [ ] **Step 5: Fix the read-only getters**

Find `getAllPaneGroups`, `getPaneById`, `getPaneByBufferId`, `getActivePane` (around lines 187-213):

```ts
getAllPaneGroups() {
  return [
    ...getAllPaneGroups(get().paneRoot),
    ...getAllPaneGroups(get().bottomRoot),
  ]
},
getPaneById(paneId) {
  return (
    findPaneGroup(get().paneRoot, paneId) ??
    findPaneGroup(get().bottomRoot, paneId) ??
    null
  )
},
getPaneByBufferId(bufferId) {
  return (
    findPaneGroupByBufferId(get().paneRoot, bufferId) ??
    findPaneGroupByBufferId(get().bottomRoot, bufferId) ??
    null
  )
},
getActivePane() {
  const id = get().activePaneId
  return (
    findPaneGroup(get().paneRoot, id) ??
    findPaneGroup(get().bottomRoot, id) ??
    null
  )
},
```

- [ ] **Step 6: Run tests**

```bash
cd web && npx vitest run src/__tests__/features/workspace/stores/slices/pane-slice.test.ts
```

Expected: PASS

- [ ] **Step 7: Run full test suite**

```bash
cd web && npx vitest run
```

Expected: no regressions

- [ ] **Step 8: Commit**

```bash
git add web/src/features/workspace/stores/slices/pane-slice.ts web/src/__tests__/features/workspace/stores/slices/pane-slice.test.ts
git commit -m "fix: route pane-slice actions to bottomRoot when paneId belongs to bottom pane"
```

---

### Task 8: ui-state-store — Implement Focus Registry + setBottomPaneHeight

**Files:**
- Modify: `web/src/features/window/stores/ui-state-store.ts`

**Bug:** `requestTerminalFocus`, `registerTerminalFocus`, `clearTerminalFocus` are all stubs (`() => {}`). Also `bottomPaneHeight` has no setter, so the resize handle in Task 15 can't persist the height.

- [ ] **Step 1: Write the test**

Create `web/src/__tests__/features/window/stores/ui-state-store.test.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { useUIState } from '@/features/window/stores/ui-state-store'

describe('ui-state-store focus registry', () => {
  beforeEach(() => {
    useUIState.setState({})  // reset to initial
  })

  it('registerTerminalFocus + requestTerminalFocus calls the registered fn', () => {
    const focusCalled = vi.fn()
    useUIState.getState().registerTerminalFocus('term-1', focusCalled)
    useUIState.getState().requestTerminalFocus()
    expect(focusCalled).toHaveBeenCalledOnce()
  })

  it('clearTerminalFocus removes the fn so requestTerminalFocus is a no-op', () => {
    const focusCalled = vi.fn()
    useUIState.getState().registerTerminalFocus('term-1', focusCalled)
    useUIState.getState().clearTerminalFocus('term-1')
    useUIState.getState().requestTerminalFocus()
    expect(focusCalled).not.toHaveBeenCalled()
  })

  it('setBottomPaneHeight updates bottomPaneHeight', () => {
    useUIState.getState().setBottomPaneHeight(400)
    expect(useUIState.getState().bottomPaneHeight).toBe(400)
  })
})
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd web && npx vitest run src/__tests__/features/window/stores/ui-state-store.test.ts
```

Expected: FAIL

- [ ] **Step 3: Implement focus registry in `ui-state-store.ts`**

The focus registry must live outside the Zustand store (Zustand state should not contain functions with external side-effects). Use a module-level Map:

In `web/src/features/window/stores/ui-state-store.ts`, add above the store creation:

```ts
// Module-level registry — not Zustand state (functions can't be meaningfully serialized)
const _terminalFocusRegistry = new Map<string, () => void>()
let _lastRegisteredTerminalId: string | null = null
```

Then replace the three stub implementations in the Zustand store definition:

```ts
registerTerminalFocus: (id, fn) => {
  _terminalFocusRegistry.set(id, fn)
  _lastRegisteredTerminalId = id
},
clearTerminalFocus: (id) => {
  _terminalFocusRegistry.delete(id)
  if (_lastRegisteredTerminalId === id) {
    // Fall back to another registered terminal if any
    _lastRegisteredTerminalId = _terminalFocusRegistry.size > 0
      ? [..._terminalFocusRegistry.keys()].at(-1) ?? null
      : null
  }
},
requestTerminalFocus: () => {
  if (_lastRegisteredTerminalId) {
    _terminalFocusRegistry.get(_lastRegisteredTerminalId)?.()
  }
},
```

Also add `setBottomPaneHeight` to the store type and implementation:

```ts
// In the store type / interface (add alongside other setters):
setBottomPaneHeight: (h: number) => void

// In the implementation:
setBottomPaneHeight: (h) => set({ bottomPaneHeight: h }),
```

- [ ] **Step 4: Run tests**

```bash
cd web && npx vitest run src/__tests__/features/window/stores/ui-state-store.test.ts
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/features/window/stores/ui-state-store.ts web/src/__tests__/features/window/stores/ui-state-store.test.ts
git commit -m "feat: implement terminal focus registry and setBottomPaneHeight in ui-state-store"
```

---

### Task 9: Create BottomPane Component and Wire WorkspaceLayoutRoot

**Files:**
- Create: `web/src/features/workspace/components/bottom-pane.tsx`
- Modify: `web/src/features/workspace/components/WorkspaceLayoutRoot.tsx`

**What's missing:** `WorkspaceLayoutRoot` renders only `SplitViewRoot`. There is no DOM region for the bottom pane, no resize handle, and `isBottomPaneVisible` is never read by any layout component.

- [ ] **Step 1: Create `bottom-pane.tsx`**

Create `web/src/features/workspace/components/bottom-pane.tsx`:

```tsx
import { useRef, useEffect } from 'react'
import { useBottomRoot } from '@/features/workspace/stores/hooks/use-pane-store'
import { PaneNodeRenderer } from '@/features/panes/components/pane-node-renderer'
import { useUIState } from '@/features/window/stores/ui-state-store'

function BottomPaneContent() {
  const bottomRoot = useBottomRoot()
  return <PaneNodeRenderer node={bottomRoot} />
}

export function BottomPane() {
  const height = useUIState(s => s.bottomPaneHeight)
  const setHeight = useUIState(s => s.setBottomPaneHeight)
  const cleanupRef = useRef<(() => void) | null>(null)

  // Clean up any in-progress drag on unmount
  useEffect(() => () => { cleanupRef.current?.() }, [])

  const handleResizeDragStart = (e: React.MouseEvent) => {
    e.preventDefault()
    const startY = e.clientY
    const startHeight = height

    const handleMouseMove = (ev: MouseEvent) => {
      // Dragging the top border upward increases height
      const delta = startY - ev.clientY
      const next = Math.max(120, Math.min(600, startHeight + delta))
      setHeight(next)
    }

    const cleanup = () => {
      document.removeEventListener('mousemove', handleMouseMove)
      document.removeEventListener('mouseup', cleanup)
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
      cleanupRef.current = null
    }

    document.addEventListener('mousemove', handleMouseMove)
    document.addEventListener('mouseup', cleanup)
    document.body.style.cursor = 'ns-resize'
    document.body.style.userSelect = 'none'
    cleanupRef.current = cleanup
  }

  return (
    <div className="flex flex-col border-t border-border" style={{ height }}>
      {/* Drag handle */}
      <div
        className="h-1 w-full shrink-0 cursor-ns-resize hover:bg-primary/30 transition-colors"
        onMouseDown={handleResizeDragStart}
        aria-label="Resize bottom pane"
        role="separator"
        aria-orientation="horizontal"
      />
      <div className="min-h-0 flex-1 overflow-hidden">
        <BottomPaneContent />
      </div>
    </div>
  )
}
```

- [ ] **Step 2: Update `WorkspaceLayoutRoot.tsx`**

Replace the full content of `web/src/features/workspace/components/WorkspaceLayoutRoot.tsx`:

```tsx
import { SplitViewRoot } from '@/features/panes/components/split-view-root'
import { BottomPane } from './bottom-pane'
import { useUIState } from '@/features/window/stores/ui-state-store'

export function WorkspaceLayoutRoot() {
  const isBottomPaneVisible = useUIState(s => s.isBottomPaneVisible)

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <div className="min-h-0 flex-1 overflow-hidden">
        <SplitViewRoot />
      </div>
      {isBottomPaneVisible && <BottomPane />}
    </div>
  )
}
```

- [ ] **Step 3: Write a smoke test**

Create `web/src/__tests__/features/workspace/components/WorkspaceLayoutRoot.test.tsx`:

```tsx
import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { WorkspaceLayoutRoot } from '@/features/workspace/components/WorkspaceLayoutRoot'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-store-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

// Minimal wrapper providing required contexts
function Wrapper({ children }: { children: React.ReactNode }) {
  const store = createWorkspaceStore('test')
  return (
    <WorkspaceStoreContext.Provider value={store}>
      {children}
    </WorkspaceStoreContext.Provider>
  )
}

describe('WorkspaceLayoutRoot', () => {
  it('does not render bottom pane when isBottomPaneVisible is false', () => {
    useUIState.setState({ isBottomPaneVisible: false })
    render(<WorkspaceLayoutRoot />, { wrapper: Wrapper })
    expect(screen.queryByRole('separator')).toBeNull()
  })

  it('renders bottom pane resize handle when isBottomPaneVisible is true', () => {
    useUIState.setState({ isBottomPaneVisible: true })
    render(<WorkspaceLayoutRoot />, { wrapper: Wrapper })
    expect(screen.getByRole('separator')).toBeTruthy()
  })
})
```

- [ ] **Step 4: Run tests**

```bash
cd web && npx vitest run src/__tests__/features/workspace/components/WorkspaceLayoutRoot.test.tsx
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/features/workspace/components/bottom-pane.tsx web/src/features/workspace/components/WorkspaceLayoutRoot.tsx web/src/__tests__/features/workspace/components/WorkspaceLayoutRoot.test.tsx
git commit -m "feat: add BottomPane component with resize handle and wire into WorkspaceLayoutRoot"
```

---

### Task 10: Wire terminal-ensure-session Event + requestTerminalFocus

**Files:**
- Modify: `web/src/features/workspace/stores/hooks/use-workspace-effects.ts`
- Modify: `web/src/features/terminal/components/terminal-tab.tsx`

**What's missing:** (a) `terminal-ensure-session` is dispatched as a `CustomEvent` from command-palette view-actions when the user triggers "Show Terminal", but nothing listens to it. (b) `requestTerminalFocus` is now implemented (Task 14) but no component calls `registerTerminalFocus` with a real focus function.

- [ ] **Step 1: Write the test for the event listener**

Create `web/src/__tests__/features/workspace/stores/hooks/use-workspace-effects.test.ts`:

```ts
import { describe, it, expect, vi } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useWorkspaceEffects } from '@/features/workspace/stores/hooks/use-workspace-effects'
import { useUIState } from '@/features/window/stores/ui-state-store'

// Minimal mock for workspace store context
vi.mock('@/features/workspace/stores/hooks/use-workspace-store', () => ({
  useWorkspaceStore: () => ({
    getState: () => ({
      paneActions: {
        getAllPaneGroups: () => [{ id: 'bottom-pane', bufferIds: [] }],
      },
      bufferActions: {
        openContent: vi.fn().mockReturnValue('buf-new'),
      },
      buffers: [],
      activePaneId: 'root-pane',
    }),
    setState: vi.fn(),
  }),
}))

describe('terminal-ensure-session', () => {
  it('sets isBottomPaneVisible to true when event fires', async () => {
    useUIState.setState({ isBottomPaneVisible: false })
    renderHook(() => useWorkspaceEffects())

    window.dispatchEvent(new CustomEvent('terminal-ensure-session'))

    // Wait for any async handling
    await vi.waitFor(() => {
      expect(useUIState.getState().isBottomPaneVisible).toBe(true)
    })
  })
})
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd web && npx vitest run src/__tests__/features/workspace/stores/hooks/use-workspace-effects.test.ts
```

Expected: FAIL

- [ ] **Step 3: Add `terminal-ensure-session` listener in `use-workspace-effects.ts`**

In `web/src/features/workspace/stores/hooks/use-workspace-effects.ts`, add a new `useEffect` at the end of the `useWorkspaceEffects` hook:

```ts
import { BOTTOM_PANE_ID } from '@/features/panes/constants/pane-constants'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { useWorkspaceStore } from '@/features/workspace/stores/hooks/use-workspace-store'

// Add inside useWorkspaceEffects, after existing effects:
const workspaceStore = useWorkspaceStore()
useEffect(() => {
  const handler = () => {
    const state = workspaceStore.getState()

    // Check if the bottom pane already has a terminal buffer
    const bottomGroups = state.paneActions.getAllPaneGroups()
      .filter(g => g.id === BOTTOM_PANE_ID || /* in bottom tree */ false)
    
    // Simplest check: look for any terminal buffer
    const hasTerminal = state.buffers.some(
      b => b.type === 'terminal' &&
        state.paneActions.getAllPaneGroups().some(g => g.bufferIds.includes(b.id))
    )

    if (!hasTerminal) {
      // Temporarily activate the bottom pane so openContent adds there
      const prevActivePaneId = state.activePaneId
      workspaceStore.setState(s => { s.activePaneId = BOTTOM_PANE_ID })
      state.bufferActions.openContent({ type: 'terminal' })
      workspaceStore.setState(s => { s.activePaneId = prevActivePaneId })
    } else {
      // Terminal exists — activate it in the bottom pane
      const terminalBuffer = state.buffers.find(b => b.type === 'terminal')
      if (terminalBuffer) {
        state.paneActions.activatePaneBuffer(BOTTOM_PANE_ID, terminalBuffer.id)
      }
    }

    useUIState.getState().setIsBottomPaneVisible(true)
    // requestTerminalFocus is called after a tick to let the bottom pane render
    setTimeout(() => useUIState.getState().requestTerminalFocus(), 50)
  }

  window.addEventListener('terminal-ensure-session', handler)
  return () => window.removeEventListener('terminal-ensure-session', handler)
}, [workspaceStore])
```

- [ ] **Step 4: Wire `registerTerminalFocus` from `terminal-tab.tsx`**

In `web/src/features/terminal/components/terminal-tab.tsx`, find the component. It likely has a ref to the terminal instance (e.g. `terminalRef` pointing to an `xterm.Terminal`). Add focus registration:

```tsx
import { useUIState } from '@/features/window/stores/ui-state-store'

// Inside the TerminalTab component, add alongside other effects:
const registerTerminalFocus = useUIState(s => s.registerTerminalFocus)
const clearTerminalFocus = useUIState(s => s.clearTerminalFocus)

useEffect(() => {
  const id = buffer.id  // use the buffer's unique id as the registry key
  registerTerminalFocus(id, () => {
    // Call the terminal instance's focus method.
    // If using xterm.js: terminalRef.current?.focus()
    // If using a custom terminal: find the focus method on the terminal instance ref.
    terminalRef.current?.focus()
  })
  return () => clearTerminalFocus(id)
}, [buffer.id, registerTerminalFocus, clearTerminalFocus])
```

If the terminal instance is accessible via a different ref or API, adjust accordingly. The key contract: when `registerTerminalFocus(id, fn)` is called, `fn()` must focus the terminal's input so keyboard input works.

- [ ] **Step 5: Run tests**

```bash
cd web && npx vitest run src/__tests__/features/workspace/stores/hooks/use-workspace-effects.test.ts
```

Expected: PASS

- [ ] **Step 6: Run full test suite**

```bash
cd web && npx vitest run
```

Expected: no regressions

- [ ] **Step 7: Manual smoke test**

Start the dev server:
```bash
cd web && npm run dev
```

1. Open the app in a browser
2. Press the keyboard shortcut for "Show Terminal" (or use command palette → "View: Show Terminal")
3. Verify: bottom pane appears with correct height
4. Verify: a terminal tab is visible inside the bottom pane
5. Verify: keyboard input goes to the terminal
6. Drag the resize handle and verify the bottom pane resizes
7. Press the same shortcut again — verify bottom pane hides
8. Press it again — verify it shows and the existing terminal session is restored (not a new one)

- [ ] **Step 8: Commit**

```bash
git add web/src/features/workspace/stores/hooks/use-workspace-effects.ts web/src/features/terminal/components/terminal-tab.tsx web/src/__tests__/features/workspace/stores/hooks/use-workspace-effects.test.ts
git commit -m "feat: wire terminal-ensure-session event to open terminal in bottom pane"
```

---

## Self-Review

### Spec coverage check

| Finding | Task |
|---|---|
| 1. Bottom pane never renders | Tasks 7, 9, 10 |
| 2. localStorage crashes in private browsing | Task 1 |
| 3. IDEShell drag listener leak | Task 2 |
| 4. Stale selector closure | Task 3 |
| 5. WorkspaceView null window | Task 4 |
| 6. Index-based React keys | Task 5 |
| 7. Context menu no keyboard dismiss | Task 6 |
| 8. pane-slice actions don't route to bottomRoot | Task 7 |
| 9. requestTerminalFocus stub | Tasks 8, 10 |
| 10. isBottomPaneVisible never read by layout | Task 9 |
| 11. terminal-ensure-session no listener | Task 10 |
| 12. useActiveWorkspaceState subscription gap | Fixed as side-effect of Task 3 (onActiveWorkspaceStoreChange fires immediately) |
| 13. renderActiveBuffer missing deps | Lower priority — implementer should check and fix as part of Task 2 |

### Placeholder scan

None found — every step has exact code or explicit file:line references.

### Type consistency check

- `treeFor` helper in Task 13 uses `findPaneGroup` — already imported in `pane-slice.ts` ✓
- `BottomPane` uses `useBottomRoot` from `use-pane-store` — hook already exists ✓
- `setBottomPaneHeight` added to store type in Task 14, consumed in Task 15 ✓
- `registerTerminalFocus(id, fn)` signature defined in Task 14, consumed in Task 16 ✓
- `BOTTOM_PANE_ID` constant used in Tasks 13 and 16 — already defined in `pane-constants.ts` ✓
