import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import type {
  AgentActivity,
  AgentChoice,
  AgentInterruption,
  AgentToolCall,
} from '@/features/agent/api/agent-api'
import { WorkingLine } from '@/features/agent/activity/working-line'
import { NO_ACTIVITY } from '@/features/agent/lib/agent-activity'

function tool(overrides: Partial<AgentToolCall> = {}): AgentToolCall {
  return {
    id: 't1',
    turnId: 'turn-1',
    seq: 1,
    name: 'Bash',
    status: 'running',
    hasRequest: false,
    hasResult: false,
    startedAt: '2026-08-17T12:00:00Z',
    ...overrides,
  }
}

function activity(overrides: Partial<AgentActivity> = {}): AgentActivity {
  return { ...NO_ACTIVITY, ...overrides }
}

function choice(overrides: Partial<AgentChoice> = {}): AgentChoice {
  return {
    id: 'k1',
    turnId: 'turn-1',
    seq: 1,
    kind: 'tool_permission',
    toolName: 'Bash',
    options: [{ id: 'allow', kind: 'allow', label: 'Allow' }],
    pending: true,
    answerable: true,
    at: '2026-08-18T12:00:00Z',
    ...overrides,
  }
}

function interruption(overrides: Partial<AgentInterruption> = {}): AgentInterruption {
  return {
    id: 'i1',
    turnId: 'turn-1',
    seq: 1,
    kind: 'permission',
    at: '2026-08-18T12:00:00Z',
    ...overrides,
  }
}

describe('WorkingLine', () => {
  it('renders nothing at all when the chat is idle', () => {
    const { container } = render(<WorkingLine activity={NO_ACTIVITY} working={false} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('degrades to the plain working line when a provider reports no activity', () => {
    render(<WorkingLine activity={NO_ACTIVITY} working />)
    expect(screen.getByTestId('agent-activity-strip')).toBeInTheDocument()
    expect(screen.queryByRole('list')).not.toBeInTheDocument()
  })

  it('names the tools that are running right now', () => {
    render(
      <WorkingLine
        working
        activity={activity({
          toolCalls: [
            tool({ id: 't1', name: 'Grep', target: 'engine/**/*.yaml' }),
            tool({ id: 't2', seq: 2, name: 'Read', target: 'protocol.go' }),
          ],
        })}
      />,
    )
    expect(screen.getByText('Grep · engine/**/*.yaml')).toBeInTheDocument()
    expect(screen.getByText('Read · protocol.go')).toBeInTheDocument()
  })

  it('counts the overflow rather than listing an unscannable wall', () => {
    render(
      <WorkingLine
        working
        activity={activity({
          toolCalls: Array.from({ length: 6 }, (_, index) =>
            tool({ id: `t${index}`, seq: index, name: `Tool${index}` }),
          ),
        })}
      />,
    )
    expect(screen.getByText('+3 more')).toBeInTheDocument()
  })

  it('omits a finished call — the working line is what is happening NOW', () => {
    render(
      <WorkingLine
        working
        activity={activity({ toolCalls: [tool({ status: 'ok', name: 'Done' })] })}
      />,
    )
    expect(screen.queryByText(/Done/)).not.toBeInTheDocument()
  })

  // A chat waiting on a person is not working, and the two used to look the same.
  it('says nothing at all while a prompt is open — not "working…", not a second banner', () => {
    const { container } = render(
      <WorkingLine working activity={activity({ choices: [choice()] })} />,
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('stays quiet for a prompt nobody here can answer', () => {
    const { container } = render(
      <WorkingLine working activity={activity({ choices: [choice({ answerable: false })] })} />,
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('says the agent is BLOCKED rather than working', () => {
    const { container } = render(
      <WorkingLine working activity={activity({ interruptions: [interruption()] })} />,
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('returns to the working line once the interruption resolves', () => {
    render(
      <WorkingLine
        working
        activity={activity({
          interruptions: [interruption({ resolvedAt: '2026-08-18T12:00:05Z' })],
        })}
      />,
    )
    expect(screen.getByTestId('agent-activity-strip')).toBeInTheDocument()
  })

  it('goes back to the working line once the prompt is resolved', () => {
    render(<WorkingLine working activity={activity({ choices: [choice({ pending: false })] })} />)
    expect(screen.getByTestId('agent-activity-strip')).toBeInTheDocument()
  })
})
