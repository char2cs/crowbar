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

describe('ToolRunRow with an ask on a member', () => {
  const fetches = (l: ReturnType<typeof ledger>) => [
    l.start('turn-1'),
    ...['f1', 'f2'].map((toolId) =>
      l.row(
        'tool_call',
        { toolId, phase: 'pre', name: 'WebFetch', kind: 'fetch', target: `https://x/${toolId}` },
        { turnId: 'turn-1' },
      ),
    ),
    l.row(
      'ask',
      {
        askId: 'q1',
        event: 'permission',
        kind: 'tool_permission',
        toolId: 'f2',
        answerable: true,
        options: [
          { id: 'ok', kind: 'allow', label: 'Allow' },
          { id: 'no', kind: 'deny', label: 'Deny' },
        ],
      },
      { turnId: 'turn-1' },
    ),
  ]
  const deny = (l: ReturnType<typeof ledger>) =>
    l.row('ask_resolved', { askId: 'q1', resolution: 'answered', optionIds: ['no'] })
  const draw = () => render(<ToolRunRow chatId="chat-1" id="g:t:f1" members={['t:f1', 't:f2']} />)
  const statusOf = (toolId: string) =>
    document.querySelector(`[data-tool-id="${toolId}"] li`)?.getAttribute('data-status')

  it('shows the call waiting on approval as its own row, then its denial', () => {
    const l = ledger()
    for (const row of fetches(l)) push(row)
    draw()
    expect(statusOf('f2')).toBe('awaiting_approval')
    expect(screen.getByText('Waiting for your approval')).toBeInTheDocument()

    push(deny(l))
    expect(statusOf('f2')).toBe('declined')
    expect(screen.getByTestId('agent-tool-ask-note')).toHaveTextContent('Denied')
    expect(screen.queryByTestId('agent-tool-run')).toBeNull()
  })

  it('reads the same after a reload', () => {
    const l = ledger()
    chatStore.setState({ feeds: {} })
    openFeed('chat-1')
    for (const row of [...fetches(l), deny(l)]) applyFeedFrame('chat-1', { type: 'row', row })
    applyFeedFrame('chat-1', { type: 'ready', epoch: 'e2' })
    draw()
    expect(statusOf('f2')).toBe('declined')
    expect(screen.getByTestId('agent-tool-ask-note')).toHaveTextContent('Denied')
    expect(screen.queryByTestId('agent-tool-run')).toBeNull()
  })
})
