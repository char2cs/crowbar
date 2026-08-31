import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'

// Mock the IDB-backed persistence so tests don't need a real IndexedDB write
// path, and so we can assert exactly when saveWorkspaceLayout is called.
vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/editor/stores/buffer-session-persistence', () => ({
  saveSessionToStore: vi.fn(),
  clearQueuedWorkspaceSessionSave: vi.fn(),
}))

import { saveWorkspaceLayout } from '@/lib/persistence/workspace-layout'
import { saveSessionToStore } from '@/features/editor/stores/buffer-session-persistence'
import {
  createWindowPaneStore,
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'

const mockSave = saveWorkspaceLayout as ReturnType<typeof vi.fn>
const mockSaveSession = saveSessionToStore as ReturnType<typeof vi.fn>

// Fake timers for EVERY test in this file, not just the "persistence
// subscription" describe block below: `createWindowPaneStore()` wires a real
// 300ms-debounced `setTimeout` on construction, and a test elsewhere in this
// file that creates a store and mutates it (without awaiting/advancing)
// would otherwise leave a REAL background timer running that can fire mid-
// way through a LATER fake-timer test and double-count `mockSave`.
beforeEach(() => {
  vi.useFakeTimers()
  mockSave.mockClear()
  mockSaveSession.mockClear()
})

afterEach(() => {
  vi.runOnlyPendingTimers()
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe('createWindowPaneStore', () => {
  it('pane layout survives a workspace switch', () => {
    // Task 26's own regression test for the trap the model spec names:
    // panes/buffers used to live on the per-workspace store the registry
    // destroyed on eviction/switch. The window-level store is a singleton,
    // untouched by which workspace happens to be active. `ROOT_PANE_ID` is
    // the one pane every fresh store actually creates — setPaneChat is a
    // no-op against an id with no backing pane.
    const store = createWindowPaneStore()
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')

    // Simulate switching workspaces — the active-workspace pointer changes,
    // but nothing about the window pane store is torn down or recreated.
    setActiveWorkspaceId('ws-2')

    expect(store.getState().paneActions.getPaneById(ROOT_PANE_ID)?.chatId).toBe('chat-1')
  })

  it('initialises with the same root/bottom leaves as the old per-workspace store', () => {
    const store = createWindowPaneStore()
    const state = store.getState()
    expect(Object.keys(state.panes).sort()).toEqual(['bottom-pane', 'root-pane'])
    expect(state.activePaneId).toBe(ROOT_PANE_ID)
    expect(state.buffers).toEqual([])
    expect(state.dormantArrangements).toEqual([])
  })

  it('is a fresh, independent instance per call — not the module singleton', () => {
    const a = createWindowPaneStore()
    const b = createWindowPaneStore()
    expect(a).not.toBe(b)
    a.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-only-in-a', null)
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
})

describe('windowPaneStore — persistence subscription', () => {
  beforeEach(() => {
    resetWindowPaneStoreForTests()
    // The reset above is itself a persisted-field change (fresh panes/
    // buffers references) — flush it out so each test starts from a clean
    // mock-call slate, not just a clean call COUNT.
    vi.runOnlyPendingTimers()
    mockSave.mockClear()
    mockSaveSession.mockClear()
  })

  it('debounces a persisted-field mutation and saves once after 300ms', () => {
    windowPaneStore.getState().paneActions.setActivePane('some-pane-id')

    // Not yet — the write is debounced.
    expect(mockSave).not.toHaveBeenCalled()

    vi.advanceTimersByTime(400)

    expect(mockSave).toHaveBeenCalledTimes(1)
  })

  it('a rapid second mutation re-arms the debounce instead of double-saving', () => {
    windowPaneStore.getState().paneActions.setActivePane('pane-a')
    vi.advanceTimersByTime(100)
    windowPaneStore.getState().paneActions.setActivePane('pane-b')
    vi.advanceTimersByTime(100)
    // Still inside the re-armed 300ms window from the second mutation.
    expect(mockSave).not.toHaveBeenCalled()

    vi.advanceTimersByTime(300)

    expect(mockSave).toHaveBeenCalledTimes(1)
  })

  it('a buffers-identity change fires the session writer', () => {
    windowPaneStore.getState().bufferActions.openContent({
      type: 'terminal',
      workspaceId: 'ws-1',
    })
    expect(mockSaveSession).toHaveBeenCalled()
  })
})
