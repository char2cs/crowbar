import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import {
  createBufferSlice,
  type BufferSlice,
} from '@/features/workspace/stores/slices/buffer-slice'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { useMarkdownViewStore } from '@/features/editor/markdown/plate/markdown-view-store'

const { killTerminalSession } = vi.hoisted(() => ({
  killTerminalSession: vi.fn(async () => {}),
}))

vi.mock('@/features/terminal/lib/kill-terminal-session', () => ({
  killTerminalSession,
}))

const { clearReconnect } = vi.hoisted(() => ({
  clearReconnect: vi.fn(),
}))

vi.mock('@/features/terminal/lib/terminal-reconnect-map', () => ({
  clearReconnect,
  saveReconnect: vi.fn(),
  loadReconnect: vi.fn(() => null),
}))

const { stopChat, deleteChat } = vi.hoisted(() => ({
  stopChat: vi.fn(async () => {}),
  deleteChat: vi.fn(async () => {}),
}))

vi.mock('@/features/agent/api/agent-api', () => ({
  stopChat,
  deleteChat,
}))

const makePaneActions = () => ({
  addEditorTabToPane: vi.fn(),
  setEditorTabPreview: vi.fn(),
  removeEditorTabFromPane: vi.fn(),
  setActivePane: vi.fn(),
  activateEditorTabInPane: vi.fn(),
  // pane-slice's real name post-Task-2 (renamed from clearPreviewBufferEverywhere,
  // and dropped its `id` param — it clears every buffer's isPreview flag).
  clearEditorTabPreviewEverywhere: vi.fn(),
  getPaneById: vi.fn(() => null),
  getPaneByEditorTabId: vi.fn((): { id: string } | null => null),
})

type PaneActions = ReturnType<typeof makePaneActions>

function makeStore(paneActions: PaneActions = makePaneActions(), workspaceId = 'ws-test') {
  const store = createStore<
    BufferSlice & { paneActions: PaneActions; workspaceId: string; activePaneId: string }
  >()(
    immer((set, get) => ({
      ...createBufferSlice(...([set, get, {}] as unknown as Parameters<typeof createBufferSlice>)),
      paneActions,
      workspaceId,
      activePaneId: ROOT_PANE_ID,
    })),
  )
  return { store, paneActions }
}

describe('buffer-slice', () => {
  let store: ReturnType<typeof makeStore>['store']
  beforeEach(() => {
    store = makeStore().store
  })

  it('starts empty', () => {
    expect(store.getState().buffers).toHaveLength(0)
  })

  it('openContent creates an editor buffer and returns its id', () => {
    const id = store.getState().bufferActions.openContent({
      type: 'editor',
      path: '/src/index.ts',
      name: 'index.ts',
      content: 'const x = 1',
    })
    expect(id).toBeTruthy()
    expect(store.getState().buffers).toHaveLength(1)
    const buf = store.getState().buffers[0]
    expect(buf.type).toBe('editor')
    expect(buf.path).toBe('/src/index.ts')
    expect(buf.id).toBe(id)
  })

  it('openContent with the same path returns the existing buffer id', () => {
    const spec = { type: 'editor' as const, path: '/src/index.ts', name: 'index.ts', content: '' }
    const id1 = store.getState().bufferActions.openContent(spec)
    const id2 = store.getState().bufferActions.openContent(spec)
    expect(id1).toBe(id2)
    expect(store.getState().buffers).toHaveLength(1)
  })

  it('openContent adds the new tab to the active pane via addEditorTabToPane, not an old action', () => {
    const paneActions = makePaneActions()
    const { store: storeInst } = makeStore(paneActions)
    const id = storeInst.getState().bufferActions.openContent({
      type: 'editor',
      path: '/src/index.ts',
      name: 'index.ts',
      content: '',
    })
    expect(paneActions.addEditorTabToPane).toHaveBeenCalledTimes(1)
    expect(paneActions.addEditorTabToPane).toHaveBeenCalledWith(
      ROOT_PANE_ID,
      expect.objectContaining({ id, type: 'editor', path: '/src/index.ts' }),
    )
  })

  it('opening an already-open terminal jumps to its existing pane via getPaneByEditorTabId/activateEditorTabInPane', () => {
    const paneActions = makePaneActions()
    const { store: storeInst } = makeStore(paneActions)
    const id = storeInst.getState().bufferActions.openContent({
      type: 'terminal',
      sessionId: 'sess-1',
      name: 'Terminal 1',
    })
    paneActions.getPaneByEditorTabId.mockReturnValue({ id: 'other-pane' })
    paneActions.addEditorTabToPane.mockClear()

    const again = storeInst.getState().bufferActions.openContent({
      type: 'terminal',
      sessionId: 'sess-1',
      name: 'Terminal 1',
    })

    expect(again).toBe(id)
    expect(paneActions.getPaneByEditorTabId).toHaveBeenCalledWith(id)
    expect(paneActions.setActivePane).toHaveBeenCalledWith('other-pane')
    expect(paneActions.activateEditorTabInPane).toHaveBeenCalledWith('other-pane', id)
    // The jump path REVEALS the existing tab — it never also lands a copy via
    // the generic add-to-active-pane path.
    expect(paneActions.addEditorTabToPane).not.toHaveBeenCalled()
  })

  it('openNewTab no longer exists — a pane with no tabs shows its own empty state for free', () => {
    expect(
      (store.getState().bufferActions as unknown as Record<string, unknown>).openNewTab,
    ).toBeUndefined()
  })

  it('closeBuffer removes it from the list', () => {
    const id = store.getState().bufferActions.openContent({
      type: 'editor',
      path: '/a.ts',
      name: 'a.ts',
      content: '',
    })
    store.getState().bufferActions.closeBuffer(id)
    expect(store.getState().buffers).toHaveLength(0)
  })

  // M6: the markdown rich/source preference is keyed by bufferId and nothing
  // else ever removed an entry, so `views` grew for the life of the session.
  it('closeBuffer releases the buffer’s markdown view preference', () => {
    const id = store.getState().bufferActions.openContent({
      type: 'editor',
      path: '/notes.md',
      name: 'notes.md',
      content: '',
    })
    useMarkdownViewStore.getState().setView(id, 'source')
    expect(useMarkdownViewStore.getState().views[id]).toBe('source')

    store.getState().bufferActions.closeBuffer(id)

    expect(useMarkdownViewStore.getState().views).toEqual({})
  })

  // BUG-015: closing a terminal tab is final (terminals never enter the
  // undo-close history), so the backend PTY must be killed on close —
  // otherwise every closed tab leaks a live shell process.
  it('closeBuffer kills the backend PTY session of a terminal buffer', async () => {
    killTerminalSession.mockClear()
    const id = store.getState().bufferActions.openContent({
      type: 'terminal',
      sessionId: 'sess-9',
      name: 'Terminal 1',
    })
    store.getState().bufferActions.closeBuffer(id)
    expect(store.getState().buffers).toHaveLength(0)
    // The kill goes through a dynamic import — flush microtasks.
    await vi.waitFor(() => expect(killTerminalSession).toHaveBeenCalledWith('sess-9'))
  })

  it('closeBuffer clears the reconnect map entry after killing a terminal buffer', async () => {
    clearReconnect.mockClear()
    const { store: localStore } = makeStore(makePaneActions(), 'ws-99')
    const id = localStore.getState().bufferActions.openContent({
      type: 'terminal',
      sessionId: 'sess-reconnect',
      name: 'Terminal 2',
    })
    localStore.getState().bufferActions.closeBuffer(id)
    // clearReconnect fires after killTerminalSession completes (both in the same async chain)
    await vi.waitFor(() => expect(clearReconnect).toHaveBeenCalledWith('ws-99', 'sess-reconnect'))
  })

  it('closeBuffer does not kill PTYs for non-terminal buffers', async () => {
    killTerminalSession.mockClear()
    const id = store.getState().bufferActions.openContent({
      type: 'editor',
      path: '/x.ts',
      name: 'x.ts',
      content: '',
    })
    store.getState().bufferActions.closeBuffer(id)
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(killTerminalSession).not.toHaveBeenCalled()
  })

  // A chat is no longer a buffer (it is `PaneGroup.chatId`), so closeBuffer's
  // old agentChat-specific stopChat behavior is unreachable and was removed
  // along with it — there is no more agentChat spec for openContent to build.
  it('closeBuffer does not stop an agent CLI for a terminal buffer', async () => {
    stopChat.mockClear()
    const id = store.getState().bufferActions.openContent({
      type: 'terminal',
      sessionId: 'sess-term',
      name: 'Terminal 1',
    })
    store.getState().bufferActions.closeBuffer(id)
    await vi.waitFor(() => expect(killTerminalSession).toHaveBeenCalledWith('sess-term'))
    expect(stopChat).not.toHaveBeenCalled()
  })

  it('preview flag is set when isPreview is true', () => {
    const id = store.getState().bufferActions.openContent({
      type: 'editor',
      path: '/b.ts',
      name: 'b.ts',
      content: '',
      isPreview: true,
    })
    const buf = store.getState().bufferActions.getBufferById(id)
    expect(buf?.isPreview).toBe(true)
  })

  it('pin toggles isPinned on the buffer', () => {
    const id = store.getState().bufferActions.openContent({
      type: 'editor',
      path: '/c.ts',
      name: 'c.ts',
      content: '',
    })
    store.getState().bufferActions.setPinned(id, true)
    expect(store.getState().bufferActions.getBufferById(id)?.isPinned).toBe(true)
    store.getState().bufferActions.setPinned(id, false)
    expect(store.getState().bufferActions.getBufferById(id)?.isPinned).toBe(false)
  })

  describe('promotePreview', () => {
    it('sets isPreview to false', () => {
      const { store: storeInst } = makeStore()
      const id = storeInst.getState().bufferActions.openContent({
        type: 'editor',
        path: '/src/a.ts',
        name: 'a.ts',
        content: '',
        isPreview: true,
      })
      storeInst.getState().bufferActions.promotePreview(id)
      expect(storeInst.getState().bufferActions.getBufferById(id)?.isPreview).toBe(false)
    })

    it('calls clearEditorTabPreviewEverywhere', () => {
      const paneActions = makePaneActions()
      const { store: storeInst } = makeStore(paneActions)
      const id = storeInst.getState().bufferActions.openContent({
        type: 'editor',
        path: '/src/a.ts',
        name: 'a.ts',
        content: '',
        isPreview: true,
      })
      storeInst.getState().bufferActions.promotePreview(id)
      expect(paneActions.clearEditorTabPreviewEverywhere).toHaveBeenCalled()
    })

    it('does nothing when buffer id is not found', () => {
      const paneActions = makePaneActions()
      const { store: storeInst } = makeStore(paneActions)
      // no buffer in store
      storeInst.getState().bufferActions.promotePreview('nonexistent-id')
      expect(paneActions.clearEditorTabPreviewEverywhere).not.toHaveBeenCalled()
    })
  })

  describe('sole editor tab closeability', () => {
    it('the sole editor tab in a pane is marked uncloseable', () => {
      const store = createWorkspaceStore('ws-test')
      const tabId = 'tab-sole'
      // Create a buffer manually
      store.setState((state) => {
        state.buffers.push({
          id: tabId,
          type: 'editor',
          path: '/test/foo.ts',
          name: 'foo.ts',
          content: '',
          savedContent: '',
          isDirty: false,
          isVirtual: false,
          tokens: [],
          isPinned: false,
          isPreview: false,
        })
      })
      // Add the tab to the pane
      store.getState().paneActions.addEditorTabToPane(ROOT_PANE_ID, {
        id: tabId,
        type: 'editor',
        name: 'foo.ts',
      })
      // Verify the pane has this tab
      const pane = store.getState().paneActions.getPaneById(ROOT_PANE_ID)
      expect(pane?.editorTabIds).toHaveLength(1)
      // Verify the tab is marked uncloseable
      const tab = store.getState().bufferActions.getBufferById(tabId)
      expect(tab?.isUncloseable).toBe(true)
    })

    it('adding a second tab clears isUncloseable on both tabs', () => {
      const store = createWorkspaceStore('ws-test')
      const tabId1 = 'tab-1'
      const tabId2 = 'tab-2'
      // Create two buffers manually
      store.setState((state) => {
        state.buffers.push({
          id: tabId1,
          type: 'editor',
          path: '/test/foo.ts',
          name: 'foo.ts',
          content: '',
          savedContent: '',
          isDirty: false,
          isVirtual: false,
          tokens: [],
          isPinned: false,
          isPreview: false,
        })
        state.buffers.push({
          id: tabId2,
          type: 'editor',
          path: '/test/bar.ts',
          name: 'bar.ts',
          content: '',
          savedContent: '',
          isDirty: false,
          isVirtual: false,
          tokens: [],
          isPinned: false,
          isPreview: false,
        })
      })
      // Add first tab
      store.getState().paneActions.addEditorTabToPane(ROOT_PANE_ID, {
        id: tabId1,
        type: 'editor',
        name: 'foo.ts',
      })
      // Verify it's marked uncloseable
      const tab1Before = store.getState().bufferActions.getBufferById(tabId1)
      expect(tab1Before?.isUncloseable).toBe(true)
      // Add second tab
      store.getState().paneActions.addEditorTabToPane(ROOT_PANE_ID, {
        id: tabId2,
        type: 'editor',
        name: 'bar.ts',
      })
      // Verify pane has both tabs
      const pane = store.getState().paneActions.getPaneById(ROOT_PANE_ID)
      expect(pane?.editorTabIds).toHaveLength(2)
      // Verify both tabs are no longer uncloseable
      const tab1After = store.getState().bufferActions.getBufferById(tabId1)
      const tab2 = store.getState().bufferActions.getBufferById(tabId2)
      expect(tab1After?.isUncloseable).toBe(false)
      expect(tab2?.isUncloseable).toBe(false)
    })

    it('removing tabs down to one marks that tab as uncloseable again', () => {
      const store = createWorkspaceStore('ws-test')
      const tabId1 = 'tab-remove-1'
      const tabId2 = 'tab-remove-2'
      // Create two buffers manually
      store.setState((state) => {
        state.buffers.push({
          id: tabId1,
          type: 'editor',
          path: '/test/foo.ts',
          name: 'foo.ts',
          content: '',
          savedContent: '',
          isDirty: false,
          isVirtual: false,
          tokens: [],
          isPinned: false,
          isPreview: false,
        })
        state.buffers.push({
          id: tabId2,
          type: 'editor',
          path: '/test/bar.ts',
          name: 'bar.ts',
          content: '',
          savedContent: '',
          isDirty: false,
          isVirtual: false,
          tokens: [],
          isPinned: false,
          isPreview: false,
        })
      })
      // Add two tabs
      store.getState().paneActions.addEditorTabToPane(ROOT_PANE_ID, {
        id: tabId1,
        type: 'editor',
        name: 'foo.ts',
      })
      store.getState().paneActions.addEditorTabToPane(ROOT_PANE_ID, {
        id: tabId2,
        type: 'editor',
        name: 'bar.ts',
      })
      // Both should be non-closeable
      expect(store.getState().bufferActions.getBufferById(tabId1)?.isUncloseable).toBe(false)
      expect(store.getState().bufferActions.getBufferById(tabId2)?.isUncloseable).toBe(false)
      // Remove first tab
      store.getState().paneActions.removeEditorTabFromPane(ROOT_PANE_ID, tabId1)
      // Second tab should now be uncloseable
      expect(store.getState().bufferActions.getBufferById(tabId2)?.isUncloseable).toBe(true)
    })

    it('moveEditorTabToPane syncs isUncloseable on both source and destination panes', () => {
      const store = createWorkspaceStore('ws-test')
      const paneActions = store.getState().paneActions
      // Create pane B (destination)
      const paneBId = paneActions.splitPane(ROOT_PANE_ID, 'horizontal')!
      // Create four buffers: one staying in pane A, one to move from A to B, one already in B
      const tabAStay = 'tab-a-stay'
      const tabToMove = 'tab-to-move'
      const tabBId = 'tab-b1'
      store.setState((state) => {
        state.buffers.push({
          id: tabAStay,
          type: 'editor',
          path: '/test/file-a-stay.ts',
          name: 'file-a-stay.ts',
          content: '',
          savedContent: '',
          isDirty: false,
          isVirtual: false,
          tokens: [],
          isPinned: false,
          isPreview: false,
          isUncloseable: false,
        })
        state.buffers.push({
          id: tabToMove,
          type: 'editor',
          path: '/test/file-to-move.ts',
          name: 'file-to-move.ts',
          content: '',
          savedContent: '',
          isDirty: false,
          isVirtual: false,
          tokens: [],
          isPinned: false,
          isPreview: false,
          isUncloseable: false,
        })
        state.buffers.push({
          id: tabBId,
          type: 'editor',
          path: '/test/file-b1.ts',
          name: 'file-b1.ts',
          content: '',
          savedContent: '',
          isDirty: false,
          isVirtual: false,
          tokens: [],
          isPinned: false,
          isPreview: false,
          isUncloseable: true,
        })
      })
      // Setup: Pane A has 2 tabs (both not uncloseable initially)
      paneActions.addEditorTabToPane(ROOT_PANE_ID, {
        id: tabAStay,
        type: 'editor',
        name: 'file-a-stay.ts',
      })
      paneActions.addEditorTabToPane(ROOT_PANE_ID, {
        id: tabToMove,
        type: 'editor',
        name: 'file-to-move.ts',
      })
      // Setup: Pane B has 1 tab (uncloseable)
      paneActions.addEditorTabToPane(paneBId, {
        id: tabBId,
        type: 'editor',
        name: 'file-b1.ts',
      })
      // Before move: pane A has 2 tabs (both closeable), pane B has 1 tab (uncloseable)
      expect(store.getState().paneActions.getPaneById(ROOT_PANE_ID)?.editorTabIds).toHaveLength(2)
      expect(store.getState().paneActions.getPaneById(paneBId)?.editorTabIds).toHaveLength(1)
      expect(store.getState().bufferActions.getBufferById(tabBId)?.isUncloseable).toBe(true)
      expect(store.getState().bufferActions.getBufferById(tabAStay)?.isUncloseable).toBe(false)
      expect(store.getState().bufferActions.getBufferById(tabToMove)?.isUncloseable).toBe(false)
      // Move tab from pane A to pane B
      paneActions.moveEditorTabToPane(tabToMove, ROOT_PANE_ID, paneBId)
      // After move: pane A has 1 tab, pane B has 2 tabs
      expect(store.getState().paneActions.getPaneById(ROOT_PANE_ID)?.editorTabIds).toHaveLength(1)
      expect(store.getState().paneActions.getPaneById(paneBId)?.editorTabIds).toHaveLength(2)
      // Critical assertion: pane A's remaining tab (tabAStay) should now be uncloseable (it's sole)
      const tabAStayAfter = store.getState().bufferActions.getBufferById(tabAStay)
      expect(tabAStayAfter?.isUncloseable).toBe(true)
      // Critical assertion: pane B's tabs should both be closeable (no longer sole)
      const tabBAfter = store.getState().bufferActions.getBufferById(tabBId)
      const tabMovedAfter = store.getState().bufferActions.getBufferById(tabToMove)
      expect(tabBAfter?.isUncloseable).toBe(false)
      expect(tabMovedAfter?.isUncloseable).toBe(false)
    })
  })
})
