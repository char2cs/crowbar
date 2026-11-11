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
    expect(toggle).toHaveTextContent('2 allowed · 1 denied')
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryAllByRole('listitem')).toHaveLength(0)
    fireEvent.click(toggle)
    expect(screen.getAllByRole('listitem').map((li) => li.textContent)).toEqual([
      'Bash · Allow',
      'Bash · Allow',
      'Bash · Deny',
    ])
  })

  it('says one request in the singular and leaves out a split that adds nothing', () => {
    render(<ResolvedTray choices={[decided('a1', 'choice', 'Blue')]} />)
    const toggle = screen.getByRole('button', { name: /1 request answered/ })
    expect(toggle).not.toHaveTextContent('allowed')
    expect(toggle).not.toHaveTextContent('denied')
  })

  it('renders nothing without a decision', () => {
    const { container } = render(<ResolvedTray choices={[]} />)
    expect(container).toBeEmptyDOMElement()
  })
})
