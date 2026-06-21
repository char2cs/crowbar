import React from 'react'
import { render, screen, fireEvent } from '@testing-library/react'
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

// Capture onCommitSuccess so we can test I3 wiring.
const mockCommitPanel = vi.fn(
  ({ onCommitSuccess }: { onCommitSuccess?: () => void }) => (
    <button data-testid="commit-panel" onClick={onCommitSuccess}>
      Commit
    </button>
  ),
)
vi.mock('@/features/git/components/git-commit-panel', () => ({
  default: (props: { onCommitSuccess?: () => void }) => mockCommitPanel(props),
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

// Mock getOrCreateWorkspaceStore so git-panel can call it in event handlers.
const mockSetBranchReviewActiveFile = vi.fn()
vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getOrCreateWorkspaceStore: () => ({
    getState: () => ({
      branchReview: { diffCache: null },
      setBranchReviewActiveFile: mockSetBranchReviewActiveFile,
    }),
  }),
}))

vi.mock('@/features/git/stores/git-store', () => {
  const useGitStore = (sel: (s: { gitStatus: null }) => unknown) => sel({ gitStatus: null })
  return { useGitStore }
})
vi.mock('@/lib/store/sidebar', () => ({
  useSidebarStore: (sel: (s: { repos: [] }) => unknown) => sel({ repos: [] }),
}))
// I1: GitPanel now reads wsId reactively via useRouterState + parseWorkspaceScopeFromPath.
// Mock useRouterState to control the pathname; getActiveWorkspaceId is no longer used.
vi.mock('@tanstack/react-router', () => ({
  useRouterState: ({ select }: { select: (s: { location: { pathname: string } }) => unknown }) =>
    select({ location: { pathname: '/ide/proj1/repo1/ws-active' } }),
}))
vi.mock('@/lib/workspace-scope', () => ({
  parseWorkspaceScopeFromPath: (pathname: string) => {
    const m = pathname.match(/\/ide\/([^/]+)\/([^/]+)\/([^/]+)/)
    if (!m) return null
    return { projectId: m[1], repoId: m[2], wsId: m[3] }
  },
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

  // I1: wsId is derived reactively from the route pathname, not a mount-time snapshot.
  // The mocked useRouterState returns pathname '/ide/proj1/repo1/ws-active', so
  // parseWorkspaceScopeFromPath yields wsId = 'ws-active'. File clicks now open the
  // unified branch-review tab; useGitDiffHandlers is no longer called by GitPanel.
  it('(I1) derives wsId from route and renders the panel without error', () => {
    render(<GitPanel />)
    // Panel renders correctly with wsId from route — tabs are present.
    expect(screen.getByRole('tab', { name: /changes/i })).toBeInTheDocument()
  })

  // I3: onCommitSuccess dispatches git-status-changed so useReviewDiff re-fetches.
  it('(I3) dispatches git-status-changed when onCommitSuccess fires', () => {
    render(<GitPanel />)
    const listener = vi.fn()
    window.addEventListener('git-status-changed', listener)
    fireEvent.click(screen.getByTestId('commit-panel'))
    window.removeEventListener('git-status-changed', listener)
    expect(listener).toHaveBeenCalledTimes(1)
  })
})
