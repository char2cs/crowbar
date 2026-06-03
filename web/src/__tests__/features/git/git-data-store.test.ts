import { describe, it, expect, beforeEach, vi } from 'vitest'
import { resetDB } from '@/lib/persistence/idb'
import { dataOf } from '@/lib/loadable'

vi.mock('@/features/git/api/git-status-api', () => ({ getGitStatus: vi.fn(async () => ({ branch: 'main', files: [] })) }))
vi.mock('@/features/git/api/git-commits-api', () => ({ getGitLog: vi.fn(async () => [{ hash: 'abc', message: 'm', date: '2026-01-01' }]) }))
vi.mock('@/features/git/api/git-branches-api', () => ({ getBranches: vi.fn(async () => ['main']) }))
vi.mock('@/features/git/api/git-stash-api', () => ({ getStashes: vi.fn(async () => []) }))

beforeEach(() => { resetDB() })

describe('git data store', () => {
  it('fetchGitData aggregates status/commits/branches/stashes into loadable', async () => {
    const { useGitStore } = await import('@/features/git/stores/git-store')
    await useGitStore.getState().fetchGitData('/repo')
    const data = dataOf(useGitStore.getState().gitData)
    expect(data?.status?.branch).toBe('main')
    expect(data?.commits).toHaveLength(1)
    expect(data?.branches).toEqual(['main'])
  })
})
