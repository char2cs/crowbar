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
  it('renders nothing when nothing is running and nothing finished is nested', () => {
    const { container } = render(<SubagentShelf activity={activity()} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('still renders the running strip exactly as before when a subagent is running', () => {
    render(<SubagentShelf activity={activity({ subagents: [subagent({ endedAt: undefined })] })} />)
    expect(screen.getByTestId('agent-subagent-shelf')).toBeInTheDocument()
  })

  // A codex-shaped nested subagent has NOTHING running — its own tool calls
  // and reply are already durable — yet the shelf used to return null the
  // instant `running.length === 0`, so its finished record had nowhere to
  // surface at all. This is the regression NestedSubagentPanel exists to fix.
  it('renders the finished nested list even when nothing is currently running', () => {
    render(
      <SubagentShelf
        activity={activity({
          subagents: [subagent({ endedAt: '2026-08-17T12:00:05Z' })],
        })}
      />,
    )
    expect(screen.queryByTestId('agent-subagent-shelf')).not.toBeInTheDocument()
    expect(screen.getByTestId('agent-nested-subagents')).toBeInTheDocument()
  })

  it('renders both the running strip and the finished nested list together', () => {
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
    expect(screen.getByTestId('agent-subagent-shelf')).toBeInTheDocument()
    expect(screen.getByTestId('agent-nested-subagents')).toBeInTheDocument()
  })

  // A CLAUDE-native subagent (real turnId) that finished must still be left
  // to AgentTurnSubagents — NestedSubagentPanel is only for one with none.
  it('never claims a finished subagent that belongs to a real turn', () => {
    render(
      <SubagentShelf
        activity={activity({
          subagents: [subagent({ turnId: 'turn-1', endedAt: '2026-08-17T12:00:05Z' })],
        })}
      />,
    )
    expect(screen.queryByTestId('agent-nested-subagents')).not.toBeInTheDocument()
  })
})
