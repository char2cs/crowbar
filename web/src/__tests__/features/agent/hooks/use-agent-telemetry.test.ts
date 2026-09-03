import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { AgentTelemetry } from '@/features/agent/api/agent-api'
import { limitResetsAt, useAgentTelemetry } from '@/features/agent/hooks/use-agent-telemetry'

const { getChatTelemetryFn } = vi.hoisted(() => ({ getChatTelemetryFn: vi.fn() }))

vi.mock('@/features/agent/api/agent-api', () => ({
  getChatTelemetry: (...args: unknown[]) => getChatTelemetryFn(...args),
}))

function telemetry(usedPercent: number): AgentTelemetry {
  return { observedAt: '2026-08-24T12:00:00Z', source: 'callback', context: { usedPercent } }
}

beforeEach(() => {
  getChatTelemetryFn.mockReset()
  getChatTelemetryFn.mockResolvedValue(telemetry(10))
})

describe('useAgentTelemetry', () => {
  it('reads nothing at all while the tab is not visible', () => {
    renderHook(() => useAgentTelemetry('w1', 'c1', false))
    expect(getChatTelemetryFn).not.toHaveBeenCalled()
  })

  it('reports what the provider sent', async () => {
    const { result } = renderHook(() => useAgentTelemetry('w1', 'c1', true))
    await waitFor(() => expect(result.current?.context?.usedPercent).toBe(10))
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
    await act(async () => {
      await Promise.resolve()
    })
    expect(result.current?.context?.usedPercent).toBe(10)
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
