import { act, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { LedgerRow } from '@/features/agent/api/chat-rows'
import { SubagentShelf } from '@/features/agent/activity/subagent-shelf'
import { applyFeedFrame, chatStore, openFeed } from '@/features/agent/stores/chat-store'
import { ledger } from '@/__tests__/__fixtures__/ledger-rows'

const push = (row: LedgerRow) => act(() => void applyFeedFrame('chat-1', { type: 'row', row }))

beforeEach(() => {
  chatStore.setState({ feeds: {} })
  openFeed('chat-1')
  applyFeedFrame('chat-1', { type: 'ready', epoch: 'e1' })
})
afterEach(() => vi.useRealTimers())

describe('SubagentShelf', () => {
  it('renders nothing when nothing is running', () => {
    const { container } = render(<SubagentShelf chatId="chat-1" />)
    expect(container).toBeEmptyDOMElement()
  })

  it('lists each running subagent by title, current action and clock', () => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'Date'] })
    const l = ledger()
    vi.setSystemTime(new Date('2026-10-01T12:00:30Z'))
    render(<SubagentShelf chatId="chat-1" />)
    push(l.start('turn-1'))
    push(
      l.row(
        'tool_call',
        { toolId: 'c', phase: 'pre', name: 'Agent', kind: 'subagent', title: 'Scout' },
        { turnId: 'turn-1' },
      ),
    )
    push(
      l.row('subagent', { phase: 'bound', spawnToolId: 'c' }, { turnId: 'turn-1', scope: 'sub-1' }),
    )
    push(
      l.row(
        'tool_call',
        { toolId: 'r', phase: 'pre', kind: 'read', target: '/a/b' },
        { turnId: 'turn-1', scope: 'sub-1' },
      ),
    )
    push(l.row('subagent', { phase: 'start', type: 'plan' }, { turnId: 'turn-1', scope: 'sub-2' }))
    const shelf = screen.getByTestId('agent-subagent-shelf')
    expect(shelf).toHaveAccessibleName('2 subagents running')
    expect(shelf).toHaveTextContent('Scout')
    expect(shelf).toHaveTextContent('Reading a/b')
    const clock = screen.getAllByTestId('agent-subagent-clock')[0]!
    const before = clock.textContent
    act(() => void vi.advanceTimersByTime(1000))
    expect(clock.textContent).not.toBe(before)
    push(l.row('subagent', { phase: 'stop' }, { turnId: 'turn-1', scope: 'sub-1' }))
    expect(shelf).toHaveAccessibleName('1 subagent running')
  })
})
