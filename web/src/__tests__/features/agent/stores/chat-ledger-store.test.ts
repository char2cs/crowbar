import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgentChatMessage, AgentChatMessagesPage } from '@/features/agent/api/agent-api'
import { chatLedgerStore } from '@/features/agent/stores/chat-ledger-store'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'

const { listChatMessagesFn } = vi.hoisted(() => ({ listChatMessagesFn: vi.fn() }))
vi.mock('@/features/agent/api/agent-api', () => ({ listChatMessages: listChatMessagesFn }))
vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/panes/lib/release-closed-chat', () => ({
  releaseClosedChat: vi.fn(async () => {}),
}))

function message(sequence: number): AgentChatMessage {
  return {
    turnId: `t${sequence}`,
    sequence,
    role: 'user',
    providerId: '',
    text: 'hi',
    at: '2026-08-24T00:00:00Z',
  }
}

function page(...sequences: number[]): AgentChatMessagesPage {
  return {
    cursor: sequences.at(-1) ?? 0,
    oldestCursor: sequences[0] ?? 0,
    hasMore: false,
    items: sequences.map(message),
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((r) => {
    resolve = r
  })
  return { promise, resolve }
}

const { loadInitial, merge, evict } = chatLedgerStore.getState()
const ledgerOf = (chatId: string) => chatLedgerStore.getState().ledgers[chatId]

describe('chat ledger store', () => {
  beforeEach(() => {
    listChatMessagesFn.mockReset()
    chatLedgerStore.setState({ ledgers: {} })
  })

  it('issues one request for two loads while the first is in flight', async () => {
    const pending = deferred<AgentChatMessagesPage>()
    listChatMessagesFn.mockReturnValue(pending.promise)

    const first = loadInitial('ws', 'c1')
    const second = loadInitial('ws', 'c1')
    pending.resolve(page(1, 5))
    await Promise.all([first, second])

    expect(listChatMessagesFn).toHaveBeenCalledTimes(1)
    expect(ledgerOf('c1').messages.map((m) => m.sequence)).toEqual([1, 5])
    expect(ledgerOf('c1').status).toBe('loaded')
  })

  it('does not fetch again once the first page is loaded', async () => {
    listChatMessagesFn.mockResolvedValue(page(1))
    await loadInitial('ws', 'c1')
    await loadInitial('ws', 'c1')

    expect(listChatMessagesFn).toHaveBeenCalledTimes(1)
  })

  it('drops the response of a load that a forced reload superseded', async () => {
    const stale = deferred<AgentChatMessagesPage>()
    listChatMessagesFn.mockReturnValueOnce(stale.promise).mockResolvedValueOnce(page(7))

    const first = loadInitial('ws', 'c1')
    await loadInitial('ws', 'c1', true)
    stale.resolve(page(1))
    await first

    expect(ledgerOf('c1').messages.map((m) => m.sequence)).toEqual([7])
  })

  it('records the failure and retries on the next load', async () => {
    listChatMessagesFn.mockRejectedValueOnce(new Error('boom')).mockResolvedValueOnce(page(1))

    await loadInitial('ws', 'c1')
    expect(ledgerOf('c1').status).toBe('error')
    expect(ledgerOf('c1').error?.message).toBe('boom')

    await loadInitial('ws', 'c1')
    expect(ledgerOf('c1').status).toBe('loaded')
    expect(ledgerOf('c1').error).toBeNull()
  })

  it('drops a response that lands after the chat was evicted', async () => {
    const pending = deferred<AgentChatMessagesPage>()
    listChatMessagesFn.mockReturnValue(pending.promise)

    const load = loadInitial('ws', 'c1')
    evict('c1')
    pending.resolve(page(1))
    await load

    expect(ledgerOf('c1')).toBeUndefined()
  })

  it('merges by turnId, ordered by displayOrder, and advances the cursors', async () => {
    listChatMessagesFn.mockResolvedValue(page(5))
    await loadInitial('ws', 'c1')

    merge('c1', [{ ...message(5), text: 'edited' }, message(9)])

    expect(ledgerOf('c1').messages.map((m) => m.text)).toEqual(['edited', 'hi'])
    expect(ledgerOf('c1').cursor).toBe(9)
  })

  it('keeps the messages reference when nothing was merged', async () => {
    listChatMessagesFn.mockResolvedValue(page(1))
    await loadInitial('ws', 'c1')
    const before = ledgerOf('c1').messages

    merge('c1', [])

    expect(ledgerOf('c1').messages).toBe(before)
  })

  describe('eviction by layout', () => {
    beforeEach(() => {
      resetWindowPaneStoreForTests()
      windowPaneStore.getState().paneActions.setActiveProject('p1')
    })

    it('evicts a chat when its view closes and keeps the chats of views still open', async () => {
      listChatMessagesFn.mockResolvedValue(page(1))
      const actions = windowPaneStore.getState().paneActions
      actions.openChat('open', { workspaceId: 'ws-a' })
      actions.openChat('closing', { workspaceId: 'ws-b' })
      await loadInitial('ws-a', 'open')
      await loadInitial('ws-b', 'closing')

      const closing = windowPaneStore.getState().viewOrder[1]
      windowPaneStore.getState().paneActions.closeView(closing)

      expect(ledgerOf('open')).toBeDefined()
      expect(ledgerOf('closing')).toBeUndefined()
    })
  })
})
