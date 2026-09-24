import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/features/agent/api/agent-api', () => ({
  stopChat: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getActiveWorkspaceId: vi.fn(() => null),
}))

import { releaseClosedChat } from '@/features/panes/lib/release-closed-chat'
import { stopChat } from '@/features/agent/api/agent-api'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { subscribeWorkspaceEviction } from '@/features/workspace/lib/workspace-eviction-request'
import type { PaneGroup } from '@/features/panes/types/pane'

const stop = vi.mocked(stopChat)
const activeWs = vi.mocked(getActiveWorkspaceId)

/** A view member: its chat and the workspace recorded when it was opened. */
function pane(id: string, chatId: string | null, workspaceId: string | null = null): PaneGroup {
  return {
    id,
    type: 'group',
    chatId,
    runnerId: null,
    workspaceId,
    editorTabIds: [],
    activeEditorTabId: null,
    editorOpen: false,
    viewId: id,
  }
}

/** A live `readPanes` reader over a mutable list, so a test can change what
 *  is on screen BETWEEN the stop and the teardown — the race the sequencing
 *  exists to survive. */
function panesReader(initial: PaneGroup[]) {
  const state = { panes: initial }
  return {
    read: () => Object.fromEntries(state.panes.map((p) => [p.id, p])),
    set: (next: PaneGroup[]) => {
      state.panes = next
    },
  }
}

let evicted: string[]
let unsubscribe: () => void

beforeEach(() => {
  vi.clearAllMocks()
  stop.mockResolvedValue(undefined)
  activeWs.mockReturnValue(null)
  evicted = []
  unsubscribe = subscribeWorkspaceEviction((wsId) => evicted.push(wsId))
})

afterEach(() => unsubscribe())

describe('releaseClosedChat — the backend half', () => {
  it("stops the chat's vendor CLI against the workspace its member recorded", async () => {
    await releaseClosedChat('chat-1', 'ws-1', panesReader([]).read)

    expect(stop).toHaveBeenCalledExactlyOnceWith('ws-1', 'chat-1')
  })

  // A closed VIEW is not a closed CHAT.
  it('stops nothing while the chat is still up in another pane', async () => {
    await releaseClosedChat('chat-1', 'ws-1', panesReader([pane('p2', 'chat-1', 'ws-1')]).read)

    expect(stop).not.toHaveBeenCalled()
    expect(evicted).toEqual([])
  })

  it('does nothing for a member with no recorded workspace', async () => {
    await releaseClosedChat('chat-1', null, panesReader([]).read)

    expect(stop).not.toHaveBeenCalled()
    expect(evicted).toEqual([])
  })
})

describe('releaseClosedChat — the frontend half', () => {
  it('asks for the workspace to be evicted once nothing of it is on screen', async () => {
    await releaseClosedChat('chat-1', 'ws-1', panesReader([]).read)

    expect(evicted).toEqual(['ws-1'])
  })

  // A workspace owns many chats. Closing one view of it says nothing about
  // the others.
  it('keeps the workspace while another of ITS OWN chats still has a view', async () => {
    await releaseClosedChat('chat-1', 'ws-1', panesReader([pane('p2', 'chat-2', 'ws-1')]).read)

    expect(stop).toHaveBeenCalledExactlyOnceWith('ws-1', 'chat-1')
    expect(evicted).toEqual([])
  })

  // Each member records its own workspace, so a sibling workspace's chat from
  // the same repo can never be mistaken for this one's.
  it("leaves another workspace's still-open chat out of it", async () => {
    await releaseClosedChat('chat-1', 'ws-1', panesReader([pane('p2', 'chat-2', 'ws-2')]).read)

    expect(evicted).toEqual(['ws-1'])
  })

  it('never evicts the workspace the route is currently on', async () => {
    activeWs.mockReturnValue('ws-1')

    await releaseClosedChat('chat-1', 'ws-1', panesReader([]).read)

    expect(stop).toHaveBeenCalledExactlyOnceWith('ws-1', 'chat-1')
    expect(evicted).toEqual([])
  })
})

describe('releaseClosedChat — sequencing (stop first, then tear down)', () => {
  it('stops the chat BEFORE anything asks for the store to go', async () => {
    const order: string[] = []
    stop.mockImplementation(async () => {
      order.push('stop')
    })
    unsubscribe()
    unsubscribe = subscribeWorkspaceEviction(() => order.push('evict'))

    await releaseClosedChat('chat-1', 'ws-1', panesReader([]).read)

    expect(order).toEqual(['stop', 'evict'])
  })

  it('abandons the teardown when the chat is reopened while the stop is in flight', async () => {
    const panes = panesReader([])
    stop.mockImplementation(async () => {
      panes.set([pane('p9', 'chat-1', 'ws-1')])
    })

    await releaseClosedChat('chat-1', 'ws-1', panes.read)

    expect(stop).toHaveBeenCalledExactlyOnceWith('ws-1', 'chat-1')
    expect(evicted).toEqual([])
  })

  it('does not tear the store down when the stop itself failed', async () => {
    stop.mockRejectedValue(new Error('daemon unreachable'))

    await releaseClosedChat('chat-1', 'ws-1', panesReader([]).read)

    expect(evicted).toEqual([])
  })

  it('never rejects, so a close is never the thing that throws', async () => {
    stop.mockRejectedValue(new Error('daemon unreachable'))

    await expect(releaseClosedChat('chat-1', 'ws-1', panesReader([]).read)).resolves.toBeUndefined()
  })
})
