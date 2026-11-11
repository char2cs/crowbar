import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { LedgerRow } from '@/features/agent/api/chat-rows'
import { applyFeedFrame, chatStore, openFeed } from '@/features/agent/stores/chat-store'
import { BottomTray } from '@/features/agent/trays/bottom-tray'
import { ledger } from '@/__tests__/__fixtures__/ledger-rows'

const push = (row: LedgerRow) => act(() => void applyFeedFrame('chat-1', { type: 'row', row }))

function startSubagents(count: number) {
  const l = ledger()
  push(l.start('turn-1'))
  for (let i = 1; i <= count; i++) {
    push(
      l.row(
        'subagent',
        { phase: 'start', type: `agent-${i}` },
        { turnId: 'turn-1', scope: `s${i}` },
      ),
    )
  }
  return l
}

beforeEach(() => {
  chatStore.setState({ feeds: {} })
  openFeed('chat-1')
  applyFeedFrame('chat-1', { type: 'ready', epoch: 'e1' })
})
afterEach(() => vi.useRealTimers())

describe('BottomTray', () => {
  it('renders nothing when no work is running', () => {
    const { container } = render(<BottomTray chatId="chat-1" />)
    expect(container).toBeEmptyDOMElement()
  })

  it('lists each subagent with its name, action and a running clock, with no group label', () => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'Date'] })
    vi.setSystemTime(new Date('2026-10-01T12:00:30Z'))
    render(<BottomTray chatId="chat-1" />)
    const l = startSubagents(1)
    push(
      l.row(
        'tool_call',
        { toolId: 'r', phase: 'pre', kind: 'read', target: '/a/b' },
        { turnId: 'turn-1', scope: 's1' },
      ),
    )
    const rows = screen.getAllByRole('listitem')
    expect(rows).toHaveLength(1)
    expect(rows[0]).toHaveTextContent('agent-1')
    expect(rows[0]).toHaveTextContent('Reading /a/b')
    expect(screen.queryByText('Subagents')).toBeNull()
    const clock = screen.getByTestId('agent-tray-clock')
    const before = clock.textContent
    act(() => void vi.advanceTimersByTime(1000))
    expect(clock.textContent).not.toBe(before)
  })

  it('folds past four rows into a group row that opens and closes', () => {
    render(<BottomTray chatId="chat-1" />)
    startSubagents(6)
    expect(screen.getAllByRole('listitem')).toHaveLength(4)
    const more = screen.getByRole('button', { name: '3 more · 3 running' })
    expect(more).toHaveAttribute('aria-expanded', 'false')
    fireEvent.click(more)
    expect(screen.getAllByRole('listitem')).toHaveLength(7)
    fireEvent.click(screen.getByRole('button', { name: 'Show fewer' }))
    expect(within(screen.getByTestId('agent-bottom-tray')).getAllByRole('listitem')).toHaveLength(4)
  })
})
