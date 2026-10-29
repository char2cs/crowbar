import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { SubagentShelf } from '@/features/agent/activity/subagent-shelf'
import type { LiveSubagent } from '@/features/agent/stores/lib/transcript-items'

function subagent(scope: string, overrides: Partial<LiveSubagent> = {}): LiveSubagent {
  return { scope, startedAt: '2026-08-17T12:00:00Z', ...overrides }
}

describe('SubagentShelf', () => {
  it('renders nothing when nothing is running', () => {
    const { container } = render(<SubagentShelf subagents={{}} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders the running strip when a subagent is running', () => {
    render(<SubagentShelf subagents={{ a1: subagent('a1') }} />)
    expect(screen.getByTestId('agent-subagent-shelf')).toBeInTheDocument()
  })

  // The fold drops a subagent from the live set when it ends, so the shelf
  // shows exactly what it is handed.
  it('counts every running subagent', () => {
    render(
      <SubagentShelf subagents={{ a1: subagent('a1'), a2: subagent('a2', { type: 'Explore' }) }} />,
    )
    expect(screen.getByTestId('agent-subagent-shelf')).toHaveAccessibleName('2 subagents running')
  })
})
