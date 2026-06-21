import React from 'react'
import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { GitPanel } from '@/features/git/components/git-panel'

// ── Module mocks ──────────────────────────────────────────────────────────────

vi.mock('@/components/ui/scroll-area', () => ({
  // Avoid react-act warnings from ScrollArea's internal resize observer state.
  ScrollArea: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}))

vi.mock('@/features/git/components/changed-files-tree', () => ({
  ChangedFilesTree: () => <div data-testid="changed-files-tree" />,
}))
vi.mock('@/features/git/components/git-commit-panel', () => ({
  default: () => <div data-testid="commit-panel" />,
}))
vi.mock('@/features/git/components/merge-section', () => ({
  MergeSection: () => <div data-testid="merge-section" />,
}))
vi.mock('@/features/git/components/git-history-list', () => ({
  GitHistoryList: () => <div data-testid="git-history" />,
}))

vi.mock('@/features/git/hooks/use-review-diff', () => ({
  useReviewDiff: () => ({ files: [], uncommittedCount: 0, loading: false }),
}))
vi.mock('@/features/git/hooks/use-git-diff-handlers', () => ({
  useGitDiffHandlers: () => ({ handleViewFileDiff: vi.fn() }),
}))
vi.mock('@/features/git/stores/git-store', () => {
  const useGitStore = (sel: (s: { gitStatus: null }) => unknown) => sel({ gitStatus: null })
  return { useGitStore }
})
vi.mock('@/lib/store/sidebar', () => ({
  useSidebarStore: (sel: (s: { repos: [] }) => unknown) => sel({ repos: [] }),
}))
vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getActiveWorkspaceId: () => null,
}))
vi.mock('@/features/panes/utils/pane-command-actions', () => ({
  openBranchReviewForActiveWorkspace: vi.fn(),
}))

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('GitPanel', () => {
  it('renders Changes and History tabs', () => {
    render(<GitPanel />)
    expect(screen.getByRole('tab', { name: /changes/i })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /history/i })).toBeInTheDocument()
  })

  it('shows the changed-files tree and commit panel in the Changes tab by default', () => {
    render(<GitPanel />)
    expect(screen.getByTestId('changed-files-tree')).toBeInTheDocument()
    expect(screen.getByTestId('commit-panel')).toBeInTheDocument()
  })

  it('does not show the merge section when the workspace has no parentBranch', () => {
    render(<GitPanel />)
    expect(screen.queryByTestId('merge-section')).not.toBeInTheDocument()
  })
})
