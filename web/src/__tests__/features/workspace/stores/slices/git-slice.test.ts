import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

const { getGitStatus, getGitLog, getReviewFiles } = vi.hoisted(() => ({
  getGitStatus: vi.fn(),
  getGitLog: vi.fn(),
  getReviewFiles: vi.fn(),
}))
vi.mock('@/features/git/api/git-status-api', () => ({ getGitStatus }))
vi.mock('@/features/git/api/git-commits-api', () => ({ getGitLog }))
vi.mock('@/features/git/api/review-api', () => ({ getReviewFiles }))

const commit = (hash: string) => ({ hash, message: hash, author: 'a', date: '2026-01-01' })

beforeEach(() => {
  vi.clearAllMocks()
  getGitStatus.mockResolvedValue({ branch: 'main', files: [] })
  getGitLog.mockResolvedValue([commit('abc')])
  getReviewFiles.mockResolvedValue([
    { path: 'a.ts', status: 'modified', additions: 1, deletions: 0 },
  ])
})

describe('git slice', () => {
  it('loads status and the first commit page into the owning workspace only', async () => {
    const a = createWorkspaceStore('ws-A')
    const b = createWorkspaceStore('ws-B')

    await a.getState().gitActions.loadGitData()

    expect(getGitStatus).toHaveBeenCalledWith('ws-A')
    expect(a.getState().gitLoad).toBe('ready')
    expect(a.getState().gitStatus?.branch).toBe('main')
    expect(a.getState().commits.map((c) => c.hash)).toEqual(['abc'])
    expect(b.getState().gitLoad).toBe('idle')
    expect(b.getState().gitStatus).toBeNull()
    expect(b.getState().commits).toEqual([])
  })

  it('does not refetch data it already holds', async () => {
    const a = createWorkspaceStore('ws-A')
    await a.getState().gitActions.loadGitData()
    await a.getState().gitActions.loadGitData()
    expect(getGitStatus).toHaveBeenCalledTimes(1)
  })

  it('goes back to retryable after a failed load', async () => {
    const a = createWorkspaceStore('ws-A')
    getGitStatus.mockRejectedValueOnce(new Error('boom'))
    await a.getState().gitActions.loadGitData()
    expect(a.getState().gitLoad).toBe('failed')

    await a.getState().gitActions.loadGitData()
    expect(a.getState().gitLoad).toBe('ready')
  })

  // A commit made in the integrated terminal arrives only as a git push; a soft
  // reset removes commits, so the reload must replace the list, not merge.
  it('reloadStatusAndLog replaces the commit list', async () => {
    const a = createWorkspaceStore('ws-A')
    await a.getState().gitActions.loadGitData()
    getGitLog.mockResolvedValueOnce([commit('fresh-1')])

    await a.getState().gitActions.reloadStatusAndLog()

    expect(a.getState().commits.map((c) => c.hash)).toEqual(['fresh-1'])
    expect(a.getState().hasMoreCommits).toBe(false)
  })

  it('loadMoreCommits appends the next page without duplicates', async () => {
    const a = createWorkspaceStore('ws-A')
    const page = Array.from({ length: 50 }, (_, i) => commit(`c${i}`))
    getGitLog.mockResolvedValueOnce(page)
    await a.getState().gitActions.loadGitData()
    expect(a.getState().hasMoreCommits).toBe(true)

    getGitLog.mockResolvedValueOnce([commit('c49'), commit('older')])
    await a.getState().gitActions.loadMoreCommits()

    expect(getGitLog).toHaveBeenLastCalledWith('ws-A', 50, 50)
    expect(a.getState().commits).toHaveLength(51)
    expect(a.getState().hasMoreCommits).toBe(false)
  })

  it('truncates the file-explorer decoration status but not the panel status', async () => {
    const files = Array.from({ length: 250 }, (_, i) => ({ path: `f${i}`, status: 'modified' }))
    getGitStatus.mockResolvedValueOnce({ branch: 'main', files })
    const a = createWorkspaceStore('ws-A')

    await a.getState().gitActions.loadGitData()

    expect(a.getState().gitStatus?.files).toHaveLength(250)
    expect(a.getState().workspaceGitStatus?.files).toHaveLength(200)
  })

  // The sidebar's changed-files list is part of the workspace's git slice, so a
  // switch back to a workspace shows it without refetching.
  it('loads the branch-review file summary with the git data, per workspace', async () => {
    const a = createWorkspaceStore('ws-A')
    const b = createWorkspaceStore('ws-B')
    expect(a.getState().reviewFiles).toBeNull()

    await a.getState().gitActions.loadGitData()

    expect(getReviewFiles).toHaveBeenCalledWith({ wsId: 'ws-A' })
    expect(a.getState().reviewFiles?.map((f) => f.file_path)).toEqual(['a.ts'])
    expect(b.getState().reviewFiles).toBeNull()
  })

  it('still loads git data when the review summary fails', async () => {
    getReviewFiles.mockRejectedValueOnce(new Error('boom'))
    const a = createWorkspaceStore('ws-A')
    await a.getState().gitActions.loadGitData()
    expect(a.getState().gitLoad).toBe('ready')
    expect(a.getState().reviewFiles).toBeNull()
  })

  it('reloadReviewFiles keeps the same array when the summary is unchanged', async () => {
    const a = createWorkspaceStore('ws-A')
    await a.getState().gitActions.loadGitData()
    const before = a.getState().reviewFiles

    await a.getState().gitActions.reloadReviewFiles()
    expect(a.getState().reviewFiles).toBe(before)

    getReviewFiles.mockResolvedValueOnce([])
    await a.getState().gitActions.reloadReviewFiles()
    expect(a.getState().reviewFiles).toEqual([])
  })
})
