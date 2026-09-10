import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AgentChoice, AgentSubagent, AgentToolCall } from '@/features/agent/api/agent-api'
import * as agentApi from '@/features/agent/api/agent-api'
import {
  AgentLiveTurnTools,
  AgentTurnChoices,
  AgentTurnSubagents,
  AgentTurnTools,
  groupChoicesByTurn,
  groupSubagentsByTurn,
  groupToolCallsByTurn,
} from '@/features/agent/transcript/turn-tools'

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

function subagent(overrides: Partial<AgentSubagent> = {}): AgentSubagent {
  return {
    id: 's1',
    turnId: 'turn-1',
    seq: 1,
    startedAt: '2026-08-17T12:00:00Z',
    endedAt: '2026-08-17T12:00:01Z',
    ...overrides,
  }
}

function choice(overrides: Partial<AgentChoice> = {}): AgentChoice {
  return {
    id: 'c1',
    turnId: 'turn-1',
    seq: 1,
    kind: 'tool_permission',
    toolName: 'Bash',
    options: [{ id: 'allow', kind: 'allow', label: 'Allow' }],
    pending: false,
    answerable: false,
    at: '2026-08-17T12:00:00Z',
    resolution: 'answered',
    answeredOptionIds: ['allow'],
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

  // AgentToolCall.error was captured by the backend and shipped on the wire
  // long before anything here read it — the row rendered the SAME red text
  // whether a call failed with "exit status 1" or with no message at all.
  it('shows a failed call’s own caption, not just its colour', () => {
    render(
      <AgentTurnTools
        turnId="turn-1"
        callsByTurn={groupToolCallsByTurn([
          tool({ status: 'error', name: 'Bash', error: 'exit status 1' }),
        ])}
      />,
    )
    expect(screen.getByText('exit status 1')).toBeInTheDocument()
  })

  // hasRequest/hasResult and getToolPayload existed on the API client with zero
  // call sites — the request/result bytes were captured and unreachable. A
  // finished row with either now opens them on demand, and one with neither
  // stays the same plain summary it always was.
  describe('with wsId/chatId — the finished call’s own payload', () => {
    afterEach(() => vi.restoreAllMocks())

    it('offers no expand affordance for a call with no captured payload', () => {
      render(
        <AgentTurnTools
          turnId="turn-1"
          wsId="ws1"
          chatId="c1"
          callsByTurn={groupToolCallsByTurn([tool({ hasRequest: false, hasResult: false })])}
        />,
      )
      expect(document.querySelector('details')).not.toBeInTheDocument()
    })

    it('fetches and shows the request/result once a captured call is expanded', async () => {
      const spy = vi
        .spyOn(agentApi, 'getToolPayload')
        .mockImplementation(async (_ws, _chat, _id, side) =>
          side === 'request' ? '{"cmd":"go test"}' : 'PASS',
        )
      render(
        <AgentTurnTools
          turnId="turn-1"
          wsId="ws1"
          chatId="c1"
          callsByTurn={groupToolCallsByTurn([tool({ hasRequest: true, hasResult: true })])}
        />,
      )
      const summary = document.querySelector('summary')
      expect(summary).toBeInTheDocument()
      // Not fetched merely because the row exists — content-addressed bytes
      // that can run to hundreds of KB per turn must stay on demand.
      expect(spy).not.toHaveBeenCalled()

      fireEvent.click(summary!)

      expect(await screen.findByText(/"cmd": "go test"/)).toBeInTheDocument()
      expect(await screen.findByText('PASS')).toBeInTheDocument()
      expect(spy).toHaveBeenCalledWith('ws1', 'c1', 't1', 'request', expect.anything())
      expect(spy).toHaveBeenCalledWith('ws1', 'c1', 't1', 'result', expect.anything())
    })

    it('says a swept payload is no longer available rather than nothing', async () => {
      vi.spyOn(agentApi, 'getToolPayload').mockResolvedValue(null)
      render(
        <AgentTurnTools
          turnId="turn-1"
          wsId="ws1"
          chatId="c1"
          callsByTurn={groupToolCallsByTurn([tool({ hasResult: true })])}
        />,
      )
      fireEvent.click(document.querySelector('summary')!)
      expect(await screen.findByText('No longer available')).toBeInTheDocument()
    })
  })
})

describe('groupSubagentsByTurn', () => {
  it('groups ended subagents by turn, sorted by seq, excluding ones still running', () => {
    const subagents = [
      subagent({ id: 'a', turnId: 't1', seq: 2 }),
      subagent({ id: 'b', turnId: 't1', seq: 1 }),
      subagent({ id: 'running', turnId: 't1', seq: 3, endedAt: undefined }),
    ]

    expect(
      groupSubagentsByTurn(subagents)
        .get('t1')
        ?.map((s) => s.id),
    ).toEqual(['b', 'a'])
  })
})

describe('AgentTurnSubagents', () => {
  // subagent-shelf.tsx deliberately shows only RUNNING subagents — it is a live
  // status strip, not a log. Without this, a subagent that finished had no
  // record anywhere: the shelf drops it, and nothing else ever showed it.
  it('shows an ended subagent under the reply', () => {
    render(
      <AgentTurnSubagents
        subagentsByTurn={groupSubagentsByTurn([
          subagent({
            agentType: 'reviewer',
            startedAt: '2026-08-17T12:00:00Z',
            endedAt: '2026-08-17T12:00:02Z',
          }),
        ])}
        turnId="turn-1"
      />,
    )
    expect(screen.getByText('Subagent · reviewer')).toBeInTheDocument()
    expect(screen.getByText('2.0s')).toBeInTheDocument()
  })

  it('renders nothing for a turn with no ended subagents', () => {
    const { container } = render(<AgentTurnSubagents subagentsByTurn={new Map()} turnId="turn-1" />)
    expect(container).toBeEmptyDOMElement()
  })
})

describe('groupChoicesByTurn', () => {
  it('groups resolved choices by turn, excluding still-pending ones', () => {
    const choices = [
      choice({ id: 'a', turnId: 't1', seq: 1 }),
      choice({ id: 'open', turnId: 't1', seq: 2, pending: true }),
    ]

    expect(
      groupChoicesByTurn(choices)
        .get('t1')
        ?.map((c) => c.id),
    ).toEqual(['a'])
  })
})

describe('AgentTurnChoices', () => {
  // Resolved choices were never rendered anywhere — no record of what was
  // approved or denied once the CLI stopped waiting on it.
  it('shows what a resolved permission was actually decided', () => {
    render(<AgentTurnChoices choicesByTurn={groupChoicesByTurn([choice()])} turnId="turn-1" />)
    expect(screen.getByText('Bash · Allow')).toBeInTheDocument()
  })

  it('renders nothing for a turn with no resolved choices', () => {
    const { container } = render(<AgentTurnChoices choicesByTurn={new Map()} turnId="turn-1" />)
    expect(container).toBeEmptyDOMElement()
  })
})

// A tool call was durable in the ledger and invisible on screen for the whole
// rest of its turn: it is filed under the OPEN turn, no message carries that
// turn id yet, and nothing else drew it — so every call of a turn appeared at
// once, when the turn ended. Measured live before this existed: five calls, the
// first known to the backend at t=13.8s, all five first painted at t=39.4s.
describe('AgentLiveTurnTools', () => {
  it('draws a call that has no reply to sit under yet', () => {
    render(
      <AgentLiveTurnTools
        calls={[tool({ name: 'commandExecution', target: 'ls -la', durationMs: 17 })]}
      />,
    )
    expect(screen.getByText('commandExecution · ls -la')).toBeInTheDocument()
    expect(screen.getByText('17ms')).toBeInTheDocument()
  })

  // The one thing a live list has to say that a finished one does not.
  it('keeps a still-running call, marked as running', () => {
    render(<AgentLiveTurnTools calls={[tool({ status: 'running', name: 'Bash' })]} />)
    expect(screen.getByText('Bash').closest('li')).toHaveAttribute('data-status', 'running')
  })

  it('renders nothing when no turn is in flight', () => {
    const { container } = render(<AgentLiveTurnTools calls={[]} />)
    expect(container).toBeEmptyDOMElement()
  })

  // A long build emits nothing but this for its whole duration, so without it
  // the row sits static with no sign of progress.
  it("shows a running call's output on that call's own row", () => {
    render(
      <AgentLiveTurnTools
        calls={[tool({ id: 'c1', status: 'running', name: 'commandExecution' })]}
        toolOutput={{ id: 'c1', text: 'line 1\nline 2\nline 3' }}
      />,
    )
    expect(screen.getByTestId('agent-tool-output')).toHaveTextContent('line 3')
  })

  it("does not put one call's output under a different call", () => {
    render(
      <AgentLiveTurnTools
        calls={[tool({ id: 'c1', status: 'running' })]}
        toolOutput={{ id: 'SOMETHING-ELSE', text: 'line 1' }}
      />,
    )
    expect(screen.queryByTestId('agent-tool-output')).not.toBeInTheDocument()
  })
})
