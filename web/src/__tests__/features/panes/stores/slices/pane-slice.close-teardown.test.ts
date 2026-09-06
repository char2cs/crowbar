import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// Isolated from `pane-slice.test.ts` because this file has to mock the daemon
// call at module scope: `closePane` firing `stopChat` is the whole subject
// here, and that suite exercises the real (unmocked) module graph.
vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/editor/stores/buffer-session-persistence', () => ({
  saveSessionToStore: vi.fn(),
  clearQueuedWorkspaceSessionSave: vi.fn(),
}))
vi.mock('@/features/agent/api/agent-api', () => ({
  stopChat: vi.fn().mockResolvedValue(undefined),
}))

import { stopChat, type AgentChat } from '@/features/agent/api/agent-api'
import {
  destroyWorkspaceStore,
  getAllActiveWorkspaceIds,
  getOrCreateWorkspaceStore,
  setActiveWorkspaceId,
} from '@/features/workspace/stores/workspace-store-registry'
import { subscribeWorkspaceEviction } from '@/features/workspace/lib/workspace-eviction-request'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'

const stop = vi.mocked(stopChat)

function chat(id: string, wsId: string): AgentChat {
  return {
    id,
    workspaceId: wsId,
    title: id,
    liveRunnerId: '',
    terminalSessionId: '',
    activeProviderId: 'claude',
    createdAt: '2026-01-01T00:00:00Z',
    order: 0,
    parentId: '',
  }
}

/** Registry state a chat has to be in for `resolveWorkspaceIdForChat` to
 *  find it: a real, registered store whose `agentChats.chats` names it. */
function seedWorkspace(wsId: string, chats: AgentChat[], working: Record<string, boolean> = {}) {
  const store = getOrCreateWorkspaceStore(wsId)
  store.getState().seedAgentChats(chats)
  for (const [chatId, isWorking] of Object.entries(working)) {
    store.getState().setAgentChatWorking(chatId, isWorking)
  }
  return store
}

/** Let the fire-and-forget release (and its awaited `stopChat`) settle.
 *  A real signal, not a timer: `stopChat`'s own resolution is the thing
 *  every assertion here is waiting on. */
async function settle() {
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
}

let evicted: string[]
let unsubscribe: () => void

beforeEach(() => {
  vi.clearAllMocks()
  stop.mockResolvedValue(undefined)
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

/**
 * "All of Crowbar's chats should die once the user has closed their view...
 * Both. It's like killing a chat tab: removes both out of memory."
 *
 * `stopChat`'s own doc has always said it "is what closing a chat TAB calls"
 * — and it had exactly one caller, the chat view's stop BUTTON. `closePane`
 * only ever did Recents bookkeeping, so a closed view left its vendor CLI
 * running in the daemon and its workspace resident in the browser. That is
 * the gap these cover.
 */
describe('closePane tears the closed chat down on both sides', () => {
  it("stops the closed chat's vendor CLI", async () => {
    seedWorkspace('ws-1', [chat('chat-1', 'ws-1')])
    windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')

    windowPaneStore.getState().paneActions.closePane(ROOT_PANE_ID)
    await settle()

    expect(stop).toHaveBeenCalledExactlyOnceWith('ws-1', 'chat-1')
  })

  it('drops the workspace store outside the keep-alive window entirely', async () => {
    seedWorkspace('ws-1', [chat('chat-1', 'ws-1')])
    windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')

    windowPaneStore.getState().paneActions.closePane(ROOT_PANE_ID)
    await settle()

    expect(evicted).toEqual(['ws-1'])
  })

  // NOT `deleteChat`. The chat entry and its conversation are kept, so the
  // row stays in the tree and reopening it resumes the real conversation.
  it('closing a CHATLESS pane stops nothing', async () => {
    const other = windowPaneStore.getState().paneActions.addPane()!

    windowPaneStore.getState().paneActions.closePane(other)
    await settle()

    expect(stop).not.toHaveBeenCalled()
    expect(evicted).toEqual([])
  })

  it('removing ONE pane from a merged view stops only that pane’s chat', async () => {
    seedWorkspace('ws-1', [chat('chat-1', 'ws-1'), chat('chat-2', 'ws-1')])
    const { paneActions } = windowPaneStore.getState()
    paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    const merged = paneActions.splitPane(ROOT_PANE_ID, 'horizontal')!
    paneActions.setPaneChat(merged, 'chat-2', 'runner-2')

    windowPaneStore.getState().paneActions.closePane(merged)
    await settle()

    expect(stop).toHaveBeenCalledExactlyOnceWith('ws-1', 'chat-2')
    // chat-1 is still up in the surviving half of the view, so the workspace
    // is still in use — nothing to evict.
    expect(evicted).toEqual([])
  })

  it('keeps the workspace the route is currently on, whose view is still mounted', async () => {
    seedWorkspace('ws-1', [chat('chat-1', 'ws-1')])
    setActiveWorkspaceId('ws-1')
    windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')

    windowPaneStore.getState().paneActions.closePane(ROOT_PANE_ID)
    await settle()

    expect(stop).toHaveBeenCalledExactlyOnceWith('ws-1', 'chat-1')
    expect(evicted).toEqual([])
  })

  // The sequencing rule: `stopChat` is what actually stops a chat mid-turn,
  // and the store being torn down is the same store that chat's stream and
  // working map live on. Nothing may be dropped before the stop has settled.
  it('stops a WORKING chat before anything asks for its store to go', async () => {
    seedWorkspace('ws-1', [chat('chat-1', 'ws-1')], { 'chat-1': true })
    windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    const order: string[] = []
    stop.mockImplementation(async () => {
      order.push('stop')
    })
    unsubscribe()
    unsubscribe = subscribeWorkspaceEviction(() => order.push('evict'))

    windowPaneStore.getState().paneActions.closePane(ROOT_PANE_ID)
    // The stop is dispatched on the synchronous path; the teardown is NOT —
    // it is behind the await, which is the whole sequencing guarantee.
    expect(order).toEqual(['stop'])
    await settle()

    expect(order).toEqual(['stop', 'evict'])
  })

  it('leaves a working chat’s store alone when the stop never landed', async () => {
    seedWorkspace('ws-1', [chat('chat-1', 'ws-1')], { 'chat-1': true })
    stop.mockRejectedValue(new Error('daemon unreachable'))
    windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')

    windowPaneStore.getState().paneActions.closePane(ROOT_PANE_ID)
    await settle()

    expect(evicted).toEqual([])
  })
})
