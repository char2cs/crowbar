import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { Button } from '@/components/ui/button'
import { MergePopover } from '@/features/git/components/merge-popover'

const { patchMergeStrategy, mergeIntoParent, setBranchReviewMergeStrategy } = vi.hoisted(() => ({
  patchMergeStrategy: vi.fn().mockResolvedValue('squash'),
  mergeIntoParent: vi.fn().mockResolvedValue(undefined),
  setBranchReviewMergeStrategy: vi.fn(),
}))
vi.mock('@/features/git/api/review-api', () => ({
  setMergeStrategy: patchMergeStrategy,
  mergeIntoParent,
}))
let strategy = 'merge'
vi.mock('@/features/workspace/stores/hooks/use-workspace-store-by-id', () => ({
  useWorkspaceStoreById: (_id: string, sel: (s: { branchReview: { mergeStrategy: string } }) => unknown) =>
    sel({ branchReview: { mergeStrategy: strategy } }),
}))
vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getOrCreateWorkspaceStore: () => ({ getState: () => ({ setBranchReviewMergeStrategy }) }),
}))

beforeEach(() => {
  vi.clearAllMocks()
  strategy = 'merge'
})

describe('MergePopover', () => {
  it('opens with the three strategies and a confirm matching the active strategy', async () => {
    const user = userEvent.setup()
    render(<MergePopover wsId="w1" parentBranch="develop" trigger={<Button>Merge into develop</Button>} />)
    await user.click(screen.getByRole('button', { name: 'Merge into develop' }))
    expect(await screen.findByText('Create a merge commit')).toBeDefined()
    expect(screen.getByText('Squash and merge')).toBeDefined()
    expect(screen.getByText('Rebase and merge')).toBeDefined()
    // Active strategy is 'merge' → confirm reads "Create merge commit"
    expect(screen.getByRole('button', { name: 'Create merge commit' })).toBeDefined()
  })

  it('confirm merges with the current strategy', async () => {
    const user = userEvent.setup()
    render(<MergePopover wsId="w1" parentBranch="develop" trigger={<Button>Merge into develop</Button>} />)
    await user.click(screen.getByRole('button', { name: 'Merge into develop' }))
    await user.click(await screen.findByRole('button', { name: 'Create merge commit' }))
    expect(mergeIntoParent).toHaveBeenCalledWith('w1', 'merge')
  })
})
