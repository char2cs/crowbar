import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import {
  createBufferSlice,
  type BufferSlice,
} from '@/features/workspace/stores/slices/buffer-slice'

vi.mock('@/features/terminal/lib/kill-terminal-session', () => ({
  killTerminalSession: vi.fn(async () => {}),
}))
vi.mock('@/features/terminal/lib/terminal-reconnect-map', () => ({
  clearReconnect: vi.fn(),
  saveReconnect: vi.fn(),
  loadReconnect: vi.fn(() => null),
}))
vi.mock('@/features/agent/api/agent-api', () => ({
  stopChat: vi.fn(async () => {}),
  deleteChat: vi.fn(async () => {}),
}))

// A pane double that actually tracks membership, so "which pane holds this
// buffer" is a real question the slice can get wrong. Mirrors the REAL
// pane-slice behaviour for unregistered panes: `getPaneById` returns null and
// `addBufferToPane`/`removeBufferFromPane` silently no-op, rather than
// auto-vivifying a pane entry. A double that is more forgiving than the real
// thing would hide exactly the orphaned-buffer bug (Finding 1) this suite
// exists to catch — a pane must be registered in `panes` before it can hold
// anything.
function makePaneActions(panes: Record<string, string[]>) {
  return {
    addBufferToPane: vi.fn((paneId: string, bufferId: string) => {
      if (!(paneId in panes)) return
      if (!panes[paneId].includes(bufferId)) panes[paneId].push(bufferId)
    }),
    removeBufferFromPane: vi.fn((paneId: string, bufferId: string) => {
      if (!(paneId in panes)) return
      panes[paneId] = panes[paneId].filter((id) => id !== bufferId)
    }),
    setPanePreviewBuffer: vi.fn(),
    clearPreviewBufferEverywhere: vi.fn(),
    activatePaneBuffer: vi.fn(),
    setActivePane: vi.fn(),
    getPaneById: vi.fn((id: string) => (id in panes ? { id, bufferIds: panes[id] } : null)),
    getPaneByBufferId: vi.fn((bufferId: string) => {
      const entry = Object.entries(panes).find(([, ids]) => ids.includes(bufferId))
      return entry ? { id: entry[0], bufferIds: entry[1] } : null
    }),
  }
}

function makeStore() {
  const panes: Record<string, string[]> = { root: [] }
  const paneActions = makePaneActions(panes)
  const store = createStore<
    BufferSlice & { paneActions: typeof paneActions; workspaceId: string; activePaneId: string }
  >()(
    immer((set, get) => ({
      ...createBufferSlice(...([set, get, {}] as unknown as Parameters<typeof createBufferSlice>)),
      paneActions,
      workspaceId: 'ws-test',
      activePaneId: 'root',
    })),
  )
  return { store, panes, paneActions }
}

describe('buffer-slice — openNewTab', () => {
  let ctx: ReturnType<typeof makeStore>
  beforeEach(() => {
    ctx = makeStore()
  })

  it('creates a newTab buffer named "New Tab"', () => {
    const id = ctx.store.getState().bufferActions.openNewTab()
    const buf = ctx.store.getState().buffers.find((b) => b.id === id)
    expect(buf?.type).toBe('newTab')
    expect(buf?.name).toBe('New Tab')
  })

  it('called twice on the same pane returns the SAME id and mints only one buffer', () => {
    const first = ctx.store.getState().bufferActions.openNewTab()
    const second = ctx.store.getState().bufferActions.openNewTab()
    expect(second).toBe(first)
    expect(ctx.store.getState().buffers.filter((b) => b.type === 'newTab')).toHaveLength(1)
  })

  it('re-activates the existing New Tab within its own pane, never through global-focus actions', () => {
    const id = ctx.store.getState().bufferActions.openNewTab()
    ctx.paneActions.addBufferToPane.mockClear()
    ctx.store.getState().bufferActions.openNewTab()
    // Re-asserted within the pane via the same mechanism `openContent` already
    // uses to re-surface a dedup'd buffer (addBufferToPane's own `setActive`) —
    // not via setActivePane/activatePaneBuffer, which both also move GLOBAL focus.
    expect(ctx.paneActions.addBufferToPane).toHaveBeenCalledWith('root', id, true)
    expect(ctx.paneActions.setActivePane).not.toHaveBeenCalled()
    expect(ctx.paneActions.activatePaneBuffer).not.toHaveBeenCalled()
  })

  it('gives a DIFFERENT, already-registered pane its own New Tab, without stealing global focus', () => {
    ctx.panes['split-1'] = []
    const rootId = ctx.store.getState().bufferActions.openNewTab('root')
    const splitId = ctx.store.getState().bufferActions.openNewTab('split-1')
    expect(splitId).not.toBe(rootId)
    expect(ctx.store.getState().buffers.filter((b) => b.type === 'newTab')).toHaveLength(2)
    // Finding 2: opening a New Tab for a BACKGROUND pane must never change which
    // pane is globally active — that would steal focus out from under the user.
    expect(ctx.store.getState().activePaneId).toBe('root')
    expect(ctx.paneActions.setActivePane).not.toHaveBeenCalled()
    expect(ctx.paneActions.activatePaneBuffer).not.toHaveBeenCalled()
  })

  it('does not mint an orphaned buffer for an unregistered pane id', () => {
    const before = ctx.store.getState().buffers.length
    const result = ctx.store.getState().bufferActions.openNewTab('does-not-exist')
    expect(result).toBeNull()
    expect(ctx.store.getState().buffers).toHaveLength(before)
  })
})

describe('buffer-slice — New Tab is consumed', () => {
  let ctx: ReturnType<typeof makeStore>
  beforeEach(() => {
    ctx = makeStore()
  })

  it('opening a file in the same pane replaces the New Tab', () => {
    const newTabId = ctx.store.getState().bufferActions.openNewTab('root')
    ctx.store.getState().bufferActions.openContent({
      type: 'editor',
      path: '/a.ts',
      name: 'a.ts',
      content: '',
    })
    expect(ctx.store.getState().buffers.find((b) => b.id === newTabId)).toBeUndefined()
    expect(ctx.store.getState().buffers.filter((b) => b.type === 'newTab')).toHaveLength(0)
    expect(ctx.panes.root).not.toContain(newTabId)
  })

  it('opening in a DIFFERENT pane leaves the New Tab alone', () => {
    ctx.panes['split-1'] = []
    const newTabId = ctx.store.getState().bufferActions.openNewTab('split-1')
    ctx.store.getState().bufferActions.openContent({
      type: 'editor',
      path: '/a.ts',
      name: 'a.ts',
      content: '',
    })
    expect(ctx.store.getState().buffers.find((b) => b.id === newTabId)).toBeDefined()
  })

  it('openNewTab itself does not consume the New Tab it just found', () => {
    const first = ctx.store.getState().bufferActions.openNewTab('root')
    expect(ctx.store.getState().bufferActions.openNewTab('root')).toBe(first)
  })

  it('opening content already open in ANOTHER pane consumes the New Tab in the pane it lands in', () => {
    ctx.panes['split-1'] = []
    ctx.store.setState({ activePaneId: 'split-1' })
    const existingId = ctx.store.getState().bufferActions.openContent({
      type: 'editor',
      path: '/a.ts',
      name: 'a.ts',
      content: '',
    })

    ctx.store.setState({ activePaneId: 'root' })
    const newTabId = ctx.store.getState().bufferActions.openNewTab('root')

    const result = ctx.store.getState().bufferActions.openContent({
      type: 'editor',
      path: '/a.ts',
      name: 'a.ts',
      content: '',
    })

    // Dedup returns the SAME buffer id — no second copy is minted.
    expect(result).toBe(existingId)
    // The New Tab that was sitting in the pane the buffer lands in (root) is
    // consumed exactly like the bottom-of-openContent brand-new-buffer path.
    expect(ctx.store.getState().buffers.find((b) => b.id === newTabId)).toBeUndefined()
    expect(ctx.panes.root).not.toContain(newTabId)
    expect(ctx.panes.root).toContain(existingId)
  })

  it("jumping to an existing agentChat/terminal buffer's own pane does NOT consume the New Tab left behind", () => {
    ctx.panes['split-1'] = []
    ctx.store.setState({ activePaneId: 'split-1' })
    const chatId = ctx.store.getState().bufferActions.openContent({
      type: 'agentChat',
      chatId: 'chat-1',
      wsId: 'ws-test',
      name: 'Chat 1',
    })

    ctx.store.setState({ activePaneId: 'root' })
    const newTabId = ctx.store.getState().bufferActions.openNewTab('root')

    const result = ctx.store.getState().bufferActions.openContent({
      type: 'agentChat',
      chatId: 'chat-1',
      wsId: 'ws-test',
      name: 'Chat 1',
    })

    // This is the JUMP path: it reveals the pane that already holds the chat —
    // nothing new lands in root, so root's New Tab must survive untouched.
    expect(result).toBe(chatId)
    expect(ctx.paneActions.setActivePane).toHaveBeenCalledWith('split-1')
    expect(ctx.paneActions.activatePaneBuffer).toHaveBeenCalledWith('split-1', chatId)
    expect(ctx.store.getState().buffers.find((b) => b.id === newTabId)).toBeDefined()
    expect(ctx.panes.root).toContain(newTabId)
  })
})
