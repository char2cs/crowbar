import { describe, it, expect, beforeEach, vi } from 'vitest'
import { resetDB } from '@/lib/persistence/idb'
import { dataOf } from '@/lib/loadable'

vi.mock('@/lib/api', () => ({
  apiFetch: vi.fn(async (path: string) =>
    path.includes('/diff') ? { files: [] } : [{ id: 'c1', title: 'chat', age: '2h', isActive: true }]),
}))

beforeEach(() => { resetDB() })

describe('branch-review data store', () => {
  it('fetches diff and chats together', async () => {
    const { useBranchReviewDataStore } = await import('@/features/branch-review/stores/branch-review-data-store')
    await useBranchReviewDataStore.getState().fetch('ws3')
    const data = dataOf(useBranchReviewDataStore.getState().data)
    expect(data?.diff).toEqual({ files: [] })
    expect(data?.chats).toHaveLength(1)
  })
})
