import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { BranchReviewDiffViewer } from '@/features/branch-review/components/branch-review-diff-viewer'
import { getMockBranchDiff } from '@/lib/mock/branch-diff'

const mockDiff = getMockBranchDiff('ws1')

describe('BranchReviewDiffViewer', () => {
  it('renders file names from the diff', () => {
    render(<BranchReviewDiffViewer multiDiff={mockDiff} threads={[]} onAddThread={() => {}} onReply={() => {}} onResolve={() => {}} />)
    expect(screen.getByText('routes.ts')).toBeTruthy()
    expect(screen.getByText('auth.ts')).toBeTruthy()
  })

  it('shows additions and deletions counts per file', () => {
    render(<BranchReviewDiffViewer multiDiff={mockDiff} threads={[]} onAddThread={() => {}} onReply={() => {}} onResolve={() => {}} />)
    expect(screen.getByText('+42')).toBeTruthy()
    expect(screen.getByText('-8')).toBeTruthy()
  })

  it('renders inline thread for the correct line', () => {
    const threads = [{
      id: 't1', filePath: 'src/api/routes.ts', lineNumber: 2,
      side: 'right' as const, isResolved: false,
      messages: [{ id: 'm1', author: 'Claude', isAgent: true, body: 'Use strict mode', createdAt: '2026-06-01' }],
    }]
    render(<BranchReviewDiffViewer multiDiff={mockDiff} threads={threads} onAddThread={() => {}} onReply={() => {}} onResolve={() => {}} />)
    expect(screen.getByText('Use strict mode')).toBeTruthy()
  })

  it('calls onAddThread when + button is clicked on a line', async () => {
    const onAddThread = vi.fn()
    render(<BranchReviewDiffViewer multiDiff={mockDiff} threads={[]} onAddThread={onAddThread} onReply={() => {}} onResolve={() => {}} />)
    const addButtons = screen.getAllByRole('button', { name: /add thread/i })
    await userEvent.click(addButtons[0])
    expect(onAddThread).toHaveBeenCalledOnce()
  })
})
