import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import type { LedgerRow } from '@/features/agent/api/chat-rows'
import { applyFeedFrame, chatStore, openFeed } from '@/features/agent/stores/chat-store'
import type { OutboxItem } from '@/features/agent/stores/lib/outbox'
import { useTopTray } from '@/features/agent/trays/use-top-tray'
import { ledger } from '@/__tests__/__fixtures__/ledger-rows'

const push = (row: LedgerRow) => act(() => void applyFeedFrame('chat-1', { type: 'row', row }))
const item = (requestId: string, state: OutboxItem['state']): OutboxItem => ({
  requestId,
  wsId: 'ws',
  text: requestId,
  state,
  steer: false,
})

beforeEach(() => {
  chatStore.setState({ feeds: {}, outbox: {} })
  openFeed('chat-1')
  applyFeedFrame('chat-1', { type: 'ready', epoch: 'e1' })
})

describe('useTopTray', () => {
  it('is empty for a quiet chat', () => {
    const { result } = renderHook(() => useTopTray('chat-1', undefined))
    expect(result.current).toEqual({ steps: null, queued: [], failed: undefined, resolved: [] })
  })

  it('keeps the plan while a step is left and drops it once all are done', () => {
    const steps = [
      { id: '1', text: 'a', status: 'done' },
      { id: '2', text: 'b', status: 'pending' },
    ]
    const { result, rerender } = renderHook(({ plan }) => useTopTray('chat-1', plan), {
      initialProps: { plan: steps },
    })
    expect(result.current.steps).toBe(steps)
    rerender({ plan: steps.map((step) => ({ ...step, status: 'done' })) })
    expect(result.current.steps).toBeNull()
  })

  it('queues only the messages still waiting to be sent', () => {
    const { result } = renderHook(() => useTopTray('chat-1', undefined))
    act(
      () =>
        void chatStore.setState({
          outbox: {
            'chat-1': [item('q', 'queued'), item('s', 'sending'), item('f', 'failed')],
          },
        }),
    )
    expect(result.current.queued.map((i) => i.requestId)).toEqual(['q'])
    expect(result.current.failed?.requestId).toBe('f')
  })

  it('holds the decided asks since the last message until the next one is typed or sent', () => {
    const l = ledger()
    const { result } = renderHook(() => useTopTray('chat-1', undefined))
    push(l.prompt('r1', 'go'))
    push(l.start('turn-1', 'r1'))
    push(l.ask('a1'))
    expect(result.current.resolved).toEqual([])
    push(l.answer('a1', 'y'))
    expect(result.current.resolved.map((choice) => choice.id)).toEqual(['a1'])
    act(() => void chatStore.setState({ outbox: { 'chat-1': [item('q', 'queued')] } }))
    expect(result.current.resolved).toEqual([])
    act(() => void chatStore.setState({ outbox: {} }))
    expect(result.current.resolved).toHaveLength(1)
    push(l.prompt('r2', 'and then?'))
    expect(result.current.resolved).toEqual([])
  })

  it('holds the decided asks of tool calls, which the transcript draws on the call and not as a row', () => {
    const l = ledger()
    const { result } = renderHook(() => useTopTray('chat-1', undefined))
    push(l.prompt('r1', 'go'))
    push(l.start('turn-1', 'r1'))
    push(l.tool('turn-1', 't1', 'pre', 'Bash'))
    push(
      l.row(
        'ask',
        {
          ...{ askId: 'a1', event: 'permission', kind: 'tool_permission', toolName: 'Bash' },
          ...{ toolId: 't1', answerable: true, options: [{ id: 'y', kind: 'allow' }] },
        },
        { turnId: 'turn-1' },
      ),
    )
    push(l.answer('a1', 'y'))
    expect(result.current.resolved.map((choice) => choice.id)).toEqual(['a1'])
    push(l.prompt('r2', 'and then?'))
    expect(result.current.resolved).toEqual([])
  })
})
