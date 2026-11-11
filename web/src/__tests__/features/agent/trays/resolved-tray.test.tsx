import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { ResolvedTray } from '@/features/agent/trays/resolved-tray'
import { askItem } from '@/__tests__/__fixtures__/ledger-rows'

const decided = (id: string, kind: string, label: string) =>
  askItem({
    id,
    pending: false,
    resolution: 'answered',
    options: [{ id: 'o', kind, label }],
    answeredOptionIds: ['o'],
  }).choice

describe('ResolvedTray', () => {
  it('folds decided asks into one line that opens to one row per decision', () => {
    const choices = [
      decided('a1', 'allow', 'Allow'),
      decided('a2', 'allow', 'Allow'),
      decided('a3', 'deny', 'Deny'),
    ]
    render(<ResolvedTray choices={choices} />)
    const toggle = screen.getByRole('button', { name: /3 requests answered/ })
    expect(toggle).toHaveTextContent('Show')
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryAllByRole('listitem')).toHaveLength(0)
    fireEvent.click(toggle)
    expect(toggle).toHaveTextContent('Hide')
    expect(toggle).toHaveTextContent('2 allowed · 1 denied')
    expect(screen.getAllByRole('listitem').map((li) => li.firstChild?.textContent)).toEqual([
      'Allowed',
      'Allowed',
      'Denied',
    ])
  })

  it('says one request in the singular', () => {
    render(<ResolvedTray choices={[decided('a1', 'choice', 'Blue')]} />)
    expect(screen.getByRole('button', { name: /1 request answered/ })).toBeInTheDocument()
  })

  it('renders nothing without a decision', () => {
    const { container } = render(<ResolvedTray choices={[]} />)
    expect(container).toBeEmptyDOMElement()
  })
})
