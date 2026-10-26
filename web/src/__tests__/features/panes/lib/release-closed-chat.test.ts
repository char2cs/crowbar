import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/features/agent/api/agent-api', () => ({
  killChat: vi.fn().mockResolvedValue(undefined),
  stopChat: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getActiveWorkspaceId: vi.fn(() => null),
}))

import { releaseClosedChat } from '@/features/panes/lib/release-closed-chat'
import { killChat, stopChat } from '@/features/agent/api/agent-api'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { subscribeWorkspaceEviction } from '@/features/workspace/lib/workspace-eviction-request'
import type { PaneGroup } from '@/features/panes/types/pane'

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

const panesOf = (panes: PaneGroup[]) => () => Object.fromEntries(panes.map((p) => [p.id, p]))

let evicted: string[]
let unsubscribe: () => void

beforeEach(() => {
  vi.clearAllMocks()
  activeWs.mockReturnValue(null)
  evicted = []
  unsubscribe = subscribeWorkspaceEviction((wsId) => evicted.push(wsId))
})

afterEach(() => unsubscribe())

describe('releaseClosedChat', () => {
  // A closed view only detaches: the agent keeps running for whoever else is
  // attached, and only an explicit kill ends it.
  it('never ends the chat’s agent', () => {
    releaseClosedChat('chat-1', 'ws-1', panesOf([]))

    expect(killChat).not.toHaveBeenCalled()
    expect(stopChat).not.toHaveBeenCalled()
  })

  it('asks for the workspace to be evicted once nothing of it is on screen', () => {
    releaseClosedChat('chat-1', 'ws-1', panesOf([]))

    expect(evicted).toEqual(['ws-1'])
  })

  // A closed VIEW is not a closed CHAT.
  it('keeps everything while the chat is still up in another pane', () => {
    releaseClosedChat('chat-1', 'ws-1', panesOf([pane('p2', 'chat-1', 'ws-1')]))

    expect(evicted).toEqual([])
  })

  it('does nothing for a member with no recorded workspace', () => {
    releaseClosedChat('chat-1', null, panesOf([]))

    expect(evicted).toEqual([])
  })

  // A workspace owns many chats. Closing one view of it says nothing about
  // the others.
  it('keeps the workspace while another of ITS OWN chats still has a view', () => {
    releaseClosedChat('chat-1', 'ws-1', panesOf([pane('p2', 'chat-2', 'ws-1')]))

    expect(evicted).toEqual([])
  })

  // Each member records its own workspace, so a sibling workspace's chat from
  // the same repo can never be mistaken for this one's.
  it("leaves another workspace's still-open chat out of it", () => {
    releaseClosedChat('chat-1', 'ws-1', panesOf([pane('p2', 'chat-2', 'ws-2')]))

    expect(evicted).toEqual(['ws-1'])
  })

  it('never evicts the workspace the route is currently on', () => {
    activeWs.mockReturnValue('ws-1')

    releaseClosedChat('chat-1', 'ws-1', panesOf([]))

    expect(evicted).toEqual([])
  })
})
