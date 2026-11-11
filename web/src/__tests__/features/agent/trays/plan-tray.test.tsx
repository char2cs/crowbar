import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { PlanTray } from '@/features/agent/trays/plan-tray'

const plan = [
  { id: '1', text: 'Read the spec', status: 'done' },
  { id: '2', text: 'Survey docs', status: 'active' },
  { id: '3', text: 'Summarize', status: 'pending' },
]

describe('PlanTray', () => {
  it('shows progress and the current step collapsed, every step once opened', () => {
    render(<PlanTray plan={plan} />)
    const toggle = screen.getByRole('button', { name: /1 of 3 done/ })
    expect(toggle).toHaveTextContent('Survey docs')
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('Read the spec')).toBeNull()
    fireEvent.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    const items = screen.getAllByRole('listitem')
    expect(items.map((li) => [li.textContent, li.getAttribute('data-status')])).toEqual([
      ['Read the spec', 'done'],
      ['Survey docs', 'active'],
      ['Summarize', 'pending'],
    ])
  })

  it('spins only the active step', () => {
    render(<PlanTray plan={plan} />)
    fireEvent.click(screen.getByRole('button'))
    expect(screen.getAllByRole('status')).toHaveLength(1)
    expect(screen.getByRole('status').closest('li')).toHaveTextContent('Survey docs')
  })

  it('renders nothing without a plan or once every step is done', () => {
    const { container, rerender } = render(<PlanTray plan={undefined} />)
    expect(container).toBeEmptyDOMElement()
    rerender(<PlanTray plan={plan.map((item) => ({ ...item, status: 'done' }))} />)
    expect(container).toBeEmptyDOMElement()
  })
})
