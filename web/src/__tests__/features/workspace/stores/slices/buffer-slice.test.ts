import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import {
  createBufferSlice,
  type BufferSlice,
} from '@/features/workspace/stores/slices/buffer-slice'
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

  // Closing an agent-chat tab STOPS its vendor CLI (so the process doesn't leak)
  // but must KEEP the chat entry resumable — the backend goes dormant, not deleted.
  // stopChat is called with the buffer's OWN wsId + chatId, and deleteChat is never
  // touched (closing a tab is not deleting a chat).
  it('closeBuffer stops the agent CLI of an agentChat buffer (keeps it resumable)', async () => {
    stopChat.mockClear()
    deleteChat.mockClear()
    killTerminalSession.mockClear()
    const id = store.getState().bufferActions.openContent({
      type: 'agentChat',
      chatId: 'chat-7',
      wsId: 'ws-agent',
      name: 'Fix the bug',
    })
    store.getState().bufferActions.closeBuffer(id)
    expect(store.getState().buffers).toHaveLength(0)
    // The stop goes through a dynamic import — flush microtasks.
    await vi.waitFor(() => expect(stopChat).toHaveBeenCalledWith('ws-agent', 'chat-7'))
    expect(deleteChat).not.toHaveBeenCalled()
    expect(killTerminalSession).not.toHaveBeenCalled()
  })

  // Closing a shell terminal STILL hard-kills its PTY (unchanged) and must NOT call the
  // agent stop endpoint — the two close behaviours are distinct.
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

  // Closing an already-dormant chat (no runner, runnerId '') is a safe no-op: the FE
  // still calls stopChat (the backend no-ops on a chat with no live CLI), never
  // deleteChat, and the tab just closes.
  it('closeBuffer on a dormant agentChat still calls stopChat and never deletes the chat', async () => {
    stopChat.mockClear()
    deleteChat.mockClear()
    const id = store.getState().bufferActions.openContent({
      type: 'agentChat',
      chatId: 'chat-dormant',
      wsId: 'ws-agent',
      name: 'Dormant chat',
      runnerId: '',
    })
    store.getState().bufferActions.closeBuffer(id)
    expect(store.getState().buffers).toHaveLength(0)
    await vi.waitFor(() => expect(stopChat).toHaveBeenCalledWith('ws-agent', 'chat-dormant'))
    expect(deleteChat).not.toHaveBeenCalled()
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
