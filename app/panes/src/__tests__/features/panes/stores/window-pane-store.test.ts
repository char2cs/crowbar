import { describe, it, expect, beforeEach, afterEach, onTestFinished, vi } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'

vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))

import { saveWorkspaceLayout } from '@/lib/persistence/workspace-layout'
import { createWindowPaneStore, windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'
import {
  setActiveWorkspaceId,
  getOrCreateWorkspaceStore,
  destroyWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import { workspaceRef } from '@/lib/api/documents'

async function flushMicrotasks(): Promise<void> {
  for (let i = 0; i < 5; i++) await Promise.resolve()
}

const mockSave = saveWorkspaceLayout as ReturnType<typeof vi.fn>

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
      doc: workspaceRef('ws-evicted-2', '/src/a.ts'),
      name: 'a.ts',
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
    vi.runOnlyPendingTimers()
    mockSave.mockClear()
  })

  it('debounces a persisted-field mutation and saves once after 300ms', () => {
    windowPaneStore.getState().paneActions.openChat('chat-1')

    expect(mockSave).not.toHaveBeenCalled()

    vi.advanceTimersByTime(400)

    expect(mockSave).toHaveBeenCalledTimes(1)
  })

  it('a rapid second mutation re-arms the debounce instead of double-saving', () => {
    windowPaneStore.getState().paneActions.openChat('chat-a')
    vi.advanceTimersByTime(100)
    windowPaneStore.getState().paneActions.openChat('chat-b')
    vi.advanceTimersByTime(100)
    expect(mockSave).not.toHaveBeenCalled()

    vi.advanceTimersByTime(300)

    expect(mockSave).toHaveBeenCalledTimes(1)
  })

  it('saves a tab without the reason its document failed to load: the boot reads it again', () => {
    const { bufferActions } = windowPaneStore.getState()
    const id = bufferActions.openContent({
      type: 'editor',
      doc: workspaceRef('ws-1', 'a.ts'),
      name: 'a.ts',
    })
    const unavailable = { title: 'Failed to open file', message: 'a.ts' }
    windowPaneStore.setState((s) => ({
      buffers: s.buffers.map((b) => (b.id === id ? { ...b, unavailable } : b)),
    }))
    vi.advanceTimersByTime(300)

    const saved = mockSave.mock.lastCall![0] as { buffers: { id: string }[] }
    expect(saved.buffers.find((b) => b.id === id)).not.toHaveProperty('unavailable')
    expect(windowPaneStore.getState().buffers[0]).toMatchObject({ unavailable })
  })

  it('reports a layout save that fails instead of leaving it unhandled', async () => {
    const error = vi.spyOn(console, 'error').mockImplementation(() => {})
    onTestFinished(() => error.mockRestore())
    mockSave.mockRejectedValueOnce(new Error('IndexedDB is unavailable'))

    windowPaneStore.getState().paneActions.openChat('chat-1')
    vi.advanceTimersByTime(300)
    await flushMicrotasks()

    expect(error).toHaveBeenCalledWith('Saving the window layout failed:', expect.any(Error))
  })
})
