import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { DiffReviewHeader } from '@/features/git/components/diff/diff-review-header'

describe('DiffReviewHeader', () => {
  it('commit flavor: shows the commit message as title and author + hash in the meta', () => {
    render(
      <DiffReviewHeader
        title="QA hardening (#16)"
        author="Mateo Urrutia"
        date="2026-06-10T00:00:00Z"
        hash="06cef53"
      />,
    )
    expect(screen.getByText('QA hardening (#16)')).toBeDefined()
    expect(screen.getByText('Mateo Urrutia')).toBeDefined()
    expect(screen.getByText('06cef53')).toBeDefined()
    // No base-branch arrow in commit mode.
    expect(screen.queryByText('develop')).toBeNull()
  })

  it('branch flavor: shows the branch name as title and the base branch as meta', () => {
    render(<DiffReviewHeader title="epoch/first-pr" baseBranch="develop" />)
    expect(screen.getByText('epoch/first-pr')).toBeDefined()
    expect(screen.getByText('develop')).toBeDefined()
    // No commit identity in branch mode.
    expect(screen.queryByText('Mateo Urrutia')).toBeNull()
  })

  it('renders the optional description under the title', () => {
    render(<DiffReviewHeader title="A title" description="Extended body text" />)
    expect(screen.getByText('Extended body text')).toBeDefined()
  })

  it('renders nothing when there is no title, description, or meta', () => {
    const { container } = render(<DiffReviewHeader />)
    expect(container.firstChild).toBeNull()
  })
})
