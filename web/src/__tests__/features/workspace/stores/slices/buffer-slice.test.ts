import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import {
  createBufferSlice,
  type BufferSlice,
} from '@/features/workspace/stores/slices/buffer-slice'

const { killTerminalSession } = vi.hoisted(() => ({
  killTerminalSession: vi.fn(async () => {}),
}))

vi.mock('@/features/terminal/lib/kill-terminal-session', () => ({
  killTerminalSession,
}))

const { destroyConversationStore } = vi.hoisted(() => ({
  destroyConversationStore: vi.fn(),
}))

vi.mock('@/features/markdown-chat/stores/conversation-store', () => ({
  destroyConversationStore,
}))

const { clearReconnect } = vi.hoisted(() => ({
  clearReconnect: vi.fn(),
}))

vi.mock('@/features/terminal/lib/terminal-reconnect-map', () => ({
  clearReconnect,
  saveReconnect: vi.fn(),
  loadReconnect: vi.fn(() => null),
}))

const makePaneActions = () => ({
  addBufferToPane: vi.fn(),
  setPanePreviewBuffer: vi.fn(),
  removeBufferFromPane: vi.fn(),
  clearPreviewBufferEverywhere: vi.fn(),
  getPaneById: vi.fn(() => null),
})

type PaneActions = ReturnType<typeof makePaneActions>

function makeStore(paneActions: PaneActions = makePaneActions(), workspaceId = 'ws-test') {
  const store = createStore<BufferSlice & { paneActions: PaneActions; workspaceId: string }>()(
    immer((set, get) => ({
      ...createBufferSlice(...([set, get, {}] as unknown as Parameters<typeof createBufferSlice>)),
      paneActions,
      workspaceId,
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

  it('openContent creates a crowbarChat buffer', () => {
    const id = store.getState().bufferActions.openContent({
      type: 'crowbarChat',
      wsId: 'ws-1',
      name: 'Chat',
    })
    expect(id).toBeTruthy()
    const buf = store.getState().bufferActions.getBufferById(id)
    expect(buf?.type).toBe('crowbarChat')
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
    await vi.waitFor(() =>
      expect(clearReconnect).toHaveBeenCalledWith('ws-99', 'sess-reconnect'),
    )
  })

  // H10: each "New Conversation" mints a fresh wsId-keyed conversation store
  // holding the full streamed turns[]. Closing the chat tab is final, so the
  // store must be destroyed on close — otherwise every closed chat leaks its
  // entire message history for the lifetime of the session.
  it('closeBuffer destroys the conversation store of a crowbarChat buffer', async () => {
    destroyConversationStore.mockClear()
    const id = store.getState().bufferActions.openContent({
      type: 'crowbarChat',
      wsId: 'chat-ws-7',
      name: 'Chat',
    })
    store.getState().bufferActions.closeBuffer(id)
    expect(store.getState().buffers).toHaveLength(0)
    // The teardown goes through a dynamic import — flush microtasks.
    await vi.waitFor(() => expect(destroyConversationStore).toHaveBeenCalledWith('chat-ws-7'))
  })

  it('closeBuffer does not destroy conversation stores for non-chat buffers', async () => {
    destroyConversationStore.mockClear()
    const id = store.getState().bufferActions.openContent({
      type: 'editor',
      path: '/y.ts',
      name: 'y.ts',
      content: '',
    })
    store.getState().bufferActions.closeBuffer(id)
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(destroyConversationStore).not.toHaveBeenCalled()
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

    it('calls clearPreviewBufferEverywhere with the buffer id', () => {
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
      expect(paneActions.clearPreviewBufferEverywhere).toHaveBeenCalledWith(id)
    })

    it('does nothing when buffer id is not found', () => {
      const paneActions = makePaneActions()
      const { store: storeInst } = makeStore(paneActions)
      // no buffer in store
      storeInst.getState().bufferActions.promotePreview('nonexistent-id')
      expect(paneActions.clearPreviewBufferEverywhere).not.toHaveBeenCalled()
    })
  })
})
