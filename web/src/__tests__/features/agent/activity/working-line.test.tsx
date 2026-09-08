import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

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
  afterEach(() => {
    vi.useRealTimers()
  })

  // Matches formatElapsed's own "m:ss, the way a stopwatch reads" — a turn
  // running past a minute used to read as a bare, ever-growing second count
  // ("65s") instead of rolling over.
  it('reads the live elapsed clock as m:ss, not a bare second count', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-18T12:01:05Z'))
    render(<WorkingLine working activity={NO_ACTIVITY} since="2026-08-18T12:00:00Z" />)
    expect(screen.getByText('· 1:05')).toBeInTheDocument()
  })

  it('pads a single-digit second under a minute the same stopwatch way', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-18T12:00:08Z'))
    render(<WorkingLine working activity={NO_ACTIVITY} since="2026-08-18T12:00:00Z" />)
    expect(screen.getByText('· 0:08')).toBeInTheDocument()
  })

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

  // This test's NAME has always said "says the agent is BLOCKED"; its assertion
  // said the opposite — that the strip renders nothing at all. Nothing else
  // renders in its place either: toDividerTag maps permission, notification and
  // elicitation to null, so the spinner vanished and the chat looked dead with no
  // explanation anywhere on screen. That is the "we lose track of whether codex is
  // working" report.
  it('says the agent is BLOCKED rather than working', () => {
    render(<WorkingLine working activity={activity({ interruptions: [interruption()] })} />)

    expect(screen.getByTestId('agent-activity-strip')).toBeInTheDocument()
    expect(screen.getByText(/waiting for your permission/i)).toBeInTheDocument()
  })

  // The original bug this quieting exists to prevent: a blocked agent must not
  // look busy. A blocked line carries no spinner and no rotating verb.
  it('shows no spinner and no working verb while blocked', () => {
    render(<WorkingLine working activity={activity({ interruptions: [interruption()] })} />)

    expect(screen.queryByText(/…$/)).not.toBeInTheDocument()
    expect(screen.getByTestId('agent-activity-strip')).toHaveAttribute('data-blocked', 'true')
  })

  it('names what an elicitation is waiting for', () => {
    render(
      <WorkingLine
        working
        activity={activity({
          interruptions: [interruption({ kind: 'elicitation', detail: 'Pick a database' })],
        })}
      />,
    )
    expect(screen.getByText('Pick a database')).toBeInTheDocument()
  })

  // The composer renders pendingChoices[0], so a pending choice already speaks for
  // the block — two voices saying it is one too many.
  it('stays quiet when a pending choice is already speaking for the block', () => {
    const { container } = render(
      <WorkingLine
        working
        activity={activity({ interruptions: [interruption()], choices: [choice()] })}
      />,
    )
    expect(container).toBeEmptyDOMElement()
  })

  // An interruption Crowbar raised about ITSELF is not a wait on a person: the
  // agent is still working and the transcript already draws a pill for it.
  it('keeps working through an interruption that is not a wait on a person', () => {
    render(
      <WorkingLine
        working
        activity={activity({ interruptions: [interruption({ kind: 'model_changed' })] })}
      />,
    )
    const strip = screen.getByTestId('agent-activity-strip')
    expect(strip).toBeInTheDocument()
    expect(strip).not.toHaveAttribute('data-blocked', 'true')
  })

  // An idle chat carrying a stale unresolved interruption must not claim to be
  // waiting on anything — it is not mid-turn at all.
  it('stays quiet when the chat is not working, however it is blocked', () => {
    const { container } = render(
      <WorkingLine working={false} activity={activity({ interruptions: [interruption()] })} />,
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

  // Compaction is not a wait on a PERSON, unlike every other interruption kind
  // above — it keeps the working line instead of going quiet. Driven by
  // `compactingLive`, NEVER by `activity`: a compaction's ledger interruption
  // record is born already resolved (a bare /compact prompt never opens a
  // tracked turn), so `blockedOn(activity)` can never observe one open — this
  // prop is the only thing that can turn this branch on in production.
  it('keeps working during compaction, with a fixed verb instead of the rotating list', () => {
    render(<WorkingLine working activity={NO_ACTIVITY} compactingLive />)
    expect(screen.getByTestId('agent-activity-strip')).toBeInTheDocument()
    expect(screen.getByText('Compacting…')).toBeInTheDocument()
  })

  // An explicit /compact never opens a tracked turn (it is delivered as a
  // bare prompt the CLI never confirms via user_prompt), so `working` stays
  // FALSE for the entire compaction — live-confirmed on dev-desktop. Gating
  // on `working` alone would hide this branch for exactly the case it exists
  // to cover.
  it('shows Compacting even when working is false — a /compact never opens a tracked turn', () => {
    render(<WorkingLine working={false} activity={NO_ACTIVITY} compactingLive />)
    expect(screen.getByTestId('agent-activity-strip')).toBeInTheDocument()
    expect(screen.getByText('Compacting…')).toBeInTheDocument()
  })

  // A reasoning model spends most of a hard turn emitting nothing but this. With
  // no home for it the whole of that stretch was a spinner over a frozen chat.
  it('shows what the agent is thinking while it works', () => {
    render(<WorkingLine working activity={activity()} reasoning="**Clarifying** the wording" />)
    expect(screen.getByTestId('agent-reasoning')).toHaveTextContent('Clarifying the wording')
  })

  // It is a status line, not a document: the newest thought is the one that says
  // what the agent is doing NOW, so a long block keeps its tail.
  it('trims a long thought from the front, keeping the newest end', () => {
    const long = `START${'x'.repeat(400)}NEWEST`
    render(<WorkingLine working activity={activity()} reasoning={long} />)

    const el = screen.getByTestId('agent-reasoning')
    expect(el).toHaveTextContent('NEWEST')
    expect(el).not.toHaveTextContent('START')
  })

  it('says nothing about thinking when the agent reports none', () => {
    render(<WorkingLine working activity={activity()} />)
    expect(screen.queryByTestId('agent-reasoning')).not.toBeInTheDocument()
  })

  // Compaction is a known, named operation — a stale thought from the turn before
  // it must not sit under "Compacting…".
  it('hides the thought while compacting', () => {
    render(<WorkingLine working activity={activity()} reasoning="old thought" compactingLive />)
    expect(screen.queryByTestId('agent-reasoning')).not.toBeInTheDocument()
  })

  // The whole point of quieting for a block is that the agent is NOT working;
  // a thought left over from before it would contradict that.
  it('hides the thought when blocked on a person', () => {
    render(
      <WorkingLine
        working
        activity={activity({ interruptions: [interruption()] })}
        reasoning="old thought"
      />,
    )
    expect(screen.queryByTestId('agent-reasoning')).not.toBeInTheDocument()
  })

  // A long build emits nothing but this for its whole duration, so without it the
  // tool row sat static with no sign of progress.
  it("shows a running tool's output on that tool's own row", () => {
    render(
      <WorkingLine
        working
        activity={activity({ toolCalls: [tool({ id: 'c1', name: 'commandExecution' })] })}
        toolOutput={{ id: 'c1', text: 'line 1\nline 2\nline 3' }}
      />,
    )
    expect(screen.getByTestId('agent-tool-output')).toHaveTextContent('line 3')
  })

  // Output belongs under the command that produced it, never under whatever else
  // happens to be running.
  it("does not put one tool's output under a different tool", () => {
    render(
      <WorkingLine
        working
        activity={activity({ toolCalls: [tool({ id: 'c1' })] })}
        toolOutput={{ id: 'SOMETHING-ELSE', text: 'line 1' }}
      />,
    )
    expect(screen.queryByTestId('agent-tool-output')).not.toBeInTheDocument()
  })

  // Statuses reaching the component are CROWBAR'S words, already translated from
  // the provider's own by its descriptor.
  it("shows the agent's own to-do list with the active step marked", () => {
    render(
      <WorkingLine
        working
        activity={activity()}
        plan={[
          { text: 'Run the command', status: 'done' },
          { text: 'Summarise it', status: 'active' },
          { text: 'Clean up', status: 'pending' },
        ]}
      />,
    )
    const list = screen.getByTestId('agent-plan')
    expect(list).toHaveTextContent('Run the command')
    expect(list).toHaveTextContent('Summarise it')
    const active = [...list.querySelectorAll('li')].filter(
      (li) => li.getAttribute('data-status') === 'active',
    )
    expect(active).toHaveLength(1)
    expect(active[0]).toHaveTextContent('Summarise it')
  })

  // A status the descriptor's map did not name is still a step: dropping it
  // would silently shorten the plan.
  it('still renders a step whose status it does not recognise', () => {
    render(
      <WorkingLine
        working
        activity={activity()}
        plan={[{ text: 'Mystery', status: 'whoKnows' }]}
      />,
    )
    expect(screen.getByTestId('agent-plan')).toHaveTextContent('Mystery')
  })

  it('shows no plan when the agent reports none', () => {
    render(<WorkingLine working activity={activity()} />)
    expect(screen.queryByTestId('agent-plan')).not.toBeInTheDocument()
  })

  it('names no tools while compacting — there is nothing to enumerate', () => {
    render(<WorkingLine working activity={activity({ toolCalls: [tool()] })} compactingLive />)
    expect(screen.queryByRole('list')).not.toBeInTheDocument()
  })

  // The ledger's own compaction interruption is dead weight for this branch —
  // it is ALWAYS already resolved by the time anything reads it, so it must
  // never be able to turn the compacting branch on by itself.
  it('does not compact off a ledger interruption alone — the ledger record is always already resolved', () => {
    const { container } = render(
      <WorkingLine
        working
        activity={activity({ interruptions: [interruption({ kind: 'compaction' })] })}
      />,
    )
    // This used to assert an EMPTY container, which held only because every
    // unresolved interruption blanked the line outright. That blanking was the
    // "we lose track of whether it is working" bug, so the assertion now states
    // what this test is actually about: a ledger compaction record must not turn
    // the Compacting branch on. A compaction is not a wait on a PERSON either, so
    // an ordinary working line beside it is correct — the chat really is working.
    expect(container).not.toBeEmptyDOMElement()
    expect(screen.queryByText(/compacting/i)).not.toBeInTheDocument()
    expect(screen.getByTestId('agent-activity-strip')).not.toHaveAttribute('data-blocked', 'true')
  })
})
