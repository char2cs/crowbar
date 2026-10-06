import type { StateCreator } from 'zustand'
import { frontendTrace } from '@/utils/frontend-trace'
import type { GitCommit, GitDiff, GitStatus } from '@/features/git/types/git-types'
import type { WorkspaceState } from '../workspace-store.types'

const MAX_WORKSPACE_GIT_STATUS_FILES = 200

// The git API modules load on first use: the store is on the boot path, a git fetch is not.
const gitDataApi = () => import('@/features/git/api/git-data-api')
const gitCommitsApi = () => import('@/features/git/api/git-commits-api')
const gitStatusApi = () => import('@/features/git/api/git-status-api')

/** `idle` until the workspace is first shown; only `ready` data is kept across switches. */
type GitLoadStatus = 'idle' | 'loading' | 'ready' | 'failed'

export interface GitSlice {
  gitLoad: GitLoadStatus
  gitStatus: GitStatus | null
  /** `gitStatus` capped for the file-explorer decorations. */
  workspaceGitStatus: GitStatus | null
  commits: GitCommit[]
  /** The sidebar's changed-files list (branch-review summary); null until loaded. */
  reviewFiles: GitDiff[] | null
  hasMoreCommits: boolean
  isLoadingMoreCommits: boolean
  gitActions: {
    loadGitData(): Promise<void>
    reloadReviewFiles(): Promise<void>
    reloadStatusAndLog(): Promise<void>
    loadMoreCommits(): Promise<void>
  }
}

function toWorkspaceGitStatus(status: GitStatus | null): GitStatus | null {
  if (!status || status.files.length <= MAX_WORKSPACE_GIT_STATUS_FILES) return status

  frontendTrace('warn', 'workspace-git', 'truncate', {
    totalFiles: status.files.length,
    keptFiles: MAX_WORKSPACE_GIT_STATUS_FILES,
  })
  return { ...status, files: status.files.slice(0, MAX_WORKSPACE_GIT_STATUS_FILES) }
}

export const createGitSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  GitSlice
> = (set, get) => ({
  gitLoad: 'idle',
  gitStatus: null,
  workspaceGitStatus: null,
  commits: [],
  reviewFiles: null,
  hasMoreCommits: true,
  isLoadingMoreCommits: false,

  // Object-form writes throughout: immer would deep-freeze status and commit arrays.
  gitActions: {
    async loadGitData() {
      const { gitLoad, workspaceId } = get()
      if (gitLoad === 'loading' || gitLoad === 'ready') return
      set({ gitLoad: 'loading' })
      try {
        const { fetchGitData, COMMITS_PER_PAGE } = await gitDataApi()
        const { status, commits, reviewFiles } = await fetchGitData(workspaceId)
        set({
          gitLoad: 'ready',
          reviewFiles,
          gitStatus: status,
          workspaceGitStatus: toWorkspaceGitStatus(status),
          commits,
          hasMoreCommits: commits.length >= COMMITS_PER_PAGE,
        })
      } catch {
        set({ gitLoad: 'failed' })
      }
    },

    // Skips the write when nothing changed so an identical reload never churns
    // the memoized changed-files tree.
    async reloadReviewFiles() {
      const [{ fetchReviewFiles }, { default: deepEqual }] = await Promise.all([
        gitDataApi(),
        import('fast-deep-equal'),
      ])
      const next = await fetchReviewFiles(get().workspaceId)
      if (next && !deepEqual(next, get().reviewFiles)) set({ reviewFiles: next })
    },

    // Status AND the log together: a terminal-side commit or soft reset changes
    // the log with no UI action, and a reset can remove commits, so the fresh
    // first page replaces the list.
    async reloadStatusAndLog() {
      const wsId = get().workspaceId
      const [{ getGitStatus }, { getGitLog }, { COMMITS_PER_PAGE }] = await Promise.all([
        gitStatusApi(),
        gitCommitsApi(),
        gitDataApi(),
      ])
      const [status, commits] = await Promise.all([
        getGitStatus(wsId),
        getGitLog(wsId, COMMITS_PER_PAGE, 0),
      ])
      set({
        gitStatus: status,
        workspaceGitStatus: toWorkspaceGitStatus(status),
        commits,
        hasMoreCommits: commits.length >= COMMITS_PER_PAGE,
      })
    },

    async loadMoreCommits() {
      const { commits, hasMoreCommits, isLoadingMoreCommits, workspaceId } = get()
      if (!hasMoreCommits || isLoadingMoreCommits) return
      set({ isLoadingMoreCommits: true })
      try {
        const [{ getGitLog }, { COMMITS_PER_PAGE }] = await Promise.all([
          gitCommitsApi(),
          gitDataApi(),
        ])
        const next = await getGitLog(workspaceId, COMMITS_PER_PAGE, commits.length)
        const known = new Set(commits.map((c) => c.hash))
        const fresh = next.filter((c) => !known.has(c.hash))
        set(
          fresh.length > 0
            ? {
                commits: [...commits, ...fresh],
                hasMoreCommits: fresh.length >= COMMITS_PER_PAGE,
              }
            : { hasMoreCommits: false },
        )
      } finally {
        set({ isLoadingMoreCommits: false })
      }
    },
  },
})
