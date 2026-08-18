import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import type { AgentActivity, AgentChoice, AgentToolCall } from '@/features/agent/api/agent-api'
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

// A chat waiting on a HUMAN is not a chat doing work, and until prompts existed
// the two looked identical. The prompt itself is AgentChoicePrompts; this strip's
// job is to stop saying anything that contradicts it.
describe('AgentActivityStrip while a prompt is open', () => {
  it('says nothing at all — not "working…", not a second banner', () => {
    const { container } = render(
      <AgentActivityStrip
        activity={activity({
          choices: [choice()],
          toolCalls: [tool({ name: 'Bash' })],
          // The permission hook records BOTH an interruption and the prompt, so
          // without this the same blockage would be announced twice.
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

    expect(container).toBeEmptyDOMElement()
  })

  // A prompt that is pending and NOT answerable still silences the strip: the CLI
  // is genuinely blocked, it is just blocked at its own terminal.
  it('stays quiet for a prompt nobody here can answer', () => {
    const { container } = render(
      <AgentActivityStrip
        activity={activity({ choices: [choice({ answerable: false })] })}
        working
        providerLabel="Claude"
      />,
    )

    expect(container).toBeEmptyDOMElement()
  })

  it('goes back to the working line once the prompt is resolved', () => {
    render(
      <AgentActivityStrip
        activity={activity({ choices: [choice({ pending: false, resolution: 'proceeded' })] })}
        working
        providerLabel="Claude"
      />,
    )

    expect(screen.getByTestId('agent-activity')).toHaveTextContent('Claude is working…')
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
