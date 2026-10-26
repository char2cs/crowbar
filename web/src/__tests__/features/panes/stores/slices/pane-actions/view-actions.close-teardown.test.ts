import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// Isolated from `pane-slice.test.ts` because this file has to mock the daemon
// calls at module scope: that no close reaches the daemon is the whole subject
// here, and that suite exercises the real (unmocked) module graph.
vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/agent/api/agent-api', () => ({
  killChat: vi.fn().mockResolvedValue(undefined),
  stopChat: vi.fn().mockResolvedValue(undefined),
}))

import { killChat, stopChat } from '@/features/agent/api/agent-api'
import {
  destroyWorkspaceStore,
  getAllActiveWorkspaceIds,
  setActiveWorkspaceId,
} from '@/features/workspace/stores/workspace-store-registry'
import { subscribeWorkspaceEviction } from '@/features/workspace/lib/workspace-eviction-request'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'

let evicted: string[]
let unsubscribe: () => void

beforeEach(() => {
  vi.clearAllMocks()
  resetWindowPaneStoreForTests()
  evicted = []
  unsubscribe = subscribeWorkspaceEviction((wsId) => evicted.push(wsId))
})

afterEach(() => {
  unsubscribe()
  getAllActiveWorkspaceIds().forEach((id) => destroyWorkspaceStore(id))
  resetWindowPaneStoreForTests()
  setActiveWorkspaceId('')
})

function expectNoAgentWasEnded() {
  expect(killChat).not.toHaveBeenCalled()
  expect(stopChat).not.toHaveBeenCalled()
}

/**
 * A chat can be open on several clients at once, so closing a view only
 * detaches this window: the agent keeps running and only an explicit Kill ends
 * it. What a close still releases is the browser's own copy of the workspace.
 */
describe('closing a chat view detaches without ending the agent', () => {
  it('closePane drops the workspace store outside the keep-alive window and kills nothing', () => {
    windowPaneStore
      .getState()
      .paneActions.openChat('chat-1', { runnerId: 'runner-1', workspaceId: 'ws-1' })

    windowPaneStore.getState().paneActions.closePane(ROOT_PANE_ID)

    expectNoAgentWasEnded()
    expect(evicted).toEqual(['ws-1'])
  })

  it('closing a CHATLESS pane releases nothing', () => {
    const other = windowPaneStore.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')!

    windowPaneStore.getState().paneActions.closePane(other)

    expectNoAgentWasEnded()
    expect(evicted).toEqual([])
  })

  it('removing ONE pane from a merged view keeps the workspace its sibling still uses', () => {
    const { paneActions } = windowPaneStore.getState()
    paneActions.openChat('chat-1', { runnerId: 'runner-1', workspaceId: 'ws-1' })
    paneActions.dropChatOnPane('chat-2', ROOT_PANE_ID, 'right', 'ws-1')
    const merged = windowPaneStore
      .getState()
      .paneActions.getAllPaneGroups()
      .find((p) => p.chatId === 'chat-2')!.id

    windowPaneStore.getState().paneActions.closePane(merged)

    expectNoAgentWasEnded()
    expect(evicted).toEqual([])
  })

  it('closeView on a merged view spanning TWO workspaces releases both', () => {
    const { paneActions } = windowPaneStore.getState()
    paneActions.openChat('chat-1', { runnerId: 'runner-1', workspaceId: 'ws-1' })
    paneActions.dropChatOnPane('chat-2', ROOT_PANE_ID, 'right', 'ws-2')
    const view = windowPaneStore.getState().panes[ROOT_PANE_ID].viewId!

    windowPaneStore.getState().paneActions.closeView(view)

    expectNoAgentWasEnded()
    expect([...evicted].sort()).toEqual(['ws-1', 'ws-2'])
  })

  it('keeps the workspace the route is currently on, whose view is still mounted', () => {
    setActiveWorkspaceId('ws-1')
    windowPaneStore
      .getState()
      .paneActions.openChat('chat-1', { runnerId: 'runner-1', workspaceId: 'ws-1' })

    windowPaneStore.getState().paneActions.closePane(ROOT_PANE_ID)

    expectNoAgentWasEnded()
    expect(evicted).toEqual([])
  })
})
