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

  it('symptom 8: names the active todo while a turn runs', () => {
    render(
      <WorkingLine working activeTodo="Survey docs" toolLabel="Reading a.ts" reasoning="Hmm" />,
    )
    expect(screen.getByTestId('agent-working-heading')).toHaveTextContent('Survey docs')
  })

  it('prefers a pending approval over every other state', () => {
    render(<WorkingLine working asking compacting activeTodo="Survey docs" agents={2} />)
    expect(screen.getByTestId('agent-working-heading')).toHaveTextContent('Waiting for your answer')
    expect(screen.queryByRole('status')).toBeNull()
  })

  it('names the running tool before any thought, and draws nothing for subagents alone', () => {
    const { rerender } = render(
      <WorkingLine working toolLabel="Running npm test" agents={2} reasoning="Hmm" />,
    )
    expect(screen.getByTestId('agent-working-heading')).toHaveTextContent('Running npm test')
    rerender(<WorkingLine working agents={2} />)
    expect(screen.queryByTestId('agent-activity-strip')).toBeNull()
  })

  it('says Writing while answer text streams, and a running tool over it', () => {
    const { rerender } = render(<WorkingLine working streaming since="2026-08-18T12:00:00Z" />)
    expect(screen.getByTestId('agent-working-heading')).toHaveTextContent('Writing…')
    rerender(<WorkingLine working streaming toolLabel="Running npm test" />)
    expect(screen.getByTestId('agent-working-heading')).toHaveTextContent('Running npm test')
  })

  it('announces its heading politely, never the clock', () => {
    render(<WorkingLine working toolLabel="Reading a.ts" since="2026-08-18T12:00:00Z" />)
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
    render(<WorkingLine working={false} connecting="Connecting to Codex…" />)
    expect(screen.getByTestId('agent-working-heading')).toHaveTextContent('Connecting to Codex…')
  })
})
