import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { BranchSection } from '@/features/git/components/branch-section'
import type { GitFile } from '@/features/git/types/git-types'

// Stub the heavy children so this test isolates BranchSection's own rendering.
vi.mock('@/features/git/components/commit-dialog', () => ({ CommitDialog: () => null }))
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
  it('shows the branch → parent header', () => {
    render(<BranchSection {...base} />)
    expect(screen.getByText('epoch/first-pr')).toBeDefined()
    expect(screen.getByText('develop')).toBeDefined()
  })

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

  it('clean + conflicts → Resolve conflicts', () => {
    render(<BranchSection {...base} status="pr-conflicts" />)
    expect(screen.getByRole('button', { name: /Resolve conflicts/ })).toBeDefined()
  })

  it('clean + ahead → a Push secondary', () => {
    render(<BranchSection {...base} ahead={1} />)
    expect(screen.getByRole('button', { name: /Push/ })).toBeDefined()
  })
})
