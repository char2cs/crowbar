import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'

// Mock the IDB-backed persistence so tests don't need a real IndexedDB write
// path, and so we can assert exactly when saveWorkspaceLayout is called.
vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))

import { saveWorkspaceLayout } from '@/lib/persistence/workspace-layout'
import {
  createWindowPaneStore,
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import {
  setActiveWorkspaceId,
  getOrCreateWorkspaceStore,
  destroyWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'

/** Flush the microtask queue a few times — enough for one dynamic `import()`
 *  plus its `.then()` chain (destroyWorkspaceStore's window-pane-store
 *  teardown) to settle. Fake timers (active file-wide in this suite) only
 *  fake macrotasks/timers, never microtasks, so plain awaits suffice. */
async function flushMicrotasks(): Promise<void> {
  for (let i = 0; i < 5; i++) await Promise.resolve()
}

const mockSave = saveWorkspaceLayout as ReturnType<typeof vi.fn>

// Fake timers for EVERY test in this file, not just the "persistence
// subscription" describe block below: `createWindowPaneStore()` wires a real
// 300ms-debounced `setTimeout` on construction, and a test elsewhere in this
// file that creates a store and mutates it (without awaiting/advancing)
// would otherwise leave a REAL background timer running that can fire mid-
// way through a LATER fake-timer test and double-count `mockSave`.
beforeEach(() => {
  vi.useFakeTimers()
  mockSave.mockClear()
})

afterEach(() => {
  vi.runOnlyPendingTimers()
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe('createWindowPaneStore', () => {
  it('pane layout survives switching the active workspace pointer', () => {
    // The active-workspace pointer changing alone (no destroy) was never the
    // actual trap — see the singleton test below for the real one
    // (destroyWorkspaceStore). This just pins that a plain switch is a no-op
    // for panes/buffers, on an isolated store.
    const store = createWindowPaneStore()
    store.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })

    setActiveWorkspaceId('ws-2')

    expect(store.getState().paneActions.getPaneById(ROOT_PANE_ID)?.chatId).toBe('chat-1')
  })

  it('initialises with the stage and bottom leaves and an empty band', () => {
    const store = createWindowPaneStore()
    const state = store.getState()
    expect(Object.keys(state.panes).sort()).toEqual(['bottom-pane', 'root-pane'])
    expect(state.activePaneId).toBe(ROOT_PANE_ID)
    expect(state.buffers).toEqual([])
    expect(state.viewOrder).toEqual([])
    expect(state.activeViewId).toBeNull()
  })

  it('is a fresh, independent instance per call — not the module singleton', () => {
    const a = createWindowPaneStore()
    const b = createWindowPaneStore()
    expect(a).not.toBe(b)
    a.getState().paneActions.openChat('chat-only-in-a')
    expect(b.getState().panes[ROOT_PANE_ID]?.chatId).toBeNull()
  })
})

describe('windowPaneStore — never destroyed, created once for the window', () => {
  it('the exported singleton is the same object across every import', async () => {
    const a = await import('@/features/panes/stores/window-pane-store')
    const b = await import('@/features/panes/stores/window-pane-store')
    expect(a.windowPaneStore).toBe(b.windowPaneStore)
    expect(a.windowPaneStore).toBe(windowPaneStore)
  })

  // Task 26 fix round 1 (I5): the ACTUAL trap the model spec names is
  // destroyWorkspaceStore — the old per-workspace registry destroyed the
  // whole store panes/buffers lived on, the moment a workspace aged out of
  // keep-alive. The previous version of this test never called
  // destroyWorkspaceStore at all (only setActiveWorkspaceId, which was never
  // what destroyed anything) and would have passed identically against the
  // OLD, broken code. This exercises the REAL singleton and the REAL
  // destroy call.
  it("pane layout survives destroyWorkspaceStore for the chat's own (evicted) workspace", async () => {
    resetWindowPaneStoreForTests()
    getOrCreateWorkspaceStore('ws-evicted')

    windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })

    destroyWorkspaceStore('ws-evicted')
    await flushMicrotasks()

    expect(windowPaneStore.getState().paneActions.getPaneById(ROOT_PANE_ID)?.chatId).toBe('chat-1')
  })

  it('an open editor buffer survives destroyWorkspaceStore for its own (evicted) workspace', async () => {
    resetWindowPaneStoreForTests()
    getOrCreateWorkspaceStore('ws-evicted-2')

    const bufferId = windowPaneStore.getState().bufferActions.openContent({
      type: 'editor',
      path: '/src/a.ts',
      name: 'a.ts',
      content: 'hello',
      workspaceId: 'ws-evicted-2',
    })

    destroyWorkspaceStore('ws-evicted-2')
    await flushMicrotasks()

    const state = windowPaneStore.getState()
    expect(state.buffers.some((b) => b.id === bufferId)).toBe(true)
    expect(state.panes[ROOT_PANE_ID]?.editorTabIds).toContain(bufferId)
  })
})

describe('windowPaneStore — persistence subscription', () => {
  beforeEach(() => {
    resetWindowPaneStoreForTests()
    // The reset above is itself a persisted-field change (fresh panes/
    // buffers references) — flush it out so each test starts from a clean
    // mock-call slate, not just a clean call COUNT.
    vi.runOnlyPendingTimers()
    mockSave.mockClear()
  })

  it('debounces a persisted-field mutation and saves once after 300ms', () => {
    windowPaneStore.getState().paneActions.openChat('chat-1')

    // Not yet — the write is debounced.
    expect(mockSave).not.toHaveBeenCalled()

    vi.advanceTimersByTime(400)

    expect(mockSave).toHaveBeenCalledTimes(1)
  })

  it('a rapid second mutation re-arms the debounce instead of double-saving', () => {
    windowPaneStore.getState().paneActions.openChat('chat-a')
    vi.advanceTimersByTime(100)
    windowPaneStore.getState().paneActions.openChat('chat-b')
    vi.advanceTimersByTime(100)
    // Still inside the re-armed 300ms window from the second mutation.
    expect(mockSave).not.toHaveBeenCalled()

    vi.advanceTimersByTime(300)

    expect(mockSave).toHaveBeenCalledTimes(1)
  })
})
