import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import type { AgentActivity, AgentSubagent } from '@/features/agent/api/agent-api'
import { SubagentShelf } from '@/features/agent/activity/subagent-shelf'
import { NO_ACTIVITY } from '@/features/agent/lib/agent-activity'

function subagent(overrides: Partial<AgentSubagent> = {}): AgentSubagent {
  return {
    id: 'a1',
    turnId: '',
    seq: 1,
    startedAt: '2026-08-17T12:00:00Z',
    ...overrides,
  }
}

function activity(overrides: Partial<AgentActivity> = {}): AgentActivity {
  return { ...NO_ACTIVITY, ...overrides }
}

describe('SubagentShelf', () => {
  it('renders nothing when nothing is running', () => {
    const { container } = render(<SubagentShelf activity={activity()} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders the running strip when a subagent is running', () => {
    render(<SubagentShelf activity={activity({ subagents: [subagent({ endedAt: undefined })] })} />)
    expect(screen.getByTestId('agent-subagent-shelf')).toBeInTheDocument()
  })

  // A finished subagent — with or without a turn of its own — is drawn by
  // AgentTurnSubagents in the transcript now, not here. The shelf is the
  // live strip only: it must go back to returning null the moment nothing is
  // still running, whatever finished earlier in the conversation.
  it('renders nothing once a subagent finishes and nothing else is running', () => {
    const { container } = render(
      <SubagentShelf
        activity={activity({ subagents: [subagent({ endedAt: '2026-08-17T12:00:05Z' })] })}
      />,
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('shows only the still-running ones when a mix of running and finished exist', () => {
    render(
      <SubagentShelf
        activity={activity({
          subagents: [
            subagent({ id: 'running', endedAt: undefined }),
            subagent({ id: 'done', endedAt: '2026-08-17T12:00:05Z' }),
          ],
        })}
      />,
    )
    const shelf = screen.getByTestId('agent-subagent-shelf')
    expect(shelf).toHaveAccessibleName('1 subagent running')
  })
})
