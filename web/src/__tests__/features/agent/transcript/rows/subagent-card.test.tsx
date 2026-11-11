import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import type { LedgerRow } from '@/features/agent/api/chat-rows'
import { applyFeedFrame, chatStore, openFeed } from '@/features/agent/stores/chat-store'
import { SubagentCard } from '@/features/agent/transcript/rows/subagent-card'
import { ledger, subagentSessionRows } from '@/__tests__/__fixtures__/ledger-rows'

function seed(chatId: string, rows: LedgerRow[]) {
  openFeed(chatId)
  for (const row of rows) applyFeedFrame(chatId, { type: 'row', row })
  applyFeedFrame(chatId, { type: 'ready', epoch: 'e1' })
}

const push = (row: LedgerRow) => act(() => void applyFeedFrame('chat-1', { type: 'row', row }))

beforeEach(() => chatStore.setState({ feeds: {} }))

describe('SubagentCard', () => {
  it('symptom 8: a subagent card shows its child tool rows and its result', () => {
    const rows = subagentSessionRows()
    seed('id-1', rows)
    const spawn = rows.find((row) => row.kind === 'tool_call')!.body as { toolId: string }
    render(<SubagentCard chatId="id-1" id={`s:${spawn.toolId}`} />)
    const card = screen.getByTestId('agent-subagent-card')
    expect(card).toHaveAttribute('data-status', 'done')
    expect(card).toHaveTextContent('Read a.txt and echo')
    fireEvent.click(within(card).getByText('Read a.txt and echo'))
    const tools = within(card).getAllByTestId('agent-activity-tool')
    expect(tools.map((tool) => tool.getAttribute('aria-label'))).toEqual([
      'Read a.txt',
      'Ran Print hi',
    ])
    expect(within(card).getByTestId('agent-subagent-result')).toHaveTextContent(
      'a.txt contains "alpha" (one line)',
    )
  })

  it('toggles from anywhere on its header, one button named by the subagent', () => {
    const rows = subagentSessionRows()
    seed('id-1', rows)
    const spawn = rows.find((row) => row.kind === 'tool_call')!.body as { toolId: string }
    render(<SubagentCard chatId="id-1" id={`s:${spawn.toolId}`} />)
    const card = screen.getByTestId('agent-subagent-card')
    const head = within(card).getByRole('button', { name: /Read a\.txt and echo/ })
    expect(within(card).getAllByRole('button')).toEqual([head])
    expect(head).toHaveAttribute('aria-expanded', 'false')
    fireEvent.click(head)
    expect(head).toHaveAttribute('aria-expanded', 'true')
    fireEvent.click(head)
    expect(within(card).queryByTestId('agent-subagent-result')).toBeNull()
  })

  it('derives the current action from the newest running child', () => {
    const l = ledger()
    const call = (toolId: string, phase: 'pre' | 'post', kind: string, target: string) =>
      l.row(
        'tool_call',
        { toolId, phase, name: kind, kind, target },
        { turnId: 'turn-1', scope: 'sub-1' },
      )
    seed('chat-1', [
      l.start('turn-1'),
      l.row(
        'tool_call',
        { toolId: 'c', phase: 'pre', name: 'Agent', kind: 'subagent', title: 'Scout' },
        { turnId: 'turn-1' },
      ),
      l.row('subagent', { phase: 'bound', spawnToolId: 'c' }, { turnId: 'turn-1', scope: 'sub-1' }),
    ])
    render(<SubagentCard chatId="chat-1" id="s:c" />)
    const action = () => screen.getByTestId('agent-subagent-action')
    expect(action()).toHaveTextContent('Thinking…')
    push(call('a', 'pre', 'read', '/repo/Makefile'))
    expect(action()).toHaveTextContent('Reading /repo/Makefile')
    push(call('b', 'pre', 'search', 'test:'))
    expect(action()).toHaveTextContent('Searching for "test:"')
    push(call('b', 'post', 'search', 'test:'))
    expect(action()).toHaveTextContent('Reading /repo/Makefile')
    push(call('a', 'post', 'read', '/repo/Makefile'))
    expect(action()).toHaveTextContent('Searched "test:"')
  })

  it('shows Parent waiting while the parent addresses the subagent', () => {
    const l = ledger()
    const wait = (phase: 'pre' | 'post') =>
      l.row(
        'tool_call',
        { toolId: 'w', phase, name: 'wait', addressesScope: 'sub-1' },
        { turnId: 'turn-1' },
      )
    seed('chat-1', [
      l.start('turn-1'),
      l.row(
        'tool_call',
        { toolId: 'c', phase: 'pre', name: 'Agent', kind: 'subagent' },
        { turnId: 'turn-1' },
      ),
      l.row('subagent', { phase: 'bound', spawnToolId: 'c' }, { turnId: 'turn-1', scope: 'sub-1' }),
    ])
    render(<SubagentCard chatId="chat-1" id="s:c" />)
    expect(screen.queryByText('Parent waiting…')).toBeNull()
    push(wait('pre'))
    expect(screen.getByText('Parent waiting…')).toBeInTheDocument()
    push(wait('post'))
    expect(screen.queryByText('Parent waiting…')).toBeNull()
  })

  it('keeps the tool uses and tokens it spent on hover, not on the line, once done', () => {
    const l = ledger()
    seed('chat-1', [
      l.start('turn-1'),
      l.row(
        'tool_call',
        { toolId: 'c', phase: 'pre', name: 'Agent', kind: 'subagent', title: 'Scout' },
        { turnId: 'turn-1' },
      ),
      l.row(
        'subagent',
        { phase: 'bound', spawnToolId: 'c', type: 'explore' },
        { turnId: 'turn-1', scope: 'sub-1' },
      ),
      l.row(
        'subagent',
        { phase: 'stop', usage: { tokens: 28_400, toolUses: 6 } },
        { turnId: 'turn-1', scope: 'sub-1' },
      ),
    ])
    render(<SubagentCard chatId="chat-1" id="s:c" />)
    expect(screen.getByTestId('agent-subagent-card')).toHaveTextContent('explore')
    expect(screen.getByTestId('agent-subagent-card')).not.toHaveTextContent('tokens')
    expect(screen.getByTitle('6 tool uses · 28k tokens')).toBeInTheDocument()
    expect(screen.getByTestId('agent-subagent-card').querySelector('.tool-glyph')).not.toBeNull()
  })

  it('reads a done subagent quietly and a stopped one with its own outcome', () => {
    const l = ledger()
    seed('chat-1', [
      l.start('turn-1'),
      l.row(
        'tool_call',
        { toolId: 'c', phase: 'pre', name: 'Agent', kind: 'subagent', title: 'Scout' },
        { turnId: 'turn-1' },
      ),
      l.row('subagent', { phase: 'bound', spawnToolId: 'c' }, { turnId: 'turn-1', scope: 'sub-1' }),
    ])
    const { container } = render(<SubagentCard chatId="chat-1" id="s:c" />)
    expect(container.querySelector('[data-flicker-spinner]')).not.toBeNull()
    push(l.row('runner_exited', { runnerId: 'r1', reason: 'stopped', exitCode: 0 }))
    expect(container.querySelector('[data-flicker-spinner]')).toBeNull()
    expect(container.querySelector('[data-outcome="stopped"]')).toHaveTextContent('Stopped')
  })
})
