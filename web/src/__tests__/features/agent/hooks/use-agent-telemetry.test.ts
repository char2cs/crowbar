import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { AgentChat, AgentTelemetry } from '@/features/agent/api/agent-api'
import { limitResetsAt, useAgentTelemetry } from '@/features/agent/hooks/use-agent-telemetry'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'

const { getChatTelemetryFn } = vi.hoisted(() => ({ getChatTelemetryFn: vi.fn() }))

vi.mock('@/features/agent/api/agent-api', () => ({
  getPendingPrompt: vi.fn().mockResolvedValue(null),
  getChatTelemetry: (...args: unknown[]) => getChatTelemetryFn(...args),
}))

function telemetry(usedPercent: number): AgentTelemetry {
  return { observedAt: '2026-08-24T12:00:00Z', source: 'callback', context: { usedPercent } }
}

const store = () => getOrCreateWorkspaceStore('w1')

beforeEach(() => {
  getChatTelemetryFn.mockReset()
  getChatTelemetryFn.mockResolvedValue(telemetry(10))
})

afterEach(() => {
  vi.useRealTimers()
  destroyWorkspaceStore('w1')
})

describe('useAgentTelemetry', () => {
  it('reads nothing at all while the tab is not visible', () => {
    renderHook(() => useAgentTelemetry('w1', 'c1', false))
    expect(getChatTelemetryFn).not.toHaveBeenCalled()
  })

  it('reports what the daemon already holds, read once', async () => {
    const { result } = renderHook(() => useAgentTelemetry('w1', 'c1', true))
    await waitFor(() => expect(result.current?.context?.usedPercent).toBe(10))
  })

  // §6a: idle cost ≈ 0. The gauge moves on the pushed `telemetry` frame; a
  // visible chat never asks again on a clock.
  it('never polls — a pushed report moves the gauge without a read', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const { result } = renderHook(() => useAgentTelemetry('w1', 'c1', true))
    await waitFor(() => expect(result.current?.context?.usedPercent).toBe(10))

    act(() => store().getState().setAgentChatTelemetry('c1', telemetry(42)))
    expect(result.current?.context?.usedPercent).toBe(42)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(60_000)
    })
    expect(getChatTelemetryFn).toHaveBeenCalledTimes(1)
  })

  // A gauge belonging to the previous chat is worse than no gauge: it is a
  // confident number about the wrong conversation.
  it('drops the previous chat’s reading the moment the chat changes', async () => {
    const { result, rerender } = renderHook(({ chatId }) => useAgentTelemetry('w1', chatId, true), {
      initialProps: { chatId: 'c1' },
    })
    await waitFor(() => expect(result.current).not.toBeNull())
    getChatTelemetryFn.mockImplementation(() => new Promise(() => {}))
    rerender({ chatId: 'c2' })
    expect(result.current).toBeNull()
  })

  it('keeps the last good reading when a read fails — it is an indicator, not the conversation', async () => {
    const { result } = renderHook(() => useAgentTelemetry('w1', 'c1', true))
    await waitFor(() => expect(result.current?.context?.usedPercent).toBe(10))
    getChatTelemetryFn.mockRejectedValue(new Error('offline'))
    act(() => {
      store()
        .getState()
        .upsertAgentChat({ id: 'c1', surface: 'terminal' } as unknown as AgentChat)
    })
    await act(async () => {
      await Promise.resolve()
    })
    expect(result.current?.context?.usedPercent).toBe(10)
  })

  // The surface decides whether the chat carries a report at all, and a switch
  // sends no telemetry frame — so it is the one edge that re-reads.
  it('re-reads once when the chat moves to another surface', async () => {
    renderHook(() => useAgentTelemetry('w1', 'c1', true))
    await waitFor(() => expect(getChatTelemetryFn).toHaveBeenCalledTimes(1))
    act(() => {
      store()
        .getState()
        .upsertAgentChat({ id: 'c1', surface: 'terminal' } as unknown as AgentChat)
    })
    await waitFor(() => expect(getChatTelemetryFn).toHaveBeenCalledTimes(2))
  })
})

describe('limitResetsAt', () => {
  it('is undefined when the provider reports no windows', () => {
    expect(limitResetsAt(null)).toBeUndefined()
    expect(limitResetsAt(telemetry(10))).toBeUndefined()
  })

  // The window that stopped the turn is the one closest to SPENT, not the one
  // that resets soonest — sending someone back in ten minutes to hit the same
  // wall is worse than telling them to come back tomorrow.
  it('names the most-consumed window, not the earliest one', () => {
    const resets = limitResetsAt({
      observedAt: '2026-08-24T12:00:00Z',
      source: 'callback',
      rateLimits: [
        { id: 'five_hour', usedPercent: 20, resetsAt: '2026-08-24T13:00:00Z' },
        { id: 'seven_day', usedPercent: 99, resetsAt: '2026-08-31T00:00:00Z' },
      ],
    })
    expect(resets).toBe('2026-08-31T00:00:00Z')
  })

  it('ignores a window the provider gave no reset for', () => {
    expect(
      limitResetsAt({
        observedAt: '2026-08-24T12:00:00Z',
        source: 'callback',
        rateLimits: [{ id: 'five_hour', usedPercent: 99 }],
      }),
    ).toBeUndefined()
  })
})
