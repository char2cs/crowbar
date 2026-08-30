import { renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgentChatMessage } from '@/features/agent/api/agent-api'
import { useChatMessages } from '@/features/agent/hooks/use-chat-messages'

const { listChatMessagesFn } = vi.hoisted(() => ({ listChatMessagesFn: vi.fn() }))
vi.mock('@/features/agent/api/agent-api', () => ({ listChatMessages: listChatMessagesFn }))

function message(sequence: number, overrides: Partial<AgentChatMessage> = {}): AgentChatMessage {
  return {
    turnId: `t${sequence}`,
    sequence,
    role: 'user',
    providerId: '',
    text: 'hi',
    at: '2026-08-24T00:00:00Z',
    ...overrides,
  }
}

describe('applyMessages empty-page guard', () => {
  beforeEach(() => {
    listChatMessagesFn.mockReset()
  })

  it('still calls onApply on an empty page, so queue reconciliation keeps working', async () => {
    const onApply = vi.fn()
    listChatMessagesFn.mockResolvedValue({ cursor: 0, oldestCursor: 0, hasMore: false, items: [] })
    renderHook(() =>
      useChatMessages({
        wsId: 'ws',
        chatId: 'c1',
        providerId: 'claude',
        visible: true,
        working: false,
        turnRevision: 0,
        awaiting: false,
        onApply,
        pendingEvidence: () => false,
        pendingBaselines: () => [],
        onRecoveryExhausted: () => {},
      }),
    )
    await waitFor(() => expect(onApply).toHaveBeenCalled())
    expect(onApply).toHaveBeenCalledWith([])
  })

  it('does not create a new messages array reference across two empty pages', async () => {
    const seen: AgentChatMessage[][] = []
    listChatMessagesFn
      .mockResolvedValueOnce({
        cursor: 5,
        oldestCursor: 1,
        hasMore: false,
        items: [message(1), message(5)],
      })
      .mockResolvedValue({ cursor: 5, oldestCursor: 1, hasMore: false, items: [] })
    // Options are built once, outside the render callback — like the real caller
    // (agent-chat-view.tsx), which passes useCallback-memoized handlers from
    // use-prompt-queue.ts. Rebuilding them inline on every render would give
    // applyMessages/loadInitial a fresh identity each time messages state
    // changes, re-triggering the mount effect and calling loadInitial again —
    // an artifact this hook's real callers never produce.
    const options = {
      wsId: 'ws',
      chatId: 'c1',
      providerId: 'claude',
      visible: true,
      working: true,
      turnRevision: 0,
      awaiting: false,
      onApply: (m: AgentChatMessage[]) => seen.push(m),
      pendingEvidence: () => false,
      pendingBaselines: (): number[] => [],
      onRecoveryExhausted: () => {},
    }
    const { result, rerender } = renderHook(() => useChatMessages(options))
    await waitFor(() => expect(result.current.messages).toHaveLength(2))
    const firstRef = result.current.messages
    await result.current.refresh()
    rerender()
    expect(result.current.messages).toBe(firstRef)
  })

  it('clears rendered messages when loadInitial re-runs and returns an empty page', async () => {
    listChatMessagesFn
      .mockResolvedValueOnce({
        cursor: 5,
        oldestCursor: 1,
        hasMore: false,
        items: [message(1), message(5)],
      })
      .mockResolvedValue({ cursor: 5, oldestCursor: 1, hasMore: false, items: [] })
    // Same stabilization as above: a stable options reference so loadInitial's
    // identity doesn't churn on its own state updates.
    const options = {
      wsId: 'ws',
      chatId: 'c1',
      providerId: 'claude',
      visible: true,
      working: false,
      turnRevision: 0,
      awaiting: false,
      onApply: () => {},
      pendingEvidence: () => false,
      pendingBaselines: (): number[] => [],
      onRecoveryExhausted: () => {},
    }
    const { result, rerender } = renderHook(() => useChatMessages(options))
    await waitFor(() => expect(result.current.messages).toHaveLength(2))

    listChatMessagesFn.mockResolvedValueOnce({ cursor: 0, oldestCursor: 0, hasMore: false, items: [] })
    await result.current.loadInitial()
    rerender()

    expect(result.current.messages).toHaveLength(0)
  })
})

describe('ordering: displayOrder over sequence', () => {
  beforeEach(() => {
    listChatMessagesFn.mockReset()
  })

  // Regression: an interrupted turn (e.g. Claude, asked to stop) can finish
  // and get PERSISTED — and so get a higher `sequence` — after a later
  // turn (e.g. Codex, dispatched after a provider switch) that finished
  // first. `displayOrder` is reserved at dispatch time specifically so the
  // transcript still shows them in the order they were SENT, not the order
  // they happened to finish.
  it('sorts by displayOrder, not sequence, when a lower-sequence turn was dispatched later', async () => {
    listChatMessagesFn
      .mockResolvedValueOnce({
        cursor: 5,
        oldestCursor: 1,
        hasMore: false,
        items: [
          // Codex: dispatched second, persisted first (higher displayOrder,
          // lower sequence).
          message(1, { text: 'codex reply', displayOrder: 2 }),
          // Claude: dispatched first, persisted late (lower displayOrder,
          // higher sequence).
          message(5, { text: 'claude reply', displayOrder: 1 }),
        ],
      })
      .mockResolvedValue({ cursor: 5, oldestCursor: 1, hasMore: false, items: [] })
    // Stable reference, built outside the render callback — see the
    // "does not create a new messages array reference" test above for why:
    // an inline object here gets a fresh onApply identity every render,
    // which re-triggers the mount effect and calls loadInitial again,
    // racing this very mock into being consumed by the wrong call.
    const options = {
      wsId: 'ws',
      chatId: 'c1',
      providerId: 'claude',
      visible: true,
      working: false,
      turnRevision: 0,
      awaiting: false,
      onApply: () => {},
      pendingEvidence: () => false,
      pendingBaselines: (): number[] => [],
      onRecoveryExhausted: () => {},
    }
    const { result } = renderHook(() => useChatMessages(options))

    await waitFor(() => expect(result.current.messages).toHaveLength(2))
    expect(result.current.messages.map((m) => m.text)).toEqual(['claude reply', 'codex reply'])
  })

  it('falls back to sequence when displayOrder is absent (fixtures predating the field)', async () => {
    listChatMessagesFn
      .mockResolvedValueOnce({
        cursor: 5,
        oldestCursor: 1,
        hasMore: false,
        items: [message(5, { text: 'second' }), message(1, { text: 'first' })],
      })
      .mockResolvedValue({ cursor: 5, oldestCursor: 1, hasMore: false, items: [] })
    const options = {
      wsId: 'ws',
      chatId: 'c1',
      providerId: 'claude',
      visible: true,
      working: false,
      turnRevision: 0,
      awaiting: false,
      onApply: () => {},
      pendingEvidence: () => false,
      pendingBaselines: (): number[] => [],
      onRecoveryExhausted: () => {},
    }
    const { result } = renderHook(() => useChatMessages(options))

    await waitFor(() => expect(result.current.messages).toHaveLength(2))
    expect(result.current.messages.map((m) => m.text)).toEqual(['first', 'second'])
  })
})

describe('mergeMessages: keyed by turnId, not sequence', () => {
  beforeEach(() => {
    listChatMessagesFn.mockReset()
  })

  // Regression, live 2026-08-29: "Noted — saw the Codex exchange…" rendered
  // twice, one copy missing `effort`. Root cause: turn/message.go's
  // closeAssistantTurn always re-records the LAST streamed item when its turn
  // closes — even when nothing but `effort` changed — which persists the SAME
  // turnId again under a FRESH, higher `sequence`. A poll that lands between
  // the two closes caches the first under its own sequence, then the second
  // poll's `after=cursor` page brings the same turnId back under the new
  // sequence — keying the merge map by sequence treated that as a distinct
  // row instead of an update, so both survived forever in this client's own
  // cache (a fresh page load never showed it: the backend's SQL storage
  // upserts by turnId, so only the latest survives there).
  it('replaces the earlier row when the same turnId reappears under a new sequence', async () => {
    // Keyed on `after`, not call order: this hook's own auto-refresh (on
    // mount, and again the instant `loading` settles) can legitimately issue
    // an extra `after=18` request before this test's own explicit refresh()
    // does — the fix must hold regardless of which call actually fetches the
    // second close.
    let secondCloseServed = false
    listChatMessagesFn.mockImplementation(
      async (_ws: string, _chat: string, opts?: { after?: number }) => {
        const after = opts?.after ?? 0
        if (after === 0) {
          return {
            cursor: 18,
            oldestCursor: 18,
            hasMore: false,
            items: [
              message(18, {
                role: 'assistant',
                turnId: 'msg-191b6652',
                displayOrder: 17,
                text: 'Noted — saw the Codex exchange with the contrary Iron Man essay.',
              }),
            ],
          }
        }
        if (!secondCloseServed) {
          secondCloseServed = true
          return {
            cursor: 20,
            oldestCursor: 18,
            hasMore: false,
            items: [
              message(20, {
                role: 'assistant',
                turnId: 'msg-191b6652',
                displayOrder: 19,
                effort: 'xhigh',
                text: 'Noted — saw the Codex exchange with the contrary Iron Man essay.',
              }),
            ],
          }
        }
        return { cursor: after, oldestCursor: 18, hasMore: false, items: [] }
      },
    )
    const options = {
      wsId: 'ws',
      chatId: 'c1',
      providerId: 'claude',
      visible: true,
      working: true,
      turnRevision: 0,
      awaiting: false,
      onApply: () => {},
      pendingEvidence: () => false,
      pendingBaselines: (): number[] => [],
      onRecoveryExhausted: () => {},
    }
    const { result } = renderHook(() => useChatMessages(options))
    await waitFor(() => expect(result.current.messages).toHaveLength(1))

    await result.current.refresh()
    await waitFor(() => {
      expect(result.current.messages).toHaveLength(1)
      expect(result.current.messages[0].sequence).toBe(20)
    })

    expect(result.current.messages[0].effort).toBe('xhigh')
  })
})

describe('streamingBubbles: suppressed by id, not by text', () => {
  beforeEach(() => {
    listChatMessagesFn.mockReset()
  })

  // Regression: turn/message.go's closeAssistantTurn can RECONCILE a streamed
  // message's text against the terminating hook's own copy when the two
  // disagree — the persisted row then legitimately has different text than
  // what was streamed, under the SAME message id (assistantTurnID(messageID)
  // never changes because the text did). A text-equality check misses that
  // row entirely and leaves the stale streaming bubble on screen underneath
  // the real one forever — the exact duplicate the user reported live.
  it('suppresses a bubble whose ledger row was reconciled to different text', async () => {
    listChatMessagesFn.mockResolvedValueOnce({
      cursor: 5,
      oldestCursor: 1,
      hasMore: false,
      items: [
        message(5, {
          role: 'assistant',
          turnId: 'msg-delta-1',
          text: 'the reconciled, authoritative final text',
        }),
      ],
    })
    const options = {
      wsId: 'ws',
      chatId: 'c1',
      providerId: 'claude',
      visible: true,
      working: false,
      turnRevision: 0,
      awaiting: false,
      // What was actually streamed, before reconciliation replaced it —
      // deliberately NOT equal to the ledger row's text above.
      streamingMessages: [{ id: 'delta-1', text: 'the streamed text, missing a word' }],
      onApply: () => {},
      pendingEvidence: () => false,
      pendingBaselines: (): number[] => [],
      onRecoveryExhausted: () => {},
    }
    const { result } = renderHook(() => useChatMessages(options))

    await waitFor(() => expect(result.current.messages).toHaveLength(1))
    expect(result.current.streamingBubbles).toEqual([])
  })

  it('still shows a bubble for a message id the ledger has not recorded yet', async () => {
    listChatMessagesFn.mockResolvedValueOnce({ cursor: 0, oldestCursor: 0, hasMore: false, items: [] })
    const options = {
      wsId: 'ws',
      chatId: 'c1',
      providerId: 'claude',
      visible: true,
      working: true,
      turnRevision: 0,
      awaiting: false,
      streamingMessages: [{ id: 'delta-1', text: 'still being said' }],
      onApply: () => {},
      pendingEvidence: () => false,
      pendingBaselines: (): number[] => [],
      onRecoveryExhausted: () => {},
    }
    const { result } = renderHook(() => useChatMessages(options))

    await waitFor(() => expect(result.current.streamingBubbles).toHaveLength(1))
    expect(result.current.streamingBubbles[0].text).toBe('still being said')
  })
})
