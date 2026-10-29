import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

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

  // A chat waiting on a person is not working; the composer speaks for the ask.
  it('says nothing at all while an ask holds the composer', () => {
    const { container } = render(<WorkingLine working asking />)
    expect(container).toBeEmptyDOMElement()
  })

  it('keeps working during compaction, with a fixed verb instead of the rotating list', () => {
    render(<WorkingLine working compacting />)
    expect(screen.getByText('Compacting…')).toBeInTheDocument()
  })

  // A /compact opens no turn, so `working` stays false the whole way through.
  it('shows Compacting even when no turn is running', () => {
    render(<WorkingLine working={false} compacting />)
    expect(screen.getByText('Compacting…')).toBeInTheDocument()
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

  // A stale thought from the turn before must not sit under "Compacting…".
  it('hides the thought while compacting', () => {
    render(<WorkingLine working reasoning="old thought" compacting />)
    expect(screen.queryByTestId('agent-reasoning')).not.toBeInTheDocument()
  })

  it("shows the agent's own to-do list with the active step marked", () => {
    render(
      <WorkingLine
        working
        plan={[
          { text: 'Run the command', status: 'done' },
          { text: 'Summarise it', status: 'active' },
          { text: 'Clean up', status: 'pending' },
        ]}
      />,
    )
    const list = screen.getByTestId('agent-plan')
    expect(list).toHaveTextContent('Run the command')
    const active = [...list.querySelectorAll('li')].filter(
      (li) => li.getAttribute('data-status') === 'active',
    )
    expect(active).toHaveLength(1)
    expect(active[0]).toHaveTextContent('Summarise it')
  })

  // Dropping a step whose status the descriptor did not map would shorten the plan.
  it('still renders a step whose status it does not recognise', () => {
    render(<WorkingLine working plan={[{ text: 'Mystery', status: 'whoKnows' }]} />)
    expect(screen.getByTestId('agent-plan')).toHaveTextContent('Mystery')
  })

  it('lists nothing while compacting — there is nothing to enumerate', () => {
    render(<WorkingLine working compacting plan={[{ text: 'Step', status: 'active' }]} />)
    expect(screen.queryByRole('list')).not.toBeInTheDocument()
  })
})
