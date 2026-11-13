import { act, render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { VERB_ROTATION_MS, verbAt } from '@/features/agent/activity/lib/verbs'
import { WorkingLine } from '@/features/agent/activity/working-line'

describe('WorkingLine', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('reads the live elapsed clock as m:ss, not a bare second count', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-18T12:01:05Z'))
    render(<WorkingLine working since="2026-08-18T12:00:00Z" />)
    expect(screen.getByText('· 1:05')).toBeInTheDocument()
  })

  it('pads a single-digit second under a minute the same stopwatch way', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-18T12:00:08Z'))
    render(<WorkingLine working since="2026-08-18T12:00:00Z" />)
    expect(screen.getByText('· 0:08')).toBeInTheDocument()
  })

  it('renders nothing at all when the chat is idle', () => {
    const { container } = render(<WorkingLine working={false} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('shows the plain working line when the agent reports nothing else', () => {
    render(<WorkingLine working />)
    expect(screen.getByTestId('agent-activity-strip')).toBeInTheDocument()
    expect(screen.queryByRole('list')).not.toBeInTheDocument()
  })

  // The label under the input box says it, so the line stays out of the transcript.
  it('draws nothing while compacting, with or without a turn', () => {
    const { container, rerender } = render(<WorkingLine working compacting />)
    expect(container).toBeEmptyDOMElement()
    rerender(<WorkingLine working={false} compacting />)
    expect(container).toBeEmptyDOMElement()
  })

  it("headlines the agent's own words instead of a flavour verb", () => {
    render(<WorkingLine working reasoning="**Clarifying** the wording" />)
    const verb = screen.getByTestId('agent-working-heading')
    expect(verb).toHaveTextContent('Clarifying the wording')
    expect(verb).not.toHaveTextContent('…')
    expect(screen.queryByTestId('agent-reasoning')).not.toBeInTheDocument()
  })

  it('headlines a bold title and keeps only the body below, never the same text twice', () => {
    render(
      <WorkingLine
        working
        reasoning={'**Checking the schema**\n\nThe table has no index on chat_id.'}
      />,
    )
    expect(screen.getByTestId('agent-working-heading')).toHaveTextContent('Checking the schema')
    const body = screen.getByTestId('agent-reasoning')
    expect(body).toHaveTextContent('The table has no index on chat_id.')
    expect(body).not.toHaveTextContent('Checking the schema')
  })

  it("updates the heading as the agent's words change and falls back when they stop", () => {
    const { rerender } = render(<WorkingLine working reasoning="First" />)
    const verb = () => screen.getByTestId('agent-working-heading')
    expect(verb()).toHaveTextContent('First')
    rerender(<WorkingLine working reasoning="Second" />)
    expect(verb()).toHaveTextContent('Second')
    rerender(<WorkingLine working />)
    expect(verb()).toHaveTextContent(/…$/)
  })

  // Each of these is already on screen as a card, a row, the plan or the streaming text.
  it.each([
    ['a pending request', { asking: true }],
    ['a running tool', { toolLabel: 'Running npm test' }],
    ['the active plan step', { activeTodo: 'Survey docs' }],
    ['streaming answer text', { streaming: true }],
    ['a thought', { asking: true, reasoning: 'Hmm' }],
  ])('draws nothing for %s, which the pane already shows', (_, facts) => {
    const { container } = render(<WorkingLine working {...facts} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('draws nothing for subagents alone', () => {
    render(<WorkingLine working agents={2} />)
    expect(screen.queryByTestId('agent-activity-strip')).toBeNull()
  })

  it('still says Stopping… while a request is pending', () => {
    render(<WorkingLine working asking stopping />)
    expect(screen.getByTestId('agent-working-heading')).toHaveTextContent('Stopping…')
  })

  it('announces its heading politely, never the clock', () => {
    render(<WorkingLine working reasoning="Reading a.ts" since="2026-08-18T12:00:00Z" />)
    const live = screen.getByTestId('agent-working-heading')
    expect(live).toHaveAttribute('aria-live', 'polite')
    expect(live).toHaveTextContent('Reading a.ts')
    expect(live).not.toHaveTextContent(/\d:\d\d/)
  })

  it('announces the rotating verb once as Working, and rotates it out of the live text', () => {
    vi.useFakeTimers()
    render(<WorkingLine working />)
    const live = screen.getByTestId('agent-working-heading')
    const spoken = within(live).getByText('Working')
    expect(spoken.closest('[aria-hidden="true"]')).toBeNull()
    const verb = within(live).getByText(`${verbAt(0)}…`)
    expect(verb).toHaveAttribute('aria-hidden', 'true')
    act(() => void vi.advanceTimersByTime(VERB_ROTATION_MS))
    expect(within(live).getByText(`${verbAt(1)}…`)).toHaveAttribute('aria-hidden', 'true')
    expect(within(live).getByText('Working')).toBe(spoken)
  })

  it('names the connection the chat is waiting on while no turn runs', () => {
    render(<WorkingLine working={false} connecting="Connecting to Provider B…" />)
    expect(screen.getByTestId('agent-working-heading')).toHaveTextContent(
      'Connecting to Provider B…',
    )
  })
})
