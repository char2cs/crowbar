import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { TodoStrip } from '@/features/agent/activity/todo-strip'

const plan = [
  { id: '1', text: 'Read the spec', status: 'done' },
  { id: '2', text: 'Survey docs', status: 'active' },
  { id: '3', text: 'Summarize', status: 'pending' },
]

describe('TodoStrip', () => {
  it('shows the active item and progress collapsed and every item expanded', () => {
    render(<TodoStrip plan={plan} />)
    const toggle = screen.getByRole('button', { name: /Survey docs/ })
    expect(toggle).toHaveTextContent('1/3')
    expect(screen.queryByText('Read the spec')).toBeNull()
    fireEvent.click(toggle)
    const items = screen.getAllByRole('listitem')
    expect(items.map((li) => [li.textContent, li.getAttribute('data-status')])).toEqual([
      ['Read the spec', 'done'],
      ['Survey docs', 'active'],
      ['Summarize', 'pending'],
    ])
  })

  it('renders nothing once every item is done', () => {
    const { container } = render(
      <TodoStrip plan={plan.map((item) => ({ ...item, status: 'done' }))} />,
    )
    expect(container).toBeEmptyDOMElement()
  })
})
