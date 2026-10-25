import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { GitPanel } from '@/features/git/components/git-panel'
import type { GitDiff, GitFile, GitStatus } from '@/features/git/types/git-types'
import {
  publishFocusedWorkspaceContext,
  type FocusedWorkspaceContext,
} from '@/features/window/stores/focused-workspace-context-store'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import type { BranchReviewContent } from '@/features/panes/types/pane-content'
import { getInitialState, useSidebarStore, type Workspace } from '@/lib/store/sidebar'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'

vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))

// jsdom has no layout, so the virtualised tree needs a sized rect to mount rows.
const originalGetBoundingClientRect = HTMLElement.prototype.getBoundingClientRect
const RECT = { top: 0, left: 0, right: 280, bottom: 800, width: 280, height: 800, x: 0, y: 0 }

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
const BRANCH_CTX: FocusedWorkspaceContext = {
  projectId: 'proj1',
  workspaceId: 'ws-branch',
  isProjectHome: false,
  repoId: 'repo1',
  repoPath: '/worktrees/branch',
  rootPath: '/worktrees/branch',
}

function diff(path: string): GitDiff {
  return {
    file_path: path,
    is_new: false,
    is_deleted: false,
    is_renamed: false,
    lines: [],
    additions: 2,
    deletions: 1,
  }
}
function gitFile(path: string): GitFile {
  return { path, status: 'modified', staged: false } as GitFile
}
function status(over: Partial<GitStatus> = {}): GitStatus {
  return { branch: 'develop', ahead: 0, behind: 0, files: [], ...over }
}
function thread(filePath: string, isResolved: boolean): ReviewThread {
  return {
    id: `${filePath}-${isResolved}`,
    filePath,
    lineNumber: 1,
    startLine: 1,
    endLine: 1,
    side: 'new',
    messages: [],
    isResolved,
  }
}

const FILES = [diff('a.ts'), diff('b.ts')]

/** The registered store for `wsId`, seeded through its public state. */
function seedWorkspace(
  wsId: string,
  seed: { files?: GitDiff[]; gitStatus?: GitStatus; threads?: ReviewThread[] } = {},
): WorkspaceStore {
  const store = getOrCreateWorkspaceStore(wsId)
  store.setState((s) => ({
    reviewFiles: seed.files ?? null,
    gitStatus: seed.gitStatus ?? null,
    branchReview: { ...s.branchReview, threads: seed.threads ?? [] },
  }))
  return store
}

function reviewBuffer(): BranchReviewContent | undefined {
  return windowPaneStore
    .getState()
    .buffers.find((b): b is BranchReviewContent => b.type === 'branchReview')
}

const user = () => userEvent.setup()

beforeEach(() => {
  HTMLElement.prototype.getBoundingClientRect = function getBoundingClientRect() {
    return { ...RECT, toJSON: () => RECT } as DOMRect
  }
  vi.clearAllMocks()
  resetWindowPaneStoreForTests()
  useSidebarStore.setState({ ...getInitialState(), activeTab: 'git' })
  publishFocusedWorkspaceContext(REPO_CTX)
})

afterEach(() => {
  HTMLElement.prototype.getBoundingClientRect = originalGetBoundingClientRect
  for (const id of ['ws-active', 'ws-home', 'ws-branch']) destroyWorkspaceStore(id)
})

describe('GitPanel', () => {
  it('is a navigator: no History, no "Review this branch" row, a Changed heading', () => {
    seedWorkspace('ws-active', { files: [diff('a.ts')] })
    render(<GitPanel />)
    expect(screen.queryByRole('button', { name: /^History$/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Review this branch/i })).not.toBeInTheDocument()
    expect(screen.getByText('Changed — 1 file')).toBeInTheDocument()
  })

  describe('file navigation', () => {
    it('has no review progress card, only the tree', () => {
      seedWorkspace('ws-active', { files: FILES, threads: [thread('a.ts', false)] })
      render(<GitPanel />)
      expect(screen.queryByText('Reviewing')).not.toBeInTheDocument()
      expect(screen.queryByText(/open thread/)).not.toBeInTheDocument()
    })

    it('opens a clicked file on Files changed and reveals it by path', async () => {
      const store = seedWorkspace('ws-active', { files: FILES })
      render(<GitPanel />)
      await user().click(await screen.findByText('b.ts'))
      expect(reviewBuffer()?.view).toBe('files')
      expect(store.getState().branchReview.revealFilePath).toBe('b.ts')
    })
  })

  describe('viewed-state load discipline', () => {
    function watchLoad(files: GitDiff[]) {
      const store = seedWorkspace('ws-active', { files })
      const load = vi.fn(() => Promise.resolve())
      store.setState({ loadBranchReviewViewed: load })
      return load
    }

    it('stays inert while another sidebar mode is shown', () => {
      useSidebarStore.setState({ activeTab: 'chats' })
      const load = watchLoad([diff('a.ts')])
      render(<GitPanel />)
      expect(load).not.toHaveBeenCalled()
    })

    it('stays inert with nothing to mark', () => {
      const load = watchLoad([])
      render(<GitPanel />)
      expect(load).not.toHaveBeenCalled()
    })

    it('loads once the tree is shown with files', () => {
      const load = watchLoad([diff('a.ts')])
      render(<GitPanel />)
      expect(load).toHaveBeenCalled()
    })
  })

  it('renders the changed-files tree with no commit box or merge controls', () => {
    seedWorkspace('ws-active', {
      files: [diff('a.ts')],
      gitStatus: status({ files: [gitFile('a.ts')], ahead: 1, behind: 1 }),
    })
    render(<GitPanel />)
    expect(screen.getByText('a.ts')).toBeInTheDocument()
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /commit|pull request|merge|push|pull|rebase/i }),
    ).not.toBeInTheDocument()
  })

  it('puts the merge card right below the branch card, only for a known workspace', () => {
    seedWorkspace('ws-active', { gitStatus: status({ branch: 'epoch/first-pr' }) })
    render(<GitPanel />)
    expect(screen.queryByRole('region', { name: 'Merge' })).not.toBeInTheDocument()
    cleanup()

    useSidebarStore.setState({
      repos: [
        {
          id: 'repo1',
          name: 'repo1',
          avatarLabel: 'R',
          avatarColor: 'bg-indigo-700',
          workspaces: [
            { id: 'ws-active', branch: 'epoch/first-pr', parentBranch: 'develop', age: '' },
          ],
        },
      ],
    })
    render(<GitPanel />)
    const card = screen.getByRole('region', { name: 'Merge' })
    const branch = screen.getByText('epoch/first-pr')
    expect(branch.compareDocumentPosition(card) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(screen.getByText('Merge into develop')).toBeInTheDocument()
  })

  it('shows the branch card with its parent', () => {
    seedWorkspace('ws-active', { gitStatus: status({ branch: 'epoch/first-pr' }) })
    useSidebarStore.setState({
      repos: [
        {
          id: 'repo1',
          name: 'repo1',
          avatarLabel: 'R',
          avatarColor: 'bg-indigo-700',
          workspaces: [
            {
              id: 'ws-active',
              branch: 'epoch/first-pr',
              parentBranch: 'develop',
              status: 'new',
              age: '',
            } as Workspace,
          ],
        },
      ],
    })
    render(<GitPanel />)
    expect(screen.getByText('epoch/first-pr')).toBeInTheDocument()
    expect(screen.getByText('develop')).toBeInTheDocument()
  })

  describe('project home focused (no repo of its own)', () => {
    beforeEach(() => publishFocusedWorkspaceContext(HOME_CTX))

    it("shows no git content at all rather than another workspace's stale status", () => {
      // Another workspace's real, non-empty git state must not leak in.
      const stale = diff('stale-from-another-project.ts')
      seedWorkspace('ws-active', {
        files: [stale],
        gitStatus: status({
          branch: 'feature/other-repo-branch',
          files: [gitFile(stale.file_path)],
        }),
      })
      seedWorkspace('ws-home', {
        files: [stale],
        gitStatus: status({
          branch: 'feature/other-repo-branch',
          files: [gitFile(stale.file_path)],
        }),
      })

      render(<GitPanel />)

      expect(screen.queryByText(/^Changed —/)).not.toBeInTheDocument()
      expect(screen.queryByText('stale-from-another-project.ts')).not.toBeInTheDocument()
      expect(screen.queryByText('feature/other-repo-branch')).not.toBeInTheDocument()
      expect(screen.getByText('No repository open')).toBeInTheDocument()
    })

    it('renders no branch pill even when a workspace store still has one', () => {
      seedWorkspace('ws-home', { gitStatus: status() })
      render(<GitPanel />)
      expect(screen.queryByText('develop')).not.toBeInTheDocument()
    })
  })

  describe('home route with a branch-workspace pane focused (split)', () => {
    it('scopes every section to the focused branch workspace', () => {
      publishFocusedWorkspaceContext(BRANCH_CTX)
      const store = seedWorkspace('ws-branch', {
        gitStatus: status({ branch: 'feat/x', ahead: 1 }),
      })
      const load = vi.fn(() => Promise.resolve())
      store.setState({ loadBranchReviewViewed: load })
      render(<GitPanel />)
      expect(screen.queryByText('No repository open')).not.toBeInTheDocument()
      expect(screen.getByText('feat/x')).toBeInTheDocument()
      expect(load).not.toHaveBeenCalled()
    })
  })
})
