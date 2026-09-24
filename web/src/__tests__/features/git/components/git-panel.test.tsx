import React from 'react'
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { GitPanel } from '@/features/git/components/git-panel'
import {
  publishFocusedWorkspaceContext,
  type FocusedWorkspaceContext,
} from '@/features/window/stores/focused-workspace-context-store'

// ── Module mocks ──────────────────────────────────────────────────────────────

vi.mock('@/components/ui/scroll-area', () => ({
  // Avoid react-act warnings from ScrollArea's internal resize observer state.
  ScrollArea: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}))

vi.mock('@/features/git/components/changed-files-tree', () => ({
  ChangedFilesTree: ({ files }: { files: Array<{ path: string }> }) => (
    <div data-testid="changed-files-tree">
      {files.map((f) => (
        <span key={f.path}>{f.path}</span>
      ))}
    </div>
  ),
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

// GitPanel sources its changed-files list from useSidebarChangedFiles (which
// gates the full-diff fetch on the review pane). The data-source behavior is
// covered by use-sidebar-changed-files.test.ts; here we just stub it — but
// controllable per-test, so a "no wsId" case can simulate that hook's own
// documented stale-global-fallback behavior (it reads `useGitStore.gitStatus`,
// a singleton NOT keyed by wsId, whenever the review-files summary hasn't
// loaded for the given wsId — which is always true for `wsId === null`) and
// prove GitPanel does not surface it.
let mockChangedFiles: Array<{ path: string; status: string; staged: boolean }> = []
vi.mock('@/features/git/hooks/use-sidebar-changed-files', () => ({
  useSidebarChangedFiles: () => ({ files: mockChangedFiles, uncommittedCount: 0 }),
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
    sel: (s: {
      repos: Array<{ workspaces: Array<{ id: string } & NonNullable<MockWs>> }>
    }) => unknown,
  ) => sel({ repos: mockActiveWs ? [{ workspaces: [{ id: 'ws-active', ...mockActiveWs }] }] : [] }),
}))

// The route is mocked only to prove GitPanel ignores it: scope comes from the
// focused workspace context.
let mockPathname = '/ide/proj1/repo1/ws-active'
vi.mock('@tanstack/react-router', () => ({
  useRouterState: ({ select }: { select: (s: { location: { pathname: string } }) => unknown }) =>
    select({ location: { pathname: mockPathname } }),
}))
const { openBranchReviewForWorkspace } = vi.hoisted(() => ({
  openBranchReviewForWorkspace: vi.fn(),
}))
vi.mock('@/features/panes/utils/pane-command-actions', () => ({ openBranchReviewForWorkspace }))

const REPO_CTX: FocusedWorkspaceContext = {
  projectId: 'proj1',
  workspaceId: 'ws-active',
  isProjectHome: false,
  repoId: 'repo1',
  repoPath: '/repo1',
  rootPath: '/repo1',
}
const HOME_CTX: FocusedWorkspaceContext = {
  projectId: 'proj1',
  workspaceId: 'ws-home',
  isProjectHome: true,
  repoId: null,
  repoPath: null,
  rootPath: '/proj1',
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('GitPanel', () => {
  beforeEach(() => {
    mockGitStatus = null
    mockActiveWs = null
    mockPathname = '/ide/proj1/repo1/ws-active'
    publishFocusedWorkspaceContext(REPO_CTX)
    mockChangedFiles = []
    vi.clearAllMocks()
  })

  it('renders the "Review this branch" row and the "Changed — n files" heading, not Changes/History tabs', () => {
    render(<GitPanel />)
    expect(screen.queryByRole('tab', { name: /changes/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('tab', { name: /history/i })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Review this branch/i })).toBeInTheDocument()
    expect(screen.getByText('Changed — 0 files')).toBeInTheDocument()
  })

  it('History is folded below the required sections, behind a disclosure', () => {
    render(<GitPanel />)
    expect(screen.queryByTestId('git-history')).not.toBeInTheDocument()
    const historyToggle = screen.getByRole('button', { name: /^History$/i })
    fireEvent.click(historyToggle)
    expect(screen.getByTestId('git-history')).toBeInTheDocument()
  })

  it('renders the changed-files tree and the unified branch section', () => {
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
    mockActiveWs = {
      branch: 'epoch/first-pr',
      parentBranch: 'develop',
      canMergeLocally: true,
      status: 'new',
    }
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

  /**
   * Live-reported: viewing a project-home-scoped chat (no workspace of its
   * own) still showed the git review panel, and it showed git context —
   * branch conflicts, a changed-files list — belonging to a COMPLETELY
   * DIFFERENT chat open elsewhere in the app. `wsId` here comes straight off
   * the URL pathname (`parseWorkspaceScopeFromPath`), which resolves to null
   * on the project-home route (`/ide/:projectId/home` has no repoId/wsId
   * segments) — but the panel used to render "Review this branch"/"Changed —
   * n files" unconditionally, backed by `useSidebarChangedFiles`, whose own
   * doc says it falls back to `useGitStore.gitStatus` — a singleton NOT keyed
   * by wsId — whenever the wsId-scoped summary hasn't loaded, which is always
   * true for `wsId === null`. That singleton is only reset when SOME
   * workspace becomes WorkspaceHost's active slot; project home does not
   * always win that race, so the previous real workspace's status can outlive
   * the switch.
   */
  describe('project home focused (no repo of its own)', () => {
    beforeEach(() => {
      mockPathname = '/ide/proj1/repo1/ws-active'
      publishFocusedWorkspaceContext(HOME_CTX)
    })

    it("shows no git content at all rather than another workspace's stale status", () => {
      // The exact leak reported: a DIFFERENT chat's real, non-empty git state
      // still sitting in the global store.
      mockGitStatus = {
        branch: 'feature/other-repo-branch',
        ahead: 3,
        behind: 1,
        files: [{ path: 'stale-from-another-project.ts', status: 'modified', staged: false }],
      }
      mockChangedFiles = [
        { path: 'stale-from-another-project.ts', status: 'modified', staged: false },
      ]

      render(<GitPanel />)

      expect(screen.queryByRole('button', { name: /Review this branch/i })).not.toBeInTheDocument()
      expect(screen.queryByText(/^Changed —/)).not.toBeInTheDocument()
      expect(screen.queryByTestId('changed-files-tree')).not.toBeInTheDocument()
      expect(screen.queryByTestId('branch-section')).not.toBeInTheDocument()
      expect(screen.queryByText('stale-from-another-project.ts')).not.toBeInTheDocument()
      expect(screen.queryByText('feature/other-repo-branch')).not.toBeInTheDocument()
      expect(screen.getByText('No repository open')).toBeInTheDocument()
    })

    it('renders no branch pill even when the global git store still has one', () => {
      mockGitStatus = { branch: 'develop', ahead: 0, behind: 0, files: [] }
      render(<GitPanel />)
      expect(screen.queryByText('develop')).not.toBeInTheDocument()
    })
  })

  describe('home route with a branch-workspace pane focused (split)', () => {
    const BRANCH_CTX: FocusedWorkspaceContext = {
      projectId: 'proj1',
      workspaceId: 'ws-branch',
      isProjectHome: false,
      repoId: 'repo1',
      repoPath: '/worktrees/branch',
      rootPath: '/worktrees/branch',
    }
    beforeEach(() => {
      mockPathname = '/ide/proj1/home'
      publishFocusedWorkspaceContext(BRANCH_CTX)
    })

    it('scopes every section to the focused branch workspace, not the route', () => {
      mockGitStatus = { branch: 'feat/x', ahead: 1, behind: 0, files: [] }
      render(<GitPanel />)
      expect(screen.queryByText('No repository open')).not.toBeInTheDocument()
      expect(branchSectionProps).toHaveBeenCalledWith(
        expect.objectContaining({ wsId: 'ws-branch' }),
      )
      fireEvent.click(screen.getByRole('button', { name: /Review this branch/i }))
      expect(openBranchReviewForWorkspace).toHaveBeenCalledWith('ws-branch')
    })
  })
})
