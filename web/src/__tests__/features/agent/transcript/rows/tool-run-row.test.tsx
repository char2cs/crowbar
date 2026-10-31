import { act, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import type { LedgerRow } from '@/features/agent/api/chat-rows'
import { applyFeedFrame, chatStore, openFeed } from '@/features/agent/stores/chat-store'
import { ToolRunRow } from '@/features/agent/transcript/rows/tool-run-row'
import { ledger } from '@/__tests__/__fixtures__/ledger-rows'

function push(row: LedgerRow) {
  act(() => void applyFeedFrame('chat-1', { type: 'row', row }))
}

beforeEach(() => {
  chatStore.setState({ feeds: {} })
  openFeed('chat-1')
  applyFeedFrame('chat-1', { type: 'ready', epoch: 'e1' })
})

describe('ToolRunRow', () => {
  it('names the newest call while live and summarizes the run once settled', () => {
    const l = ledger()
    const read = (toolId: string, phase: 'pre' | 'post', target: string) =>
      l.row(
        'tool_call',
        { toolId, phase, name: 'Read', kind: 'read', target },
        { turnId: 'turn-1' },
      )
    push(l.start('turn-1'))
    push(read('a', 'pre', '/repo/a.ts'))
    push(
      l.row(
        'tool_call',
        { toolId: 'b', phase: 'pre', name: 'Grep', kind: 'search', target: 'TODO' },
        { turnId: 'turn-1' },
      ),
    )
    render(<ToolRunRow chatId="chat-1" id="g:t:a" members={['t:a', 't:b']} />)
    const run = screen.getByTestId('agent-tool-run')
    expect(run).toHaveAttribute('data-status', 'running')
    expect(run).toHaveTextContent('Reading 1 file, searching 1 pattern…')
    expect(screen.getByTestId('agent-tool-run-ticker')).toHaveTextContent('Searching for "TODO"')

    push(read('a', 'post', '/repo/a.ts'))
    push(
      l.row(
        'tool_call',
        { toolId: 'b', phase: 'post', name: 'Grep', kind: 'search', target: 'TODO' },
        { turnId: 'turn-1' },
      ),
    )
    expect(run).toHaveAttribute('data-status', 'ok')
    expect(run).toHaveTextContent('Read 1 file, searched 1 pattern')
    expect(screen.queryByTestId('agent-tool-run-ticker')).toBeNull()
    expect(screen.queryByTestId('agent-activity-tool')).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'Show the run’s tool calls' }))
    expect(screen.getAllByTestId('agent-activity-tool')).toHaveLength(2)
  })

  it('draws a run of one call as that call’s own row', () => {
    const l = ledger()
    push(
      l.row(
        'tool_call',
        { toolId: 'a', phase: 'post', name: 'Read', kind: 'read', target: '/a' },
        { turnId: 'turn-1' },
      ),
    )
    render(<ToolRunRow chatId="chat-1" id="g:t:a" members={['t:a']} />)
    expect(screen.queryByTestId('agent-tool-run')).toBeNull()
    expect(screen.getByTestId('agent-activity-tool')).toHaveAttribute('data-tool-id', 'a')
  })
})
