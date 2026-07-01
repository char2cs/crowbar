import { describe, it, expect, beforeEach, vi } from 'vitest'
import { resetDB } from '@/lib/persistence/idb'
import { dataOf } from '@/lib/loadable'

vi.mock('@/features/git/api/git-status-api', () => ({
  getGitStatus: vi.fn(async () => ({ branch: 'main', files: [] })),
}))
vi.mock('@/features/git/api/git-commits-api', () => ({
  getGitLog: vi.fn(async () => [{ hash: 'abc', message: 'm', date: '2026-01-01' }]),
}))
vi.mock('@/features/git/api/git-branches-api', () => ({ getBranches: vi.fn(async () => ['main']) }))
vi.mock('@/features/git/api/git-stash-api', () => ({ getStashes: vi.fn(async () => []) }))

beforeEach(() => {
  resetDB()
})

describe('git data store', () => {
  it('fetchGitData aggregates status/commits/branches/stashes into loadable', async () => {
    const { useGitStore } = await import('@/features/git/stores/git-store')
    await useGitStore.getState().fetchGitData('/repo')
    const data = dataOf(useGitStore.getState().gitData)
    expect(data?.status?.branch).toBe('main')
    expect(data?.commits).toHaveLength(1)
    expect(data?.branches).toEqual(['main'])
  })

  // BUG-020: a commit made in the integrated terminal arrives only as a git
  // WS push. The push-driven reload must refetch the log too, REPLACING the
  // commit list — a soft reset removes commits, so a merge-only update would
  // keep hashes that no longer exist.
  it('reloadStatusAndLog refetches status and replaces the commit list', async () => {
    const { useGitStore } = await import('@/features/git/stores/git-store')
    const { getGitLog } = await import('@/features/git/api/git-commits-api')

    useGitStore.getState().actions.setCommits([
      { hash: 'stale-1', message: 'gone after reset', author: 'a', date: '2026-01-01' },
      { hash: 'stale-2', message: 'old', author: 'a', date: '2026-01-01' },
    ] as never)

    vi.mocked(getGitLog).mockResolvedValueOnce([
      { hash: 'fresh-1', message: 'committed in terminal', author: 'a', date: '2026-06-10' },
    ] as never)

    await useGitStore.getState().actions.reloadStatusAndLog('/repo')

    const { commits, gitStatus, hasMoreCommits } = useGitStore.getState()
    expect(commits.map((c) => c.hash)).toEqual(['fresh-1'])
    expect(gitStatus?.branch).toBe('main')
    expect(hasMoreCommits).toBe(false)
  })
})
