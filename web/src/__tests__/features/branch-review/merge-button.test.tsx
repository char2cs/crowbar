import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MergeButton } from '@/features/branch-review/components/merge-button'

describe('MergeButton', () => {
  it('shows the selected strategy label', () => {
    render(<MergeButton strategy="merge" isLocked={false} hasConflicts={false} onMerge={() => {}} onStrategyChange={() => {}} />)
    expect(screen.getByRole('button', { name: /merge commit/i })).toBeTruthy()
  })

  it('is disabled when parent is locked', () => {
    render(<MergeButton strategy="merge" isLocked={true} hasConflicts={false} onMerge={() => {}} onStrategyChange={() => {}} />)
    expect(screen.getByRole('button', { name: /merge commit/i })).toBeDisabled()
  })

  it('is disabled when branch has conflicts', () => {
    render(<MergeButton strategy="squash" isLocked={false} hasConflicts={true} onMerge={() => {}} onStrategyChange={() => {}} />)
    expect(screen.getByRole('button', { name: /squash and merge/i })).toBeDisabled()
  })

  it('opens a commit-message popover and calls onMerge on submit', async () => {
    const onMerge = vi.fn()
    render(<MergeButton strategy="rebase" branchName="feature/x" isLocked={false} hasConflicts={false} onMerge={onMerge} onStrategyChange={() => {}} />)
    // Click the trigger to open the popover
    await userEvent.click(screen.getByRole('button', { name: /rebase and merge/i }))
    // The form fields appear
    expect(await screen.findByText('Commit message')).toBeInTheDocument()
    // Submit (the in-popover submit button is the second "rebase and merge")
    const submitButtons = screen.getAllByRole('button', { name: /rebase and merge/i })
    await userEvent.click(submitButtons[submitButtons.length - 1])
    expect(onMerge).toHaveBeenCalledOnce()
    expect(onMerge).toHaveBeenCalledWith({ title: 'Merge feature/x', description: '' })
  })
})
