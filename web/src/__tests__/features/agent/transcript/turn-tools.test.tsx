import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { AgentToolCall } from '@/features/agent/api/agent-api'
import { AgentTurnTools, groupToolCallsByTurn } from '@/features/agent/transcript/turn-tools'

function tool(overrides: Partial<AgentToolCall> = {}): AgentToolCall {
  return {
    id: 't1',
    turnId: 'turn-1',
    seq: 1,
    name: 'Bash',
    status: 'ok',
    hasRequest: false,
    hasResult: false,
    startedAt: '2026-08-17T12:00:00Z',
    ...overrides,
  }
}

describe('groupToolCallsByTurn', () => {
  it('groups finished calls by turn, sorted by seq, excluding running calls', () => {
    const calls = [
      tool({ id: 'a', turnId: 't1', seq: 2 }),
      tool({ id: 'b', turnId: 't1', seq: 1 }),
      tool({ id: 'c', turnId: 't2', seq: 1, status: 'running' }),
      tool({ id: 'd', turnId: 't2', seq: 2 }),
    ]
    const grouped = groupToolCallsByTurn(calls)
    expect(grouped.get('t1')?.map((c) => c.id)).toEqual(['b', 'a'])
    expect(grouped.get('t2')?.map((c) => c.id)).toEqual(['d'])
  })

  it('returns an empty map for no calls', () => {
    expect(groupToolCallsByTurn([]).size).toBe(0)
  })
})

describe('AgentTurnTools', () => {
  it('shows the finished work a reply is built on', () => {
    render(
      <AgentTurnTools
        turnId="turn-1"
        callsByTurn={groupToolCallsByTurn([
          tool({ name: 'Grep', target: 'x.ts', durationMs: 1200 }),
        ])}
      />,
    )
    expect(screen.getByText('Grep · x.ts')).toBeInTheDocument()
    expect(screen.getByText('1.2s')).toBeInTheDocument()
  })

  it('shows only the tools of ITS turn', () => {
    render(
      <AgentTurnTools
        turnId="turn-1"
        callsByTurn={groupToolCallsByTurn([
          tool({ id: 'a', name: 'Mine' }),
          tool({ id: 'b', turnId: 'turn-2', name: 'Theirs' }),
        ])}
      />,
    )
    expect(screen.getByText('Mine')).toBeInTheDocument()
    expect(screen.queryByText('Theirs')).not.toBeInTheDocument()
  })

  it('renders nothing for a reply with no tools, and omits still-running ones', () => {
    const { container } = render(
      <AgentTurnTools
        turnId="turn-1"
        callsByTurn={groupToolCallsByTurn([tool({ status: 'running' })])}
      />,
    )
    expect(container).toBeEmptyDOMElement()
  })

  // A reply built on a failed tool must not read as clean.
  it('marks a failed tool', () => {
    render(
      <AgentTurnTools
        turnId="turn-1"
        callsByTurn={groupToolCallsByTurn([tool({ status: 'error', name: 'Bash' })])}
      />,
    )
    expect(screen.getByText('Bash').closest('li')).toHaveAttribute('data-status', 'error')
  })

  it('renders nothing without a turn id — a streaming bubble has no turn yet', () => {
    const { container } = render(
      <AgentTurnTools turnId="" callsByTurn={groupToolCallsByTurn([tool()])} />,
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing when the turn has no entry in the map', () => {
    const { container } = render(<AgentTurnTools callsByTurn={new Map()} turnId="t1" />)
    expect(container).toBeEmptyDOMElement()
  })
})
