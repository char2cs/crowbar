import React from 'react'
import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { GitPanel } from '@/features/git/components/git-panel'

// ── Module mocks ──────────────────────────────────────────────────────────────

vi.mock('@/components/ui/scroll-area', () => ({
  // Avoid react-act warnings from ScrollArea's internal resize observer state.
  ScrollArea: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}))

vi.mock('@/features/git/components/changed-files-tree', () => ({
  ChangedFilesTree: () => <div data-testid="changed-files-tree" />,
}))

// Capture the props the unified BranchSection receives.
const branchSectionProps = vi.fn()
vi.mock('@/features/git/components/branch-section', () => ({
  BranchSection: (props: Record<string, unknown>) => {
    branchSectionProps(props)
    return <div data-testid="branch-section" />
  },
}))

vi.mock('@/features/git/components/git-history-list', () => ({
  GitHistoryList: () => <div data-testid="git-history" />,
}))

vi.mock('@/features/git/hooks/use-review-diff', () => ({
  useReviewDiff: () => ({ files: [], uncommittedCount: 0, loading: false }),
}))

vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getOrCreateWorkspaceStore: () => ({
    getState: () => ({ branchReview: { diffCache: null }, setBranchReviewActiveFile: vi.fn() }),
  }),
}))

// gitStatus is controlled per-test.
let mockGitStatus: {
  branch: string
  ahead: number
  behind: number
  files: Array<{ path: string; status: string; staged: boolean }>
} | null = null
vi.mock('@/features/git/stores/git-store', () => ({
  useGitStore: (sel: (s: { gitStatus: typeof mockGitStatus }) => unknown) =>
    sel({ gitStatus: mockGitStatus }),
}))

// activeWs is controlled per-test.
type MockWs = {
  branch?: string
  parentBranch?: string
  canMergeLocally?: boolean
  status?: string
} | null
let mockActiveWs: MockWs = null
vi.mock('@/lib/store/sidebar', () => ({
  useSidebarStore: (
    sel: (s: { repos: Array<{ workspaces: Array<{ id: string } & NonNullable<MockWs>> }> }) => unknown,
  ) => sel({ repos: mockActiveWs ? [{ workspaces: [{ id: 'ws-active', ...mockActiveWs }] }] : [] }),
}))

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
  beforeEach(() => {
    mockGitStatus = null
    mockActiveWs = null
    vi.clearAllMocks()
  })

  it('renders Changes and History tabs', () => {
    render(<GitPanel />)
    expect(screen.getByRole('tab', { name: /changes/i })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /history/i })).toBeInTheDocument()
  })

  it('renders the changed-files tree and the unified branch section in the Changes tab', () => {
    // BranchSection (and the branch pill above it) only render when a branch is
    // resolved for the active workspace.
    mockGitStatus = { branch: 'develop', ahead: 0, behind: 0, files: [] }
    render(<GitPanel />)
    expect(screen.getByTestId('changed-files-tree')).toBeInTheDocument()
    expect(screen.getByTestId('branch-section')).toBeInTheDocument()
  })

  it('feeds BranchSection the parent/ahead/behind/files from the stores', () => {
    mockGitStatus = {
      branch: 'epoch/first-pr',
      ahead: 2,
      behind: 1,
      files: [{ path: 'a.ts', status: 'modified', staged: false }],
    }
    mockActiveWs = { branch: 'epoch/first-pr', parentBranch: 'develop', canMergeLocally: true, status: 'new' }
    render(<GitPanel />)
    // The branch name is rendered in the pill above BranchSection, not passed as
    // a prop; BranchSection receives the merge/diff metadata.
    expect(branchSectionProps).toHaveBeenCalledWith(
      expect.objectContaining({
        wsId: 'ws-active',
        parentBranch: 'develop',
        canMergeLocally: true,
        status: 'new',
        ahead: 2,
        behind: 1,
        files: mockGitStatus.files,
      }),
    )
  })
})
