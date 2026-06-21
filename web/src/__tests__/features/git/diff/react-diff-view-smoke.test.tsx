import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { ReactDiffViewSmoke } from '@/features/git/components/diff/__react-diff-view-smoke'

describe('react-diff-view smoke: mounts under React 19', () => {
  it('renders inserted change text', () => {
    render(<ReactDiffViewSmoke />)
    // The component renders a unified diff with an insert and a delete change
    expect(screen.getByText('+new line')).toBeDefined()
  })

  it('renders deleted change text', () => {
    render(<ReactDiffViewSmoke />)
    expect(screen.getByText('-old line')).toBeDefined()
  })

  it('renders normal (context) change text', () => {
    render(<ReactDiffViewSmoke />)
    // The context line has a leading space; use a function matcher to handle whitespace normalization
    expect(screen.getByText((content) => content.includes('context line'))).toBeDefined()
  })
})
