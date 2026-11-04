import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { BranchReviewHeader } from '@/features/git/components/branch-review-header'

describe('BranchReviewHeader', () => {
  it('carries the full title, branch and base in title attributes and keeps the counts', () => {
    const title = 'T'.repeat(120)
    const branch = 'feature/' + 'long-branch-name-'.repeat(6)
    const base = 'release/' + 'long-base-name-'.repeat(5)
    render(
      <BranchReviewHeader
        title={title}
        branch={branch}
        baseBranch={base}
        stats={{ files: 3, additions: 10, deletions: 4 }}
      />,
    )

    expect(screen.getByRole('heading').getAttribute('title')).toBe(title)
    expect(screen.getByText(branch).getAttribute('title')).toBe(branch)
    expect(screen.getByText(base).getAttribute('title')).toBe(base)
    expect(screen.getByText('3 files')).toBeDefined()
    expect(screen.getByText('+10')).toBeDefined()
    expect(screen.getByText('-4')).toBeDefined()
  })

  it('keeps the branch and base pills in one unit, apart from the counts', () => {
    render(
      <BranchReviewHeader
        title="x"
        branch="feat"
        baseBranch="main"
        stats={{ files: 1, additions: 0, deletions: 0 }}
      />,
    )
    const unit = screen.getByText('feat').parentElement as HTMLElement
    expect(unit.contains(screen.getByText('main'))).toBe(true)
    expect(unit.contains(screen.getByText('1 file'))).toBe(false)
  })
})
