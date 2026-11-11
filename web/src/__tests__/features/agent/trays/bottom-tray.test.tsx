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
    expect(clock).toHaveTextContent(/^\d+s$/)
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

  it('shows a running shell and a finished monitor, each in its own group with no clock once done', () => {
    render(<BottomTray chatId="chat-1" />)
    const l = startSubagents(1)
    push(
      l.row(
        'shell',
        { phase: 'start', shellId: 'sh-1', command: 'npm run dev', label: 'Dev server' },
        { turnId: 'turn-1' },
      ),
    )
    push(
      l.row(
        'shell',
        { phase: 'start', shellId: 'mon-1', kind: 'monitor', command: 'tail -f app.log' },
        { turnId: 'turn-1' },
      ),
    )
    push(
      l.row(
        'shell',
        { phase: 'end', shellId: 'mon-1', status: 'failed', exitCode: 1 },
        { turnId: 'turn-1' },
      ),
    )

    expect(screen.getByText('Subagents')).toBeInTheDocument()
    expect(screen.getByText('Shells')).toBeInTheDocument()
    expect(screen.getByText('Monitors')).toBeInTheDocument()
    const monitor = screen.getByText('tail -f app.log').closest('li') as HTMLElement
    expect(monitor).toHaveAttribute('data-kind', 'monitor')
    expect(monitor).toHaveAttribute('data-status', 'failed')
    expect(monitor).toHaveTextContent('Exit 1')
    expect(within(monitor).queryByTestId('agent-tray-clock')).toBeNull()
    expect(screen.getByText('Dev server').closest('li')).toHaveAttribute('data-status', 'running')
  })

  describe('ending a shell', () => {
    function startShells() {
      const l = ledger()
      push(l.start('turn-1'))
      const start = (body: object) =>
        push(l.row('shell', { phase: 'start', ...body } as never, { turnId: 'turn-1' }))
      start({ shellId: 'sh-1', command: 'npm run dev', label: 'Dev server' })
      start({ shellId: 'mon-1', kind: 'monitor', command: 'tail -f app.log' })
      return l
    }

    it('offers an End control on each running shell and monitor that asks for that one', () => {
      const onStop = vi.fn()
      render(<BottomTray chatId="chat-1" onStopShell={onStop} />)
      startShells()

      fireEvent.click(screen.getByRole('button', { name: 'End this monitor' }))
      expect(onStop).toHaveBeenCalledWith('mon-1')
      fireEvent.click(screen.getByRole('button', { name: 'End this shell' }))
      expect(onStop).toHaveBeenLastCalledWith('sh-1')
    })

    it('offers none where the provider cannot be asked, nor on a subagent', () => {
      render(<BottomTray chatId="chat-1" />)
      startShells()
      push(
        ledger().row(
          'subagent',
          { phase: 'start', type: 'agent-1' },
          { turnId: 'turn-1', scope: 's1' },
        ),
      )
      expect(screen.queryByRole('button', { name: /End this/ })).toBeNull()
    })

    it('says Stopping until the shell really ends, and never closes it itself', () => {
      render(<BottomTray chatId="chat-1" onStopShell={vi.fn()} />)
      const l = startShells()

      push(l.row('shell', { phase: 'stopping', shellId: 'sh-1' } as never, { turnId: 'turn-1' }))
      const shell = screen.getByText('Dev server').closest('li') as HTMLElement
      expect(shell).toHaveTextContent('Stopping…')
      expect(shell).toHaveAttribute('data-status', 'running')
      expect(within(shell).queryByRole('button')).toBeNull()

      push(
        l.row('shell', { phase: 'end', shellId: 'sh-1', status: 'stopped' }, { turnId: 'turn-1' }),
      )
      expect(screen.getByText('Dev server').closest('li')).toHaveAttribute('data-status', 'stopped')
    })
  })
})
