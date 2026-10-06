import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { useAgentActivity } from '@/features/agent/hooks/use-agent-activity'

const listChatActivity = vi.fn()

vi.mock('@/features/agent/api/agent-api', () => ({
  getPendingPrompt: vi.fn().mockResolvedValue(null),
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
      renderHook(() => useAgentActivity('ws1', 'c1', false, false, true))
      await vi.advanceTimersByTimeAsync(10_000)

      expect(listChatActivity).toHaveBeenCalledTimes(1)
    } finally {
      vi.useRealTimers()
    }
  })

  it('reads nothing while the tab is hidden', () => {
    renderHook(() => useAgentActivity('ws1', 'c1', true, false, false))

    expect(listChatActivity).not.toHaveBeenCalled()
  })

  it('polls while a turn runs', async () => {
    vi.useFakeTimers()
    try {
      renderHook(() => useAgentActivity('ws1', 'c1', true, false, true))
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

    const { result } = renderHook(() => useAgentActivity('ws1', 'c1', true, false, true))

    await waitFor(() => expect(result.current.toolCalls).toHaveLength(1))
  })

  // The falling edge matters: the last tool completion lands after the turn
  // state flips, so without a final read a finished turn shows stale work.
  it('takes one final read when the turn ends', async () => {
    const { rerender } = renderHook(
      ({ working }: { working: boolean }) => useAgentActivity('ws1', 'c1', working, false, true),
      { initialProps: { working: true } },
    )
    await waitFor(() => expect(listChatActivity).toHaveBeenCalled())
    const duringTurn = listChatActivity.mock.calls.length

    rerender({ working: false })

    await waitFor(() => expect(listChatActivity.mock.calls.length).toBeGreaterThan(duringTurn))
  })

  // REGRESSION: the falling-edge read can itself lose the race it exists to
  // close — the last tool completion landing server-side AFTER `working` flips
  // is exactly what the surrounding code's own comment describes, and a single
  // unconditional read has no way to tell "genuinely still running" apart from
  // "the completion just hasn't landed yet". Without a retry, a tool caught mid-
  // flight here shows as running FOREVER: `live` is already false, so nothing
  // ever polls again until some later, unrelated turn does.
  it('retries the falling-edge read until a tool call it caught mid-flight actually closes', async () => {
    vi.useFakeTimers()
    try {
      const running = {
        id: 't1',
        turnId: 'turn-1',
        seq: 1,
        name: 'Bash',
        status: 'running',
        hasRequest: false,
        hasResult: false,
        startedAt: 'x',
      }
      listChatActivity
        .mockResolvedValueOnce({ ...empty, toolCalls: [running] }) // during the turn
        .mockResolvedValueOnce({ ...empty, toolCalls: [running] }) // falling edge: still open
        .mockResolvedValueOnce({
          ...empty,
          toolCalls: [{ ...running, status: 'ok', endedAt: 'y' }],
        })

      const { result, rerender } = renderHook(
        ({ working }: { working: boolean }) => useAgentActivity('ws1', 'c1', working, false, true),
        { initialProps: { working: true } },
      )
      await act(() => vi.advanceTimersByTimeAsync(0))
      expect(result.current.toolCalls[0]?.status).toBe('running')

      rerender({ working: false })
      await act(() => vi.advanceTimersByTimeAsync(0))
      expect(result.current.toolCalls[0]?.status).toBe('running')

      await act(() => vi.advanceTimersByTimeAsync(1_000))
      expect(result.current.toolCalls[0]?.status).toBe('ok')
    } finally {
      vi.useRealTimers()
    }
  })

  // REGRESSION: a compaction never sets `working` (compact.go opens no tracked
  // turn), so without `compacting` counted as its own live edge, the resolved
  // compaction's divider stayed invisible until some LATER, unrelated read —
  // in practice, the user's next prompt.
  it('takes one final read when a compaction ends, even though working never moved', async () => {
    const { rerender } = renderHook(
      ({ compacting }: { compacting: boolean }) =>
        useAgentActivity('ws1', 'c1', false, compacting, true),
      { initialProps: { compacting: true } },
    )
    await waitFor(() => expect(listChatActivity).toHaveBeenCalled())
    const duringCompaction = listChatActivity.mock.calls.length

    rerender({ compacting: false })

    await waitFor(() =>
      expect(listChatActivity.mock.calls.length).toBeGreaterThan(duringCompaction),
    )
  })

  // A prompt waiting on a human has a channel of its own — the daemon pushes it —
  // so it is neither read for nor polled for. Only tool calls lack one.
  it('does not poll on a pending prompt’s account once the turn stopped working', async () => {
    vi.useFakeTimers()
    try {
      const { result } = renderHook(() =>
        useAgentActivity('ws1', 'c1', false, false, true, 0, [choice()] as never),
      )
      await act(() => vi.advanceTimersByTimeAsync(0))
      await act(() => vi.advanceTimersByTimeAsync(10_000))

      expect(result.current.choices).toHaveLength(1)
      expect(listChatActivity).toHaveBeenCalledTimes(1)
    } finally {
      vi.useRealTimers()
    }
  })

  it('shows the pushed prompt over a read that does not know it, and drops a stale pending one', async () => {
    listChatActivity.mockResolvedValue({
      ...empty,
      choices: [choice({ id: 'stale', pending: true }), choice({ id: 'old', pending: false })],
    })

    const { result } = renderHook(() =>
      useAgentActivity('ws1', 'c1', false, false, true, 0, [choice({ id: 'fresh' })] as never),
    )

    await waitFor(() => expect(result.current.choices.map((c) => c.id)).toEqual(['old', 'fresh']))
  })

  // However the prompt stopped pending — answered, expired, decided at the
  // terminal — the daemon says so, and the resolved record is one read away.
  it('takes one read of the resolved record when the pushed prompt goes away', async () => {
    const { rerender } = renderHook(
      ({ pending }: { pending: unknown[] }) =>
        useAgentActivity('ws1', 'c1', false, false, true, 0, pending as never),
      { initialProps: { pending: [choice()] } },
    )
    await waitFor(() => expect(listChatActivity).toHaveBeenCalledTimes(1))

    rerender({ pending: [] })

    await waitFor(() => expect(listChatActivity).toHaveBeenCalledTimes(2))
  })

  it('drops the previous chat timeline when the chat changes', async () => {
    listChatActivity.mockResolvedValue({
      ...empty,
      subagents: [{ id: 'a', turnId: 't', seq: 1, startedAt: 'x' }],
    })
    const { result, rerender } = renderHook(
      ({ chatId }: { chatId: string }) => useAgentActivity('ws1', chatId, true, false, true),
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
    const { result } = renderHook(() => useAgentActivity('ws1', 'c1', true, false, true))
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

    const { result } = renderHook(() => useAgentActivity('ws1', 'c1', false, false, true))

    await waitFor(() => expect(result.current.toolCalls).toHaveLength(1))
  })

  it('still reads nothing while hidden', () => {
    renderHook(() => useAgentActivity('ws1', 'c1', false, false, false))

    expect(listChatActivity).not.toHaveBeenCalled()
  })
})

// A parked chat stays mounted. Its timeline is kept current by the same edges a
// visible chat sees, so showing it again reads nothing unless a turn is still
// running (a tool starting mid-turn announces nothing, so that read is the only
// way to catch up).
describe('useAgentActivity across hide and show', () => {
  type Props = { working: boolean; visible: boolean }
  const mount = (initialProps: Props) =>
    renderHook(
      ({ working, visible }: Props) => useAgentActivity('ws1', 'c1', working, false, visible),
      {
        initialProps,
      },
    )

  it('does not re-read an idle chat each time it is shown again', async () => {
    const { rerender } = mount({ working: false, visible: true })
    await waitFor(() => expect(listChatActivity).toHaveBeenCalledTimes(1))

    rerender({ working: false, visible: false })
    rerender({ working: false, visible: true })
    rerender({ working: false, visible: false })
    rerender({ working: false, visible: true })
    await act(async () => {})

    expect(listChatActivity).toHaveBeenCalledTimes(1)
  })

  it('takes the falling-edge read while hidden, so a turn that finished unseen shows its work on show', async () => {
    const { rerender } = mount({ working: true, visible: true })
    await waitFor(() => expect(listChatActivity).toHaveBeenCalled())
    rerender({ working: true, visible: false })
    listChatActivity.mockClear()

    rerender({ working: false, visible: false })
    await waitFor(() => expect(listChatActivity).toHaveBeenCalledTimes(1))

    rerender({ working: false, visible: true })
    await act(async () => {})
    expect(listChatActivity).toHaveBeenCalledTimes(1)
  })

  it('reads at once when a chat still running is shown, without waiting for the next poll', async () => {
    const { rerender } = mount({ working: true, visible: false })
    expect(listChatActivity).not.toHaveBeenCalled()

    rerender({ working: true, visible: true })

    await waitFor(() => expect(listChatActivity).toHaveBeenCalledTimes(1))
  })

  // A prompt that opened while the chat was parked is already there on the first
  // frame it is shown: the daemon pushed it, so showing it reads nothing for it.
  it('shows a prompt that arrived while hidden with no activity read on show', async () => {
    const { result, rerender } = renderHook(
      ({ visible, pending }: { visible: boolean; pending: unknown[] }) =>
        useAgentActivity('ws1', 'c1', false, false, visible, 0, pending as never),
      { initialProps: { visible: true, pending: [] as unknown[] } },
    )
    await waitFor(() => expect(listChatActivity).toHaveBeenCalledTimes(1))
    rerender({ visible: false, pending: [] })

    rerender({ visible: false, pending: [choice()] })
    expect(result.current.choices.map((c) => c.id)).toEqual(['k1'])

    rerender({ visible: true, pending: [choice()] })
    expect(result.current.choices.map((c) => c.id)).toEqual(['k1'])
    await act(async () => {})
    expect(listChatActivity).toHaveBeenCalledTimes(1)
  })

  // A reconnect bumps every chat's turn revision with no working edge: a turn can
  // have run, start to finish, inside the outage.
  it('re-reads an idle hidden chat when its turn revision moves with no working edge', async () => {
    const { rerender } = renderHook(
      ({ turnRevision, visible }: { turnRevision: number; visible: boolean }) =>
        useAgentActivity('ws1', 'c1', false, false, visible, turnRevision),
      { initialProps: { turnRevision: 0, visible: true } },
    )
    await waitFor(() => expect(listChatActivity).toHaveBeenCalledTimes(1))
    rerender({ turnRevision: 0, visible: false })

    rerender({ turnRevision: 1, visible: false })

    await waitFor(() => expect(listChatActivity).toHaveBeenCalledTimes(2))
  })
})
