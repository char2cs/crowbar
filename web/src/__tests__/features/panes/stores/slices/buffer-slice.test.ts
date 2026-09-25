import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  createWindowPaneStore,
  type WindowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { ROOT_PANE_ID, BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
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

vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))

const editor = (path: string, extra: { isPreview?: boolean; workspaceId?: string } = {}) => ({
  type: 'editor' as const,
  path,
  name: path.split('/').pop() ?? path,
  content: '',
  ...extra,
})

/** Close a tab the way every close affordance does: the pane lets go. */
function closeTab(store: WindowPaneStore, paneId: string, id: string): void {
  store.getState().paneActions.removeEditorTabFromPane(paneId, id)
  store.getState().bufferActions.closeBuffer(id)
}

describe('buffer-slice', () => {
  let store: WindowPaneStore
  beforeEach(() => {
    store = createWindowPaneStore()
  })

  it('starts empty', () => {
    expect(store.getState().buffers).toHaveLength(0)
  })

  it('openContent creates an editor buffer and returns its id', () => {
    const id = store.getState().bufferActions.openContent({
      ...editor('/src/index.ts'),
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
    const id1 = store.getState().bufferActions.openContent(editor('/src/index.ts'))
    const id2 = store.getState().bufferActions.openContent(editor('/src/index.ts'))
    expect(id1).toBe(id2)
    expect(store.getState().buffers).toHaveLength(1)
  })

  it('openContent seats the new tab in the focused pane by default', () => {
    const id = store.getState().bufferActions.openContent(editor('/src/index.ts'))
    expect(store.getState().panes[ROOT_PANE_ID].editorTabIds).toEqual([id])
    expect(store.getState().panes[ROOT_PANE_ID].activeEditorTabId).toBe(id)
  })

  it('openContent lands in the pane it names, whatever has focus (C8)', () => {
    const id = store
      .getState()
      .bufferActions.openContent(editor('/src/index.ts'), { paneId: BOTTOM_PANE_ID })
    expect(store.getState().activePaneId).toBe(ROOT_PANE_ID)
    expect(store.getState().panes[BOTTOM_PANE_ID].editorTabIds).toEqual([id])
    expect(store.getState().panes[ROOT_PANE_ID].editorTabIds).toEqual([])
  })

  it('openContent into a pane that does not exist creates nothing', () => {
    expect(
      store.getState().bufferActions.openContent(editor('/a.ts'), { paneId: 'no-such-pane' }),
    ).toBe('')
    expect(store.getState().buffers).toEqual([])
  })

  it('opening an already-open terminal reveals its pane instead of adding a copy', () => {
    const { bufferActions } = store.getState()
    const id = bufferActions.openContent(
      { type: 'terminal', sessionId: 'sess-1', name: 'Terminal 1' },
      { paneId: BOTTOM_PANE_ID },
    )

    const again = bufferActions.openContent({
      type: 'terminal',
      sessionId: 'sess-1',
      name: 'Terminal 1',
    })

    expect(again).toBe(id)
    expect(store.getState().activePaneId).toBe(BOTTOM_PANE_ID)
    expect(store.getState().panes[BOTTOM_PANE_ID].activeEditorTabId).toBe(id)
    expect(store.getState().panes[ROOT_PANE_ID].editorTabIds).toEqual([])
  })

  it('openNewTab no longer exists — a pane with no tabs shows its own empty state for free', () => {
    expect(
      (store.getState().bufferActions as unknown as Record<string, unknown>).openNewTab,
    ).toBeUndefined()
  })

  it('closing a tab removes its buffer from the list', () => {
    const id = store.getState().bufferActions.openContent(editor('/a.ts'))
    closeTab(store, ROOT_PANE_ID, id)
    expect(store.getState().buffers).toHaveLength(0)
  })

  it('closeBuffer leaves a buffer a pane still lists alone', () => {
    const id = store.getState().bufferActions.openContent(editor('/a.ts'))
    store.getState().bufferActions.closeBuffer(id)
    expect(store.getState().buffers.map((b) => b.id)).toEqual([id])
  })

  // M6: the markdown rich/source preference is keyed by bufferId and nothing
  // else ever removed an entry, so `views` grew for the life of the session.
  it('closing a tab releases the buffer’s markdown view preference', () => {
    const id = store.getState().bufferActions.openContent(editor('/notes.md'))
    useMarkdownViewStore.getState().setView(id, 'source')
    expect(useMarkdownViewStore.getState().views[id]).toBe('source')

    closeTab(store, ROOT_PANE_ID, id)

    expect(useMarkdownViewStore.getState().views).toEqual({})
  })

  // BUG-015: closing a terminal tab is final (terminals never enter the
  // undo-close history), so the backend PTY must be killed on close —
  // otherwise every closed tab leaks a live shell process.
  it('closing a terminal tab kills the backend PTY session', async () => {
    killTerminalSession.mockClear()
    const id = store.getState().bufferActions.openContent({
      type: 'terminal',
      sessionId: 'sess-9',
      name: 'Terminal 1',
    })
    closeTab(store, ROOT_PANE_ID, id)
    expect(store.getState().buffers).toHaveLength(0)
    await vi.waitFor(() => expect(killTerminalSession).toHaveBeenCalledWith('sess-9'))
  })

  it('closing a terminal tab clears its reconnect map entry', async () => {
    clearReconnect.mockClear()
    const id = store.getState().bufferActions.openContent({
      type: 'terminal',
      sessionId: 'sess-reconnect',
      name: 'Terminal 2',
      workspaceId: 'ws-99',
    })
    closeTab(store, ROOT_PANE_ID, id)
    await vi.waitFor(() => expect(clearReconnect).toHaveBeenCalledWith('ws-99', 'sess-reconnect'))
  })

  it('closing a non-terminal tab kills no PTY', async () => {
    killTerminalSession.mockClear()
    const id = store.getState().bufferActions.openContent(editor('/x.ts'))
    closeTab(store, ROOT_PANE_ID, id)
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(killTerminalSession).not.toHaveBeenCalled()
  })

  // A chat is no longer a buffer (it is `PaneGroup.chatId`), so closing a
  // terminal never reaches the agent API.
  it('closing a terminal tab does not stop an agent CLI', async () => {
    stopChat.mockClear()
    const id = store.getState().bufferActions.openContent({
      type: 'terminal',
      sessionId: 'sess-term',
      name: 'Terminal 1',
    })
    closeTab(store, ROOT_PANE_ID, id)
    await vi.waitFor(() => expect(killTerminalSession).toHaveBeenCalledWith('sess-term'))
    expect(stopChat).not.toHaveBeenCalled()
  })

  it('preview flag is set when isPreview is true', () => {
    const id = store.getState().bufferActions.openContent(editor('/b.ts', { isPreview: true }))
    expect(store.getState().bufferActions.getBufferById(id)?.isPreview).toBe(true)
  })

  it('pin toggles isPinned on the buffer', () => {
    const id = store.getState().bufferActions.openContent(editor('/c.ts'))
    store.getState().bufferActions.setPinned(id, true)
    expect(store.getState().bufferActions.getBufferById(id)?.isPinned).toBe(true)
    store.getState().bufferActions.setPinned(id, false)
    expect(store.getState().bufferActions.getBufferById(id)?.isPinned).toBe(false)
  })

  describe('promotePreview', () => {
    it('clears the preview flag on every buffer', () => {
      const { bufferActions } = store.getState()
      const id = bufferActions.openContent(editor('/src/a.ts', { isPreview: true }))
      const other = bufferActions.openContent(editor('/src/b.ts', { isPreview: true }), {
        paneId: BOTTOM_PANE_ID,
      })
      bufferActions.promotePreview(id)
      expect(bufferActions.getBufferById(id)?.isPreview).toBe(false)
      expect(bufferActions.getBufferById(other)?.isPreview).toBe(false)
    })

    it('does nothing when buffer id is not found', () => {
      const before = store.getState()
      store.getState().bufferActions.promotePreview('nonexistent-id')
      expect(store.getState()).toBe(before)
    })
  })

  describe('sole editor tab closeability', () => {
    it('the sole editor tab in a pane is marked uncloseable', () => {
      const id = store.getState().bufferActions.openContent(editor('/test/foo.ts'))
      expect(store.getState().panes[ROOT_PANE_ID].editorTabIds).toHaveLength(1)
      expect(store.getState().bufferActions.getBufferById(id)?.isUncloseable).toBe(true)
    })

    it('adding a second tab clears isUncloseable on both tabs', () => {
      const { bufferActions } = store.getState()
      const first = bufferActions.openContent(editor('/test/foo.ts'))
      expect(bufferActions.getBufferById(first)?.isUncloseable).toBe(true)
      const second = bufferActions.openContent(editor('/test/bar.ts'))
      expect(store.getState().panes[ROOT_PANE_ID].editorTabIds).toHaveLength(2)
      expect(bufferActions.getBufferById(first)?.isUncloseable).toBe(false)
      expect(bufferActions.getBufferById(second)?.isUncloseable).toBe(false)
    })

    it('removing tabs down to one marks that tab as uncloseable again', () => {
      const { bufferActions, paneActions } = store.getState()
      const first = bufferActions.openContent(editor('/test/foo.ts'))
      const second = bufferActions.openContent(editor('/test/bar.ts'))
      paneActions.removeEditorTabFromPane(ROOT_PANE_ID, first)
      expect(bufferActions.getBufferById(second)?.isUncloseable).toBe(true)
    })

    it('moveEditorTabToPane syncs isUncloseable on both source and destination panes', () => {
      const { bufferActions, paneActions } = store.getState()
      // ROOT and BOTTOM are each alone in their own tree, so a sole tab in
      // either stays protected.
      const stay = bufferActions.openContent(editor('/test/stay.ts'))
      const move = bufferActions.openContent(editor('/test/move.ts'))
      const inB = bufferActions.openContent(editor('/test/b1.ts'), { paneId: BOTTOM_PANE_ID })
      expect(bufferActions.getBufferById(inB)?.isUncloseable).toBe(true)
      expect(bufferActions.getBufferById(stay)?.isUncloseable).toBe(false)

      paneActions.moveEditorTabToPane(move, ROOT_PANE_ID, BOTTOM_PANE_ID)

      expect(store.getState().panes[ROOT_PANE_ID].editorTabIds).toHaveLength(1)
      expect(store.getState().panes[BOTTOM_PANE_ID].editorTabIds).toHaveLength(2)
      expect(bufferActions.getBufferById(stay)?.isUncloseable).toBe(true)
      expect(bufferActions.getBufferById(inB)?.isUncloseable).toBe(false)
      expect(bufferActions.getBufferById(move)?.isUncloseable).toBe(false)
    })

    // Closing a split pane's last editor tab collapses the split into its
    // sibling, so a sole tab in a split pane stays closeable.
    it('a sole editor tab in a pane that is part of a split stays closeable', () => {
      const { bufferActions, paneActions } = store.getState()
      const paneB = paneActions.splitPane(ROOT_PANE_ID, 'horizontal')!
      const id = bufferActions.openContent(editor('/test/split-tab.ts'), { paneId: paneB })
      expect(store.getState().panes[paneB].editorTabIds).toHaveLength(1)
      expect(bufferActions.getBufferById(id)?.isUncloseable).toBe(false)
    })
  })
})
