import { renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgentChatMessage } from '@/features/agent/api/agent-api'
import { useChatMessages } from '@/features/agent/hooks/use-chat-messages'

const { listChatMessagesFn } = vi.hoisted(() => ({ listChatMessagesFn: vi.fn() }))
vi.mock('@/features/agent/api/agent-api', () => ({ listChatMessages: listChatMessagesFn }))

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
    const { result, rerender } = renderHook(() =>
      useChatMessages({
        wsId: 'ws',
        chatId: 'c1',
        providerId: 'claude',
        visible: true,
        working: true,
        turnRevision: 0,
        awaiting: false,
        onApply: (m) => seen.push(m),
        pendingEvidence: () => false,
        pendingBaselines: () => [],
        onRecoveryExhausted: () => {},
      }),
    )
    await waitFor(() => expect(result.current.messages).toHaveLength(2))
    const firstRef = result.current.messages
    await result.current.refresh()
    rerender()
    expect(result.current.messages).toBe(firstRef)
  })
})
