import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MergeButton } from '@/features/branch-review/components/merge-button'

describe('MergeButton', () => {
  it('shows the selected strategy label', () => {
    render(<MergeButton strategy="merge" isLocked={false} hasConflicts={false} onMerge={() => {}} onStrategyChange={() => {}} />)
    expect(screen.getByRole('button', { name: /merge commit/i })).toBeTruthy()
  })

  it('is disabled and shows tooltip when parent is locked', () => {
    render(<MergeButton strategy="merge" isLocked={true} hasConflicts={false} onMerge={() => {}} onStrategyChange={() => {}} />)
    const btn = screen.getByRole('button', { name: /merge commit/i })
    expect(btn).toBeDisabled()
    expect(btn.title).toMatch(/locked/i)
  })

  it('is disabled and shows tooltip when branch has conflicts', () => {
    render(<MergeButton strategy="squash" isLocked={false} hasConflicts={true} onMerge={() => {}} onStrategyChange={() => {}} />)
    const btn = screen.getByRole('button', { name: /squash and merge/i })
    expect(btn).toBeDisabled()
    expect(btn.title).toMatch(/conflict/i)
  })

  it('calls onMerge when enabled and clicked', async () => {
    const onMerge = vi.fn()
    render(<MergeButton strategy="rebase" isLocked={false} hasConflicts={false} onMerge={onMerge} onStrategyChange={() => {}} />)
    await userEvent.click(screen.getByRole('button', { name: /rebase and merge/i }))
    expect(onMerge).toHaveBeenCalledOnce()
  })
})
