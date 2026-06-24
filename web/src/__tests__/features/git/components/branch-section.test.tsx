import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { BranchSection } from '@/features/git/components/branch-section'
import type { GitFile } from '@/features/git/types/git-types'

// Stub the heavy children so this test isolates BranchSection's own rendering.
vi.mock('@/features/git/components/commit-popover', () => ({
  CommitPopover: ({ trigger }: { trigger: React.ReactElement }) => trigger,
}))
vi.mock('@/features/git/components/merge-popover', () => ({
  MergePopover: ({ trigger }: { trigger: React.ReactElement }) => trigger,
}))
vi.mock('@/features/git/api/git-remotes-api', () => ({
  pushChanges: vi.fn().mockResolvedValue({ success: true }),
  pullChanges: vi.fn().mockResolvedValue({ success: true }),
}))

const base = {
  wsId: 'w1',
  branch: 'epoch/first-pr',
  parentBranch: 'develop',
  canMergeLocally: true,
  status: 'new',
  ahead: 0,
  behind: 0,
  files: [] as GitFile[],
}

describe('BranchSection', () => {
  // The branch → parent header lives in the pill ABOVE BranchSection (rendered by
  // GitPanel, asserted in git-panel.test); BranchSection itself no longer renders
  // the branch name (ca42c0d removed the duplicate row).

  it('uncommitted → Commit changes', () => {
    render(<BranchSection {...base} files={[{ path: 'a.ts', status: 'modified', staged: false }]} />)
    expect(screen.getByRole('button', { name: 'Commit changes' })).toBeDefined()
  })

  it('clean + mergeable → Merge into parent', () => {
    render(<BranchSection {...base} />)
    expect(screen.getByRole('button', { name: /Merge into develop/ })).toBeDefined()
  })

  it('clean + protected → a disabled "protected — open a PR" button', () => {
    render(<BranchSection {...base} canMergeLocally={false} />)
    const btn = screen.getByRole('button', { name: /protected/i })
    expect(btn).toBeDefined()
    expect(btn).toBeDisabled()
  })

  it('conflicts with parent (pr-conflicts) → active "Rebase onto parent" + explain message', () => {
    render(<BranchSection {...base} status="pr-conflicts" />)
    // Active (not disabled) — the user chooses to rebase; Crowbar never does it on its own.
    const btn = screen.getByRole('button', { name: /Rebase onto develop/ })
    expect(btn).not.toBeDisabled()
    // No active "Merge into develop" affordance while conflicted.
    expect(screen.queryByRole('button', { name: /Merge into develop/ })).toBeNull()
    expect(screen.getByText(/drag it back to undo/)).toBeDefined()
  })

  it('clean + ahead → a Push secondary', () => {
    render(<BranchSection {...base} ahead={1} />)
    expect(screen.getByRole('button', { name: /Push/ })).toBeDefined()
  })

  it('diverged (ahead + behind) → shows both, and Pull wins as the action', () => {
    render(<BranchSection {...base} ahead={1} behind={1} />)
    expect(screen.getByText(/1 ahead, 1 behind/)).toBeDefined()
    // Pull-before-push: you can't push while behind, so Pull is the action.
    expect(screen.getByRole('button', { name: /Pull/ })).toBeDefined()
  })
})
