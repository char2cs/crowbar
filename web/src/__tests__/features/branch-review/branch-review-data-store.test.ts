import { describe, it, expect, beforeEach, vi } from 'vitest'
import { resetDB } from '@/lib/persistence/idb'
import { dataOf } from '@/lib/loadable'

vi.mock('@/lib/api', () => ({
  apiFetch: vi.fn(async (path: string) =>
    path.includes('/diff') ? { files: [] } : [{ id: 'c1', title: 'chat', age: '2h', isActive: true }]),
}))

beforeEach(() => { resetDB() })

describe('branch-review data stores (decoupled)', () => {
  it('diff store fetches the diff independently', async () => {
    const { useBranchReviewDiffStore } = await import('@/features/branch-review/stores/branch-review-data-store')
    await useBranchReviewDiffStore.getState().fetch('ws3')
    expect(dataOf(useBranchReviewDiffStore.getState().data)).toEqual({ files: [] })
  })

  it('chats store fetches chats independently', async () => {
    const { useBranchReviewChatsStore } = await import('@/features/branch-review/stores/branch-review-data-store')
    await useBranchReviewChatsStore.getState().fetch('ws3')
    expect(dataOf(useBranchReviewChatsStore.getState().data)).toHaveLength(1)
  })

  it('a chats fetch failure does not affect the diff store', async () => {
    const api = await import('@/lib/api')
    const { useBranchReviewDiffStore, useBranchReviewChatsStore } = await import('@/features/branch-review/stores/branch-review-data-store')
    // diff succeeds, chats fails — diff must still resolve to success
    ;(api.apiFetch as ReturnType<typeof vi.fn>).mockImplementation(async (path: string) => {
      if (path.includes('/diff')) return { files: [] }
      throw new Error('500')
    })
    await useBranchReviewDiffStore.getState().fetch('ws3')
    await useBranchReviewChatsStore.getState().fetch('ws3')
    expect(useBranchReviewDiffStore.getState().data.status).toBe('success')
    expect(dataOf(useBranchReviewDiffStore.getState().data)).toEqual({ files: [] })
    expect(useBranchReviewChatsStore.getState().data.status).toBe('error')
  })
})
