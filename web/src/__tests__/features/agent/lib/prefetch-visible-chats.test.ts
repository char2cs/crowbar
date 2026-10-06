import { beforeEach, describe, expect, it, vi } from 'vitest'
import { chatLedgerStore } from '@/features/agent/stores/chat-ledger-store'
import { prefetchVisibleChats } from '@/features/agent/lib/prefetch-visible-chats'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { __resetWorkspaceScopesForTest, recordWorkspaceScope } from '@/lib/workspace-scope'

const { listChatMessagesFn } = vi.hoisted(() => ({ listChatMessagesFn: vi.fn() }))
vi.mock('@/features/agent/api/agent-api', () => ({ listChatMessages: listChatMessagesFn }))
vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/panes/lib/release-closed-chat', () => ({
  releaseClosedChat: vi.fn(async () => {}),
}))

const emptyPage = { cursor: 0, oldestCursor: 0, hasMore: false, items: [] }
const actions = () => windowPaneStore.getState().paneActions

describe('prefetchVisibleChats', () => {
  beforeEach(() => {
    listChatMessagesFn.mockReset()
    listChatMessagesFn.mockResolvedValue(emptyPage)
    chatLedgerStore.setState({ ledgers: {} })
    __resetWorkspaceScopesForTest()
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-a' })
    resetWindowPaneStoreForTests()
    actions().setActiveProject('p1')
  })

  it('requests the first page of the chat on screen, under the workspace scope the sidebar recorded', async () => {
    actions().openChat('c1', { workspaceId: 'ws-a' })

    await prefetchVisibleChats()

    expect(listChatMessagesFn).toHaveBeenCalledWith('ws-a', 'c1', { limit: 100 })
    expect(chatLedgerStore.getState().ledgers.c1.status).toBe('loaded')
  })

  it('does not request a chat parked in another view', async () => {
    actions().openChat('shown', { workspaceId: 'ws-a' })
    actions().openChat('parked', { workspaceId: 'ws-a' })
    actions().activateView(windowPaneStore.getState().viewOrder[0])

    await prefetchVisibleChats()

    expect(listChatMessagesFn).toHaveBeenCalledTimes(1)
    expect(listChatMessagesFn.mock.calls[0][1]).toBe('shown')
  })

  it('requests nothing when no scope is recorded for the chat workspace', async () => {
    actions().openChat('c1', { workspaceId: 'ws-elsewhere' })

    await prefetchVisibleChats()

    expect(listChatMessagesFn).not.toHaveBeenCalled()
  })

  it('requests nothing when the layout has no chat', async () => {
    await prefetchVisibleChats()

    expect(listChatMessagesFn).not.toHaveBeenCalled()
  })

  it('drops the ledger of a failed prefetch so the view retries from scratch', async () => {
    listChatMessagesFn.mockRejectedValue(new Error('down'))
    actions().openChat('c1', { workspaceId: 'ws-a' })

    await prefetchVisibleChats()

    expect(chatLedgerStore.getState().ledgers.c1).toBeUndefined()
  })
})
