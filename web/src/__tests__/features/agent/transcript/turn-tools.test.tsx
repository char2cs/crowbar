import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import type { AgentActivity, AgentToolCall } from '@/features/agent/api/agent-api'
import { AgentTurnTools } from '@/features/agent/transcript/turn-tools'
import { NO_ACTIVITY } from '@/features/agent/lib/agent-activity'

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

function activity(toolCalls: AgentToolCall[]): AgentActivity {
  return { ...NO_ACTIVITY, toolCalls }
}

describe('AgentTurnTools', () => {
  it('shows the finished work a reply is built on', () => {
    render(
      <AgentTurnTools
        turnId="turn-1"
        activity={activity([tool({ name: 'Grep', target: 'x.ts', durationMs: 1200 })])}
      />,
    )
    expect(screen.getByText('Grep · x.ts')).toBeInTheDocument()
    expect(screen.getByText('1.2s')).toBeInTheDocument()
  })

  it('shows only the tools of ITS turn', () => {
    render(
      <AgentTurnTools
        turnId="turn-1"
        activity={activity([
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
      <AgentTurnTools turnId="turn-1" activity={activity([tool({ status: 'running' })])} />,
    )
    expect(container).toBeEmptyDOMElement()
  })

  // A reply built on a failed tool must not read as clean.
  it('marks a failed tool', () => {
    render(
      <AgentTurnTools
        turnId="turn-1"
        activity={activity([tool({ status: 'error', name: 'Bash' })])}
      />,
    )
    expect(screen.getByText('Bash').closest('li')).toHaveAttribute('data-status', 'error')
  })

  it('renders nothing without a turn id — a streaming bubble has no turn yet', () => {
    const { container } = render(<AgentTurnTools turnId="" activity={activity([tool()])} />)
    expect(container).toBeEmptyDOMElement()
  })
})
