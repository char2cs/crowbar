import { describe, it, expect, beforeEach, vi } from 'vitest'

vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
const terminalKill = vi.hoisted(() => vi.fn().mockResolvedValue(undefined))
vi.mock('@/lib/crowbar-bridge', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  terminalKill,
}))

import {
  createWindowPaneStore,
  type WindowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import {
  releaseUnreferencedBuffers,
  type BufferOwnershipState,
} from '@/features/panes/lib/buffer-release'
import type { PaneContent } from '@/features/panes/types/pane-content'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'

/** The PTYs the daemon was told to close, by tab session id. */
function killed(): string[] {
  return terminalKill.mock.calls.map(([connectionId]) =>
    String(connectionId).replace(/^conn-/, ''),
  )
}

/** Invariant C2, checked on every notification the store emits. */
function orphans(store: WindowPaneStore): string[] {
  const s = store.getState()
  const listed = new Set(Object.values(s.panes).flatMap((p) => p.editorTabIds))
  const known = new Set(s.buffers.map((b) => b.id))
  return [
    ...s.buffers.filter((b) => !listed.has(b.id)).map((b) => `orphan buffer ${b.id}`),
    ...[...listed].filter((id) => !known.has(id)).map((id) => `dangling tab ${id}`),
  ]
}

function watchInvariant(store: WindowPaneStore): string[] {
  const seen: string[] = []
  store.subscribe(() => seen.push(...orphans(store)))
  return seen
}

function openTerminal(store: WindowPaneStore, sessionId: string): string {
  useTerminalStore.setState((s) => ({
    sessions: new Map(s.sessions).set(sessionId, { connectionId: `conn-${sessionId}` }),
  }))
  return store.getState().bufferActions.openContent({ type: 'terminal', sessionId })
}

describe('buffer ownership (invariant C2)', () => {
  let store: WindowPaneStore
  beforeEach(() => {
    terminalKill.mockClear()
    store = createWindowPaneStore()
    store.getState().paneActions.setActiveProject('p1')
  })

  it('closing a view kills the terminal its pane held, in the same set', async () => {
    const { paneActions } = store.getState()
    paneActions.openChat('chat-a')
    openTerminal(store, 'pty-1')
    const viewId = store.getState().activeViewId!
    const violations = watchInvariant(store)

    paneActions.closeView(viewId)

    expect(store.getState().buffers).toEqual([])
    expect(violations).toEqual([])
    await vi.waitFor(() => expect(killed()).toEqual(['pty-1']))
  })

  it('closing the last pane of a view releases its editor tabs into the reopen history', () => {
    const { paneActions, bufferActions } = store.getState()
    paneActions.openChat('chat-a')
    bufferActions.openContent({ type: 'editor', path: 'a.ts', name: 'a.ts', content: '' })
    const paneId = store.getState().activePaneId

    paneActions.closePane(paneId)

    expect(store.getState().buffers).toEqual([])
    expect(store.getState().closedBuffersHistory[0]?.path).toBe('a.ts')
  })

  it('closing one member of a group hands its tabs to the survivor — nothing is killed', async () => {
    const { paneActions } = store.getState()
    paneActions.openChat('chat-a')
    const survivor = store.getState().activePaneId
    paneActions.dropChatOnPane('chat-b', survivor, 'right')
    const closing = store.getState().activePaneId
    const bufferId = openTerminal(store, 'pty-2')

    paneActions.closePane(closing)

    expect(store.getState().buffers.map((b) => b.id)).toEqual([bufferId])
    expect(store.getState().panes[survivor].editorTabIds).toContain(bufferId)
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(killed()).toEqual([])
  })

  it('a buffer shared by a split survives while any pane still lists it', () => {
    const { paneActions } = store.getState()
    paneActions.openChat('chat-a')
    const left = store.getState().activePaneId
    const bufferId = store
      .getState()
      .bufferActions.openContent({ type: 'editor', path: 'b.ts', name: 'b.ts', content: '' })
    const right = paneActions.splitPane(left, 'horizontal', bufferId)!

    paneActions.removeEditorTabFromPane(left, bufferId)
    expect(store.getState().buffers.map((b) => b.id)).toEqual([bufferId])

    paneActions.removeEditorTabFromPane(right, bufferId)
    expect(store.getState().buffers).toEqual([])
  })

  it('forgetting a chat and closing a project release their panes’ buffers', async () => {
    const { paneActions } = store.getState()
    paneActions.openChat('chat-a')
    openTerminal(store, 'pty-a')
    paneActions.openChat('chat-b')
    openTerminal(store, 'pty-b')
    const violations = watchInvariant(store)

    paneActions.forgetChat('chat-a')
    paneActions.closeViewsForProject('p1')

    expect(store.getState().buffers).toEqual([])
    expect(violations).toEqual([])
    await vi.waitFor(() => expect(killed().sort()).toEqual(['pty-a', 'pty-b']))
  })

  it('no sequence of pane gestures ever leaves an orphan or a dangling tab', () => {
    const violations = watchInvariant(store)
    let seed = 7
    const rand = (n: number) => {
      seed = (seed * 1103515245 + 12345) & 0x7fffffff
      return seed % n
    }
    for (let step = 0; step < 400; step++) {
      const s = store.getState()
      const paneIds = Object.keys(s.panes)
      const pane = paneIds[rand(paneIds.length)]
      const viewIds = s.viewOrder
      switch (rand(9)) {
        case 0:
          s.paneActions.openChat(`chat-${rand(6)}`)
          break
        case 1:
          s.paneActions.setActivePane(pane)
          s.bufferActions.openContent({ type: 'terminal', sessionId: `pty-${step}` })
          break
        case 2:
          s.paneActions.setActivePane(pane)
          s.bufferActions.openContent({
            type: 'editor',
            path: `f${rand(4)}.ts`,
            name: 'f',
            content: '',
          })
          break
        case 3:
          s.paneActions.closePane(pane)
          break
        case 4:
          if (viewIds.length) s.paneActions.closeView(viewIds[rand(viewIds.length)])
          break
        case 5:
          s.paneActions.dropChatOnPane(`chat-${rand(6)}`, pane, 'right')
          break
        case 6:
          s.paneActions.forgetChat(`chat-${rand(6)}`)
          break
        case 7: {
          const tab = s.panes[pane]?.editorTabIds[0]
          if (tab) s.paneActions.removeEditorTabFromPane(pane, tab)
          break
        }
        case 8:
          s.paneActions.splitPane(pane, 'vertical', s.panes[pane]?.activeEditorTabId ?? undefined)
          break
      }
    }
    expect(violations.slice(0, 5)).toEqual([])
    expect(orphans(store)).toEqual([])
  })
})

describe('releaseUnreferencedBuffers', () => {
  it('drops only buffers no pane lists and records file buffers for reopen', () => {
    const state: BufferOwnershipState = {
      panes: { a: { editorTabIds: ['keep'] } },
      buffers: [
        { id: 'keep', type: 'editor', path: 'k', name: 'k', workspaceId: 'w' },
        { id: 'gone', type: 'editor', path: 'g', name: 'g', workspaceId: 'w' },
      ] as unknown as PaneContent[],
      closedBuffersHistory: [],
    }
    const released = releaseUnreferencedBuffers(state)
    expect(released.map((b) => b.id)).toEqual(['gone'])
    expect(state.buffers.map((b) => b.id)).toEqual(['keep'])
    expect(state.closedBuffersHistory.map((b) => b.path)).toEqual(['g'])
  })
})
