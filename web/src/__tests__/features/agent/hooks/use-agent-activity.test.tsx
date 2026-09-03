import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { useAgentActivity } from '@/features/agent/hooks/use-agent-activity'

const listChatActivity = vi.fn()

vi.mock('@/features/agent/api/agent-api', () => ({
  listChatActivity: (...args: unknown[]) => listChatActivity(...args),
}))

const empty = { toolCalls: [], subagents: [], interruptions: [], choices: [] }

beforeEach(() => {
  listChatActivity.mockReset()
  listChatActivity.mockResolvedValue(empty)
})

function choice(overrides: Record<string, unknown> = {}) {
  return {
    id: 'k1',
    turnId: 'turn-1',
    seq: 1,
    kind: 'tool_permission',
    toolName: 'Bash',
    options: [],
    pending: true,
    answerable: true,
    at: '2026-08-18T12:00:00Z',
    ...overrides,
  }
}

describe('useAgentActivity', () => {
  // An idle chat reads its timeline ONCE and then costs nothing: activity has no
  // push channel, so polling is scoped to exactly the window where something can
  // change.
  it('reads once for an idle chat and never polls it', async () => {
    vi.useFakeTimers()
    try {
      renderHook(() => useAgentActivity('ws1', 'c1', false, true))
      await vi.advanceTimersByTimeAsync(10_000)

      expect(listChatActivity).toHaveBeenCalledTimes(1)
    } finally {
      vi.useRealTimers()
    }
  })

  it('reads nothing while the tab is hidden', () => {
    renderHook(() => useAgentActivity('ws1', 'c1', true, false))

    expect(listChatActivity).not.toHaveBeenCalled()
  })

  it('polls while a turn runs', async () => {
    vi.useFakeTimers()
    try {
      renderHook(() => useAgentActivity('ws1', 'c1', true, true))
      await vi.advanceTimersByTimeAsync(5_000)

      expect(listChatActivity.mock.calls.length).toBeGreaterThan(2)
    } finally {
      vi.useRealTimers()
    }
  })

  it('reads immediately once a turn starts', async () => {
    listChatActivity.mockResolvedValue({
      ...empty,
      toolCalls: [
        {
          id: 't1',
          turnId: 'turn-1',
          seq: 1,
          name: 'Bash',
          status: 'running',
          hasRequest: false,
          hasResult: false,
          startedAt: 'x',
        },
      ],
    })

    const { result } = renderHook(() => useAgentActivity('ws1', 'c1', true, true))

    await waitFor(() => expect(result.current.toolCalls).toHaveLength(1))
  })

  // The falling edge matters: the last tool completion lands after the turn
  // state flips, so without a final read a finished turn shows stale work.
  it('takes one final read when the turn ends', async () => {
    const { rerender } = renderHook(
      ({ working }: { working: boolean }) => useAgentActivity('ws1', 'c1', working, true),
      { initialProps: { working: true } },
    )
    await waitFor(() => expect(listChatActivity).toHaveBeenCalled())
    const duringTurn = listChatActivity.mock.calls.length

    rerender({ working: false })

    await waitFor(() => expect(listChatActivity.mock.calls.length).toBeGreaterThan(duringTurn))
  })

  // A prompt waiting on a human is the OTHER way a chat is unfinished. It can
  // stop pending with no action here — somebody answers at the terminal, or the
  // relay holding the CLI's gate expires — so the poll has to outlive the turn
  // that opened it or the card would sit there offering buttons that reach nobody.
  it('keeps polling for a pending prompt after the turn stops working', async () => {
    vi.useFakeTimers()
    try {
      listChatActivity.mockResolvedValue({ ...empty, choices: [choice({ pending: true })] })

      renderHook(() => useAgentActivity('ws1', 'c1', false, true))
      // The mount read has to LAND before the window that observes polling: the
      // prompt it carries is what makes the chat live in the first place.
      await act(() => vi.advanceTimersByTimeAsync(0))
      await act(() => vi.advanceTimersByTimeAsync(5_000))

      expect(listChatActivity.mock.calls.length).toBeGreaterThan(2)
    } finally {
      vi.useRealTimers()
    }
  })

  // And it goes quiet again the moment the server says the prompt stopped
  // pending — however it stopped. This view is advisory: the terminal can resolve
  // a prompt at any instant, so the read is the authority, not the click.
  it('stops polling once the prompt stops pending', async () => {
    vi.useFakeTimers()
    try {
      listChatActivity.mockResolvedValue({ ...empty, choices: [choice({ pending: true })] })
      const { result } = renderHook(() => useAgentActivity('ws1', 'c1', false, true))
      await act(() => vi.advanceTimersByTimeAsync(0))
      await act(() => vi.advanceTimersByTimeAsync(5_000))
      expect(listChatActivity.mock.calls.length).toBeGreaterThan(2)

      // Answered at the TERMINAL, by somebody else.
      listChatActivity.mockResolvedValue({
        ...empty,
        choices: [choice({ pending: false, resolution: 'proceeded' })],
      })
      await act(() => vi.advanceTimersByTimeAsync(5_000))
      await act(() => vi.advanceTimersByTimeAsync(0))
      expect(result.current.choices[0]?.pending).toBe(false)

      const settled = listChatActivity.mock.calls.length
      await act(() => vi.advanceTimersByTimeAsync(10_000))
      expect(listChatActivity.mock.calls.length).toBe(settled)
    } finally {
      vi.useRealTimers()
    }
  })

  it('drops the previous chat timeline when the chat changes', async () => {
    listChatActivity.mockResolvedValue({
      ...empty,
      subagents: [{ id: 'a', turnId: 't', seq: 1, startedAt: 'x' }],
    })
    const { result, rerender } = renderHook(
      ({ chatId }: { chatId: string }) => useAgentActivity('ws1', chatId, true, true),
      { initialProps: { chatId: 'c1' } },
    )
    await waitFor(() => expect(result.current.subagents).toHaveLength(1))

    listChatActivity.mockImplementation(() => new Promise(() => {}))
    rerender({ chatId: 'c2' })

    await waitFor(() => expect(result.current.subagents).toHaveLength(0))
  })

  // Activity is a legibility surface, not the conversation. A failed read leaves
  // the last good timeline standing.
  it('keeps the last good timeline when a read fails', async () => {
    listChatActivity.mockResolvedValueOnce({
      ...empty,
      subagents: [{ id: 'a', turnId: 't', seq: 1, startedAt: 'x' }],
    })
    const { result } = renderHook(() => useAgentActivity('ws1', 'c1', true, true))
    await waitFor(() => expect(result.current.subagents).toHaveLength(1))

    listChatActivity.mockRejectedValue(new Error('daemon restarting'))
    await new Promise((resolve) => setTimeout(resolve, 0))

    expect(result.current.subagents).toHaveLength(1)
  })
})

// A chat opened AFTER its turns finished still has a timeline. Without a read on
// mount it shows a reply with none of the work that produced it.
describe('useAgentActivity on mount', () => {
  it('reads the completed timeline when an idle chat becomes visible', async () => {
    listChatActivity.mockResolvedValue({
      ...empty,
      toolCalls: [
        {
          id: 't1',
          turnId: 'turn-1',
          seq: 1,
          name: 'Bash',
          status: 'ok',
          hasRequest: true,
          hasResult: true,
          startedAt: 'x',
        },
      ],
    })

    const { result } = renderHook(() => useAgentActivity('ws1', 'c1', false, true))

    await waitFor(() => expect(result.current.toolCalls).toHaveLength(1))
  })

  it('still reads nothing while hidden', () => {
    renderHook(() => useAgentActivity('ws1', 'c1', false, false))

    expect(listChatActivity).not.toHaveBeenCalled()
  })
})
