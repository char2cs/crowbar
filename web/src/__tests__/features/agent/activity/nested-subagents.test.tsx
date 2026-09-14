import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import type { AgentActivity, AgentSubagent, AgentToolCall } from '@/features/agent/api/agent-api'
import {
  finishedNestedSubagents,
  NestedSubagentPanel,
} from '@/features/agent/activity/nested-subagents'
import { NO_ACTIVITY } from '@/features/agent/lib/agent-activity'

function subagent(overrides: Partial<AgentSubagent> = {}): AgentSubagent {
  return {
    id: 'a1',
    turnId: '',
    seq: 1,
    startedAt: '2026-08-17T12:00:00Z',
    endedAt: '2026-08-17T12:00:05Z',
    ...overrides,
  }
}

function tool(overrides: Partial<AgentToolCall> = {}): AgentToolCall {
  return {
    id: 't1',
    turnId: '',
    seq: 1,
    name: 'Bash',
    status: 'ok',
    hasRequest: false,
    hasResult: false,
    startedAt: '2026-08-17T12:00:01Z',
    endedAt: '2026-08-17T12:00:02Z',
    ...overrides,
  }
}

function activity(overrides: Partial<AgentActivity> = {}): AgentActivity {
  return { ...NO_ACTIVITY, ...overrides }
}

describe('finishedNestedSubagents', () => {
  it('keeps only ended subagents with no top-level turn of their own', () => {
    const got = finishedNestedSubagents(
      activity({
        subagents: [
          subagent({ id: 'nested-done', turnId: '' }),
          subagent({ id: 'nested-running', turnId: '', endedAt: undefined }),
          subagent({ id: 'claude-native', turnId: 'turn-1' }),
        ],
      }),
    )

    expect(got.map((s) => s.id)).toEqual(['nested-done'])
  })

  it('sorts by seq', () => {
    const got = finishedNestedSubagents(
      activity({
        subagents: [
          subagent({ id: 'second', turnId: '', seq: 2 }),
          subagent({ id: 'first', turnId: '', seq: 1 }),
        ],
      }),
    )

    expect(got.map((s) => s.id)).toEqual(['first', 'second'])
  })
})

describe('NestedSubagentPanel', () => {
  it('renders nothing when there are no finished nested subagents', () => {
    const { container } = render(
      <NestedSubagentPanel activity={activity({ subagents: [subagent({ turnId: 'turn-1' })] })} />,
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('renders a row for each finished nested subagent', () => {
    render(
      <NestedSubagentPanel
        activity={activity({
          subagents: [subagent({ id: 'a1', turnId: '', agentType: 'general-purpose' })],
        })}
      />,
    )
    expect(screen.getByTestId('agent-nested-subagents')).toBeInTheDocument()
    expect(screen.getByText('Subagent · general-purpose')).toBeInTheDocument()
  })

  it("expanding a row reveals the subagent's own tool calls and reply", () => {
    render(
      <NestedSubagentPanel
        activity={activity({
          subagents: [
            subagent({
              id: 'a1',
              turnId: '',
              messages: [{ text: 'done', at: '2026-08-17T12:00:05Z' }],
            }),
          ],
          toolCalls: [tool({ id: 'child-1', subagentId: 'a1', name: 'Bash' })],
        })}
      />,
    )

    expect(screen.queryByText('done')).not.toBeInTheDocument()
    fireEvent.click(screen.getByText('Subagent'))

    expect(screen.getByTestId('agent-nested-subagent-tools')).toBeInTheDocument()
    expect(screen.getByText('done')).toBeInTheDocument()
  })

  it("never shows another subagent's tool calls under this one's row", () => {
    render(
      <NestedSubagentPanel
        activity={activity({
          subagents: [
            subagent({ id: 'a1', turnId: '' }),
            subagent({ id: 'a2', turnId: '', endedAt: undefined }),
          ],
          toolCalls: [
            tool({ id: 'a1-tool', subagentId: 'a1', name: 'Bash' }),
            tool({ id: 'a2-tool', subagentId: 'a2', name: 'Read' }),
          ],
        })}
      />,
    )

    fireEvent.click(screen.getByText('Subagent'))
    expect(screen.getByText('Bash')).toBeInTheDocument()
    expect(screen.queryByText('Read')).not.toBeInTheDocument()
  })

  it('a subagent with neither tool calls nor a reply is not expandable', () => {
    render(<NestedSubagentPanel activity={activity({ subagents: [subagent({ turnId: '' })] })} />)

    const row = screen.getByText('Subagent').closest('li')
    expect(row?.querySelector('details')).toBeNull()
  })
})
