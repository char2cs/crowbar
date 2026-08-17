import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import type { AgentActivity, AgentToolCall } from '@/features/agent/api/agent-api'
import {
  AgentActivityStrip,
  AgentTurnTools,
} from '@/features/agent/components/agent-activity-strip'
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

describe('AgentActivityStrip', () => {
  it('renders nothing at all when the chat is idle', () => {
    const { container } = render(
      <AgentActivityStrip activity={NO_ACTIVITY} working={false} providerLabel="Claude" />,
    )

    expect(container).toBeEmptyDOMElement()
  })

  // A provider that reports nothing renders exactly the spinner it did before —
  // absent capability is absent UI, never a broken-looking empty panel.
  it('degrades to the plain working line when a provider reports no activity', () => {
    render(<AgentActivityStrip activity={NO_ACTIVITY} working providerLabel="Codex" />)

    expect(screen.getByTestId('agent-activity')).toHaveTextContent('Codex is working…')
    expect(screen.queryByTestId('agent-running-tools')).toBeNull()
    expect(screen.queryByTestId('agent-subagent-count')).toBeNull()
  })

  it('names the tools that are running right now', () => {
    render(
      <AgentActivityStrip
        activity={activity({
          toolCalls: [
            tool({ id: 'a', seq: 1, name: 'Edit', target: 'a.go' }),
            tool({ id: 'b', seq: 2, name: 'Bash', target: 'go test ./...' }),
            tool({ id: 'done', seq: 3, name: 'Read', status: 'ok' }),
          ],
        })}
        working
        providerLabel="Claude"
      />,
    )

    const tools = screen.getByTestId('agent-running-tools')
    expect(tools).toHaveTextContent('Edit · a.go')
    expect(tools).toHaveTextContent('Bash · go test ./...')
    expect(tools).not.toHaveTextContent('Read')
  })

  it('counts the overflow rather than listing an unscannable wall', () => {
    render(
      <AgentActivityStrip
        activity={activity({
          toolCalls: [1, 2, 3, 4, 5].map((n) => tool({ id: `t${n}`, seq: n, name: `Tool${n}` })),
        })}
        working
        providerLabel="Claude"
      />,
    )

    expect(screen.getByTestId('agent-running-tools')).toHaveTextContent('+2 more')
  })

  it('reports live subagents', () => {
    render(
      <AgentActivityStrip
        activity={activity({
          subagents: [
            { id: 'a', turnId: 'turn-1', seq: 1, startedAt: 'x' },
            { id: 'b', turnId: 'turn-1', seq: 2, startedAt: 'x' },
          ],
        })}
        working
        providerLabel="Claude"
      />,
    )

    expect(screen.getByTestId('agent-subagent-count')).toHaveTextContent('2 subagents')
  })

  // The failure this whole surface exists for: a provider blocked on a trust
  // prompt used to render as silence, or worse, as an indefinite spinner.
  it('says the agent is BLOCKED rather than working', () => {
    render(
      <AgentActivityStrip
        activity={activity({
          interruptions: [
            {
              id: 'i1',
              turnId: 'turn-1',
              seq: 1,
              kind: 'permission',
              detail: 'Bash',
              at: '2026-08-17T12:00:00Z',
            },
          ],
        })}
        working
        providerLabel="Claude"
      />,
    )

    expect(screen.getByTestId('agent-interruption')).toHaveTextContent(
      'Waiting for your permission to run Bash',
    )
    expect(screen.queryByTestId('agent-activity')).toBeNull()
  })

  // An interruption that has been resolved is history, not a state.
  it('returns to the working line once the interruption resolves', () => {
    render(
      <AgentActivityStrip
        activity={activity({
          interruptions: [
            {
              id: 'i1',
              turnId: 'turn-1',
              seq: 1,
              kind: 'compaction',
              at: '2026-08-17T12:00:00Z',
              resolvedAt: '2026-08-17T12:00:05Z',
            },
          ],
        })}
        working
        providerLabel="Claude"
      />,
    )

    expect(screen.queryByTestId('agent-interruption')).toBeNull()
    expect(screen.getByTestId('agent-activity')).toBeInTheDocument()
  })
})

describe('AgentTurnTools', () => {
  it('shows the finished work a reply is built on', () => {
    render(
      <AgentTurnTools
        activity={activity({
          toolCalls: [
            tool({ id: 'a', seq: 2, name: 'Edit', target: 'a.go', status: 'ok', durationMs: 37 }),
            tool({ id: 'b', seq: 1, name: 'Read', target: 'b.go', status: 'ok' }),
          ],
        })}
        turnId="turn-1"
      />,
    )

    const list = screen.getByTestId('agent-turn-tools')
    expect(list).toHaveTextContent('Read · b.go')
    expect(list).toHaveTextContent('Edit · a.go')
    expect(list).toHaveTextContent('37ms')
  })

  it('shows only the tools of ITS turn', () => {
    render(
      <AgentTurnTools
        activity={activity({
          toolCalls: [
            tool({ id: 'mine', turnId: 'turn-1', name: 'Mine', status: 'ok' }),
            tool({ id: 'theirs', turnId: 'turn-2', name: 'Theirs', status: 'ok' }),
          ],
        })}
        turnId="turn-1"
      />,
    )

    expect(screen.getByTestId('agent-turn-tools')).not.toHaveTextContent('Theirs')
  })

  it('renders nothing for a reply with no tools, and omits still-running ones', () => {
    const { container } = render(<AgentTurnTools activity={NO_ACTIVITY} turnId="turn-1" />)
    expect(container).toBeEmptyDOMElement()

    const { container: running } = render(
      <AgentTurnTools activity={activity({ toolCalls: [tool()] })} turnId="turn-1" />,
    )
    expect(running).toBeEmptyDOMElement()
  })

  it('marks a failed tool so a reply built on one is not read as clean', () => {
    render(
      <AgentTurnTools
        activity={activity({ toolCalls: [tool({ status: 'error', name: 'Bash' })] })}
        turnId="turn-1"
      />,
    )

    expect(
      screen.getByTestId('agent-turn-tools').querySelector('[data-tool-status="error"]'),
    ).not.toBeNull()
  })
})
