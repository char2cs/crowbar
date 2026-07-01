import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { Button } from '@/components/ui/button'
import { MergePopover } from '@/features/git/components/merge-popover'

const { patchMergeStrategy, mergeIntoParent, setBranchReviewMergeStrategy, navigateMock } =
  vi.hoisted(() => ({
    patchMergeStrategy: vi.fn().mockResolvedValue('squash'),
    mergeIntoParent: vi.fn().mockResolvedValue(undefined),
    setBranchReviewMergeStrategy: vi.fn(),
    navigateMock: vi.fn(),
  }))
vi.mock('@/features/git/api/review-api', () => ({
  setMergeStrategy: patchMergeStrategy,
  mergeIntoParent,
}))
vi.mock('@tanstack/react-router', () => ({ useNavigate: () => navigateMock }))
vi.mock('@/lib/store/sidebar', () => ({
  useSidebarStore: {
    getState: () => ({
      repos: [{ id: 'r1', projectId: 'p1', workspaces: [{ id: 'parent-ws' }, { id: 'w1' }] }],
    }),
  },
  getPostDeleteNavigationTarget: () => 'parent-ws',
}))
let strategy = 'merge'
vi.mock('@/features/workspace/stores/hooks/use-workspace-store-by-id', () => ({
  useWorkspaceStoreById: (
    _id: string,
    sel: (s: { branchReview: { mergeStrategy: string } }) => unknown,
  ) => sel({ branchReview: { mergeStrategy: strategy } }),
}))
vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getOrCreateWorkspaceStore: () => ({ getState: () => ({ setBranchReviewMergeStrategy }) }),
}))

beforeEach(() => {
  vi.clearAllMocks()
  strategy = 'merge'
})

const renderPopover = () =>
  render(
    <MergePopover wsId="w1" parentBranch="develop" trigger={<Button>Merge into develop</Button>} />,
  )

describe('MergePopover', () => {
  it('opens with the three strategies and a confirm matching the active strategy', async () => {
    const user = userEvent.setup()
    renderPopover()
    await user.click(screen.getByRole('button', { name: 'Merge into develop' }))
    expect(await screen.findByText('Create a merge commit')).toBeDefined()
    expect(screen.getByText('Squash and merge')).toBeDefined()
    expect(screen.getByText('Rebase and merge')).toBeDefined()
    expect(screen.getByRole('button', { name: 'Create merge commit' })).toBeDefined()
  })

  it('defaults to deleting the child + redirects to the parent after merge', async () => {
    const user = userEvent.setup()
    renderPopover()
    await user.click(screen.getByRole('button', { name: 'Merge into develop' }))
    // The delete checkbox is present and on by default.
    expect(await screen.findByText('Delete this workspace after merging')).toBeDefined()
    expect(screen.getByRole('checkbox')).toBeDefined()

    await user.click(screen.getByRole('button', { name: 'Create merge commit' }))
    expect(mergeIntoParent).toHaveBeenCalledWith('w1', 'merge', true)
    expect(navigateMock).toHaveBeenCalledWith({
      to: '/ide/$projectId/$repoId/$wsId',
      params: { projectId: 'p1', repoId: 'r1', wsId: 'parent-ws' },
    })
  })

  it('unchecking "delete" keeps the child and does not redirect', async () => {
    const user = userEvent.setup()
    renderPopover()
    await user.click(screen.getByRole('button', { name: 'Merge into develop' }))
    // Toggle off via the label text (base-ui checkbox button doesn't toggle on a
    // direct role-click under jsdom's missing PointerEvent).
    await user.click(await screen.findByText('Delete this workspace after merging'))
    await user.click(screen.getByRole('button', { name: 'Create merge commit' }))
    expect(mergeIntoParent).toHaveBeenCalledWith('w1', 'merge', false)
    expect(navigateMock).not.toHaveBeenCalled()
  })
})
