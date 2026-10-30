import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { IDBFactory } from 'fake-indexeddb'
import { chatLedgerStore } from '@/features/agent/stores/chat-ledger-store'
import { prefetchVisibleChats } from '@/features/agent/lib/prefetch-visible-chats'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { saveChatHistory } from '@/lib/persistence/chat-history'
import { resetDB } from '@/lib/persistence/idb'
import { __resetWorkspaceScopesForTest, recordWorkspaceScope } from '@/lib/workspace-scope'
import {
  loadProjects,
  markHostsRead,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

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
    globalThis.indexedDB = new IDBFactory()
    resetDB()
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

  it('keeps the ledger of a failed prefetch when it holds a cached transcript', async () => {
    await saveChatHistory('c1', {
      messages: [
        { turnId: 't1', sequence: 1, role: 'user', providerId: '', text: 'hi', at: '2026-08-24' },
      ],
      hasOlder: false,
    })
    listChatMessagesFn.mockRejectedValue(new Error('down'))
    actions().openChat('c1', { workspaceId: 'ws-a' })

    await prefetchVisibleChats()

    expect(chatLedgerStore.getState().ledgers.c1.messages).toHaveLength(1)
  })
})

// Boot prefetches before the project list loads. On a paired install that list
// is what says whose daemon a chat is on, so a request sent first would fail,
// be evicted, and leave the view to repeat it.
describe('prefetchVisibleChats on an install that has paired a remote host', () => {
  beforeEach(() => {
    listChatMessagesFn.mockReset()
    listChatMessagesFn.mockResolvedValue(emptyPage)
    chatLedgerStore.setState({ ledgers: {} })
    __resetWorkspaceScopesForTest()
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-a' })
    resetWindowPaneStoreForTests()
    actions().setActiveProject('p1')
    resetHostWorld()
    pairRemoteHost('host-1')
    actions().openChat('c1', { workspaceId: 'ws-a' })
  })
  afterEach(() => resetHostWorld())

  it('asks nothing until the project list places the workspace, then loads the first page', async () => {
    const prefetch = prefetchVisibleChats()
    await Promise.resolve()
    expect(listChatMessagesFn).not.toHaveBeenCalled()
    expect(chatLedgerStore.getState().ledgers.c1).toBeUndefined()

    loadProjects(projectOn('p1', 'local'))
    await prefetch

    expect(listChatMessagesFn).toHaveBeenCalledExactlyOnceWith('ws-a', 'c1', { limit: 100 })
    expect(chatLedgerStore.getState().ledgers.c1.status).toBe('loaded')
  })

  it('gives up on a workspace whose project the list and every paired host lack, asking nothing', async () => {
    const prefetch = prefetchVisibleChats()
    const settled = vi.fn()
    void prefetch.then(settled)

    loadProjects(projectOn('p-other', 'local'))
    for (let i = 0; i < 10; i++) await Promise.resolve()
    expect(settled).not.toHaveBeenCalled()
    markHostsRead('host-1')
    await prefetch

    expect(listChatMessagesFn).not.toHaveBeenCalled()
    expect(chatLedgerStore.getState().ledgers.c1).toBeUndefined()
  })
})
