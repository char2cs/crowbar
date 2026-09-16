import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/features/agent/api/agent-api', () => ({
  stopChat: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  resolveWorkspaceIdForChat: vi.fn(),
  resolveChatOwnerWorkspaceId: vi.fn(),
  getActiveWorkspaceId: vi.fn(() => null),
}))

import { releaseClosedChat } from '@/features/panes/lib/release-closed-chat'
import { stopChat } from '@/features/agent/api/agent-api'
import {
  getActiveWorkspaceId,
  resolveChatOwnerWorkspaceId,
  resolveWorkspaceIdForChat,
} from '@/features/workspace/stores/workspace-store-registry'
import { subscribeWorkspaceEviction } from '@/features/workspace/lib/workspace-eviction-request'
import type { PaneGroup } from '@/features/panes/types/pane'

const stop = vi.mocked(stopChat)
/** Where the chat was FOUND — a registry key with a live store, which is what
 *  the stop URL is built against. Repo-scoped chat lists mean this is NOT the
 *  chat's owning workspace. */
const resolveWs = vi.mocked(resolveWorkspaceIdForChat)
/** The workspace the chat BELONGS to — the only id that can answer whether a
 *  workspace is still in use. */
const ownerWs = vi.mocked(resolveChatOwnerWorkspaceId)
const activeWs = vi.mocked(getActiveWorkspaceId)

function pane(id: string, chatId: string | null): PaneGroup {
  return {
    id,
    type: 'group',
    chatId,
    runnerId: null,
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

/** Register a chat: `found` is the registry key a lookup lands on (repo-wide
 *  chat lists mean this is shared across the repo), `owner` the workspace it
 *  really belongs to. */
const chats = new Map<string, { found: string | null; owner: string | null }>()

function register(chatId: string, found: string | null, owner: string | null = found) {
  chats.set(chatId, { found, owner })
}

beforeEach(() => {
  vi.clearAllMocks()
  chats.clear()
  stop.mockResolvedValue(undefined)
  activeWs.mockReturnValue(null)
  resolveWs.mockImplementation((id) => chats.get(id)?.found ?? null)
  ownerWs.mockImplementation((id) => chats.get(id)?.owner ?? null)
  evicted = []
  unsubscribe = subscribeWorkspaceEviction((wsId) => evicted.push(wsId))
})

afterEach(() => unsubscribe())

describe('releaseClosedChat — the backend half', () => {
  it("stops the chat's vendor CLI, against the workspace whose store holds it", async () => {
    register('chat-1', 'ws-1')

    await releaseClosedChat('chat-1', panesReader([]).read)

    expect(stop).toHaveBeenCalledExactlyOnceWith('ws-1', 'chat-1')
  })

  // A closed VIEW is not a closed CHAT: the same chat can still be up in
  // another pane, and ending one view of it must not stop it.
  it('stops nothing while the chat is still up in another pane', async () => {
    register('chat-1', 'ws-1')

    await releaseClosedChat('chat-1', panesReader([pane('p2', 'chat-1')]).read)

    expect(stop).not.toHaveBeenCalled()
    expect(evicted).toEqual([])
  })

  it('does nothing for a chat no registered store knows', async () => {
    await releaseClosedChat('chat-1', panesReader([]).read)

    expect(stop).not.toHaveBeenCalled()
    expect(evicted).toEqual([])
  })
})

describe('releaseClosedChat — the frontend half', () => {
  it('asks for the OWNING workspace to be evicted once nothing of it is on screen', async () => {
    register('chat-1', 'ws-store', 'ws-owner')

    await releaseClosedChat('chat-1', panesReader([]).read)

    // The stop went to the store that had it; the eviction goes to the owner.
    expect(stop).toHaveBeenCalledExactlyOnceWith('ws-store', 'chat-1')
    expect(evicted).toEqual(['ws-owner'])
  })

  // A workspace owns many chats. Closing one view of it says nothing about
  // the others.
  it('keeps the workspace while another of ITS OWN chats still has a view', async () => {
    register('chat-1', 'ws-1')
    register('chat-2', 'ws-1')

    await releaseClosedChat('chat-1', panesReader([pane('p2', 'chat-2')]).read)

    expect(stop).toHaveBeenCalledExactlyOnceWith('ws-1', 'chat-1')
    expect(evicted).toEqual([])
  })

  /**
   * The bug this resolver split exists for, measured live: `listChats` is
   * REPO-scoped, so every workspace store in a repo is seeded with the whole
   * repo's chats and a registry-key lookup matches any of them. Asking "is
   * this workspace still in use" that way answered yes for panes holding a
   * completely different workspace's chats, and nothing was ever torn down.
   */
  it('is not fooled by a sibling workspace’s chat sharing the same repo store', async () => {
    register('chat-1', 'ws-store', 'ws-owner')
    // Found in the SAME store (repo-wide list), but owned by another workspace.
    register('chat-2', 'ws-store', 'ws-other')

    await releaseClosedChat('chat-1', panesReader([pane('p2', 'chat-2')]).read)

    expect(evicted).toEqual(['ws-owner'])
  })

  it("leaves another workspace's still-open chat out of it", async () => {
    register('chat-1', 'ws-1')
    register('chat-2', 'ws-2')

    await releaseClosedChat('chat-1', panesReader([pane('p2', 'chat-2')]).read)

    expect(evicted).toEqual(['ws-1'])
  })

  // The ACTIVE workspace is the route: `WorkspaceView` is mounted over its
  // store and would re-create it the instant it went away.
  it('never evicts the workspace the route is currently on', async () => {
    register('chat-1', 'ws-1')
    activeWs.mockReturnValue('ws-1')

    await releaseClosedChat('chat-1', panesReader([]).read)

    expect(stop).toHaveBeenCalledExactlyOnceWith('ws-1', 'chat-1')
    expect(evicted).toEqual([])
  })

  // The stop still has to happen: the CLI belongs to the daemon, not to
  // whether this client happens to know who owns the chat.
  it('still stops the chat when the owner cannot be resolved', async () => {
    register('chat-1', 'ws-1', null)

    await releaseClosedChat('chat-1', panesReader([]).read)

    expect(stop).toHaveBeenCalledExactlyOnceWith('ws-1', 'chat-1')
    expect(evicted).toEqual([])
  })
})

describe('releaseClosedChat — sequencing (stop first, then tear down)', () => {
  it('stops the chat BEFORE anything asks for the store to go', async () => {
    register('chat-1', 'ws-1')
    const order: string[] = []
    stop.mockImplementation(async () => {
      order.push('stop')
    })
    unsubscribe()
    unsubscribe = subscribeWorkspaceEviction(() => order.push('evict'))

    await releaseClosedChat('chat-1', panesReader([]).read)

    expect(order).toEqual(['stop', 'evict'])
  })

  // A close is undoable — clicking the row again while the stop is in flight
  // puts the chat straight back on screen, and the store it lives on must
  // survive that.
  it('abandons the teardown when the chat is reopened while the stop is in flight', async () => {
    register('chat-1', 'ws-1')
    const panes = panesReader([])
    stop.mockImplementation(async () => {
      panes.set([pane('p9', 'chat-1')])
    })

    await releaseClosedChat('chat-1', panes.read)

    expect(stop).toHaveBeenCalledExactlyOnceWith('ws-1', 'chat-1')
    expect(evicted).toEqual([])
  })

  // A stop that FAILED means the CLI is still running and still writing into
  // this workspace's store. Evicting it would orphan a live turn.
  it('does not tear the store down when the stop itself failed', async () => {
    register('chat-1', 'ws-1')
    stop.mockRejectedValue(new Error('daemon unreachable'))

    await releaseClosedChat('chat-1', panesReader([]).read)

    expect(evicted).toEqual([])
  })

  it('never rejects, so a close is never the thing that throws', async () => {
    register('chat-1', 'ws-1')
    stop.mockRejectedValue(new Error('daemon unreachable'))

    await expect(releaseClosedChat('chat-1', panesReader([]).read)).resolves.toBeUndefined()
  })
})
