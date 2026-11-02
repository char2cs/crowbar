import { act, render, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'

import type { AgentTelemetry } from '@/features/agent/api/agent-chat-reads'
import { applyEntityFrame } from '@/core/entities/entity-store'
import {
  limitResetsAt,
  useChatTelemetry,
  useChatWorking,
} from '@/features/agent/hooks/use-chat-entity'
import { resetChatStore, setChatWorking, writeChat } from '@/__tests__/__fixtures__/agent-chat'

function telemetry(usedPercent: number): AgentTelemetry {
  return { observedAt: '2026-08-24T12:00:00Z', source: 'callback', context: { usedPercent } }
}

beforeEach(() => {
  resetChatStore()
})

describe('useChatWorking', () => {
  it('re-renders a sidebar row only when its own chat working flips', () => {
    const renders: Record<string, number> = { a: 0, b: 0 }
    function Row({ chatId }: { chatId: string }) {
      renders[chatId] = (renders[chatId] ?? 0) + 1
      return <span data-testid={chatId}>{String(useChatWorking(chatId))}</span>
    }
    writeChat({ id: 'a' })
    writeChat({ id: 'b' })
    const { getByTestId } = render(
      <>
        <Row chatId="a" />
        <Row chatId="b" />
      </>,
    )
    act(() => setChatWorking('a', true))
    act(() => writeChat({ id: 'a', title: 'renamed' }))
    expect(getByTestId('a').textContent).toBe('true')
    expect(renders).toEqual({ a: 2, b: 1 })
  })
})

describe('useChatTelemetry', () => {
  const report = (chatId: string, v: number, usedPercent: number) =>
    applyEntityFrame({
      t: 'entity',
      kind: 'telemetry',
      id: chatId,
      scope: `chat:${chatId}`,
      v,
      data: telemetry(usedPercent),
    })

  it('is null until the chat scope sends a report', () => {
    const { result } = renderHook(() => useChatTelemetry('c1'))
    expect(result.current).toBeNull()
  })

  it('moves with each pushed report and ignores an older one', () => {
    const { result } = renderHook(() => useChatTelemetry('c1'))
    act(() => report('c1', 2, 42))
    expect(result.current?.context?.usedPercent).toBe(42)
    act(() => report('c1', 1, 10))
    expect(result.current?.context?.usedPercent).toBe(42)
  })

  // A gauge belonging to the previous chat is worse than no gauge: it is a
  // confident number about the wrong conversation.
  it('drops the previous chat’s reading the moment the chat changes', () => {
    act(() => report('c1', 1, 10))
    const { result, rerender } = renderHook(({ chatId }) => useChatTelemetry(chatId), {
      initialProps: { chatId: 'c1' },
    })
    expect(result.current).not.toBeNull()
    rerender({ chatId: 'c2' })
    expect(result.current).toBeNull()
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
