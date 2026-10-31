import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { ReasoningItem } from '@/features/agent/stores/lib/transcript-items'
import { ReasoningRow } from '@/features/agent/transcript/rows/reasoning-row'

const item = (startedAt: string, endedAt: string): ReasoningItem => ({
  kind: 'reasoning',
  id: 'r:turn-1:m1',
  seq: 4,
  turnId: 'turn-1',
  at: endedAt,
  text: 'Checking **the schema** first',
  startedAt,
  endedAt,
})

describe('ReasoningRow', () => {
  it('collapses persisted reasoning to Thought for Ns', () => {
    render(<ReasoningRow item={item('2026-10-01T12:00:00Z', '2026-10-01T12:00:12Z')} />)
    const toggle = screen.getByRole('button', { name: 'Thought for 12s' })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText(/Checking/)).toBeNull()
    fireEvent.click(toggle)
    expect(screen.getByText(/Checking/)).toBeInTheDocument()
  })

  it('says only Thought when its time is not known', () => {
    render(<ReasoningRow item={item('2026-10-01T12:00:00Z', '2026-10-01T12:00:00Z')} />)
    expect(screen.getByRole('button', { name: 'Thought' })).toBeInTheDocument()
  })
})
