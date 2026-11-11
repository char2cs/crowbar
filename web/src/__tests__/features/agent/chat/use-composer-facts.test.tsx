import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import type { LedgerRow } from '@/features/agent/api/chat-rows'
import { useComposerFacts, useWorkingLineFacts } from '@/features/agent/chat/use-composer-facts'
import { applyFeedFrame, chatStore, openFeed } from '@/features/agent/stores/chat-store'
import { delta, ledger } from '@/__tests__/__fixtures__/ledger-rows'

const push = (row: LedgerRow) => act(() => void applyFeedFrame('chat-1', { type: 'row', row }))

beforeEach(() => {
  chatStore.setState({ feeds: {}, outbox: {} })
  openFeed('chat-1')
  applyFeedFrame('chat-1', { type: 'ready', epoch: 'e1' })
})

describe('useComposerFacts', () => {
  it('counts the clock from Enter before the turn starts', () => {
    const { result } = renderHook(() => ({
      ...useWorkingLineFacts('chat-1'),
      view: useComposerFacts('chat-1'),
    }))
    expect(result.current.working).toBe(false)
    act(() =>
      chatStore.setState({
        outbox: {
          'chat-1': [
            {
              requestId: 'r1',
              wsId: 'ws-1',
              text: 'hi',
              state: 'sending',
              steer: false,
              enteredAt: '2026-10-01T12:00:00Z',
            },
          ],
        },
      }),
    )
    expect(result.current.working).toBe(true)
    expect(result.current.view.working).toBe(false)
    expect(result.current.since).toBe('2026-10-01T12:00:00Z')
    const l = ledger()
    push(l.prompt('r1', 'hi'))
    push(l.start('turn-1', 'r1'))
    expect(result.current.since).toBe(
      chatStore.getState().feeds['chat-1']!.facts.running!.startedAt,
    )
  })

  it('keeps the line while the daemon holds the prompt for a CLI still starting', () => {
    const { result } = renderHook(() => ({
      ...useWorkingLineFacts('chat-1'),
      view: useComposerFacts('chat-1'),
    }))
    const l = ledger()
    const row = l.prompt('r1', 'hi')
    push(row)
    expect(result.current.working).toBe(true)
    expect(result.current.view.working).toBe(false)
    expect(result.current.since).toBe(row.at)
    push(l.start('turn-1', 'r1'))
    push(l.end('turn-1'))
    expect(result.current.working).toBe(false)
  })

  it('names the active todo, the running main tool and the agents the turn waits on', () => {
    const { result } = renderHook(() => ({
      ...useWorkingLineFacts('chat-1'),
      view: useComposerFacts('chat-1'),
    }))
    const l = ledger()
    push(l.start('turn-1'))
    push(
      l.row(
        'plan',
        { op: 'replace', items: [{ text: 'Survey docs', status: 'active' }] },
        { turnId: 'turn-1' },
      ),
    )
    push(
      l.row(
        'tool_call',
        { toolId: 'x', phase: 'pre', kind: 'execute', target: 'npm test' },
        { turnId: 'turn-1' },
      ),
    )
    push(
      l.row('subagent', { phase: 'start', type: 'explore' }, { turnId: 'turn-1', scope: 'sub-1' }),
    )
    expect(result.current.activeTodo).toBe('Survey docs')
    expect(result.current.toolLabel).toBe('Running npm test')
    expect(result.current.agents).toBe(1)
    push(
      l.row(
        'tool_call',
        { toolId: 'x', phase: 'post', kind: 'execute', target: 'npm test' },
        { turnId: 'turn-1' },
      ),
    )
    expect(result.current.toolLabel).toBeUndefined()
  })

  it('reads a reply streaming ahead of its rows as streaming', () => {
    const { result } = renderHook(() => ({
      ...useWorkingLineFacts('chat-1'),
      view: useComposerFacts('chat-1'),
    }))
    const l = ledger()
    push(l.start('turn-1'))
    expect(result.current.streaming).toBe(false)
    act(() => void applyFeedFrame('chat-1', { type: 'delta', delta: delta('m1', 1, 'Hel') }))
    expect(result.current.streaming).toBe(true)
  })

  it('renders nothing for rows that change none of its facts', () => {
    let renders = 0
    renderHook(() => {
      renders += 1
      return useComposerFacts('chat-1')
    })
    const l = ledger()
    push(l.start('turn-1'))
    push(
      l.row('subagent', { phase: 'start', type: 'explore' }, { turnId: 'turn-1', scope: 'sub-1' }),
    )
    const before = renders
    for (let i = 0; i < 10; i++) {
      push(
        l.row(
          'tool_call',
          { toolId: `read-${i}`, phase: 'pre', kind: 'read', target: 'a.ts' },
          { turnId: 'turn-1', scope: 'sub-1' },
        ),
      )
    }
    expect(renders).toBe(before)
  })

  // The view renders the whole chat: a tool's label or a token is the working line's.
  it('renders the view nothing for the tools and text of a running turn', () => {
    let renders = 0
    renderHook(() => {
      renders += 1
      return useComposerFacts('chat-1')
    })
    const l = ledger()
    push(l.prompt('r1', 'go'))
    push(l.start('turn-1', 'r1'))
    const before = renders
    for (let i = 0; i < 5; i++) {
      push(l.tool('turn-1', `t-${i}`, 'pre'))
      push(l.tool('turn-1', `t-${i}`, 'post'))
      push(l.text('turn-1', `m-${i}`, 0, 'step', true))
    }
    act(() => void applyFeedFrame('chat-1', { type: 'delta', delta: delta('m9', 1, 'Hel') }))
    expect(renders).toBe(before)
  })
})
