import { describe, it, expect, vi, beforeEach } from 'vitest'
import { pushChanges, pullChanges, fetchChanges } from '@/features/git/api/git-remotes-api'

const { apiFetch } = vi.hoisted(() => ({ apiFetch: vi.fn() }))
vi.mock('@/lib/api', () => ({ apiFetch }))
vi.mock('@/lib/workspace-scope-url', () => ({
  workspaceBase: (wsId: string) => `/v0/ws/${wsId}`,
}))

beforeEach(() => {
  vi.clearAllMocks()
  apiFetch.mockResolvedValue(undefined) // daemon answers 202 with no body
})

describe('git-remotes-api remote ops', () => {
  it('pushChanges POSTs to the workspace push route', async () => {
    const res = await pushChanges('w1')
    expect(apiFetch).toHaveBeenCalledWith('/v0/ws/w1/git/push', { method: 'POST' })
    expect(res).toEqual({ success: true })
  })

  it('pullChanges POSTs to the workspace pull route', async () => {
    const res = await pullChanges('w1')
    expect(apiFetch).toHaveBeenCalledWith('/v0/ws/w1/git/pull', { method: 'POST' })
    expect(res).toEqual({ success: true })
  })

  it('fetchChanges POSTs to the workspace fetch route', async () => {
    const res = await fetchChanges('w1')
    expect(apiFetch).toHaveBeenCalledWith('/v0/ws/w1/git/fetch', { method: 'POST' })
    expect(res).toEqual({ success: true })
  })

  it('maps a rejected request to { success: false, error }', async () => {
    apiFetch.mockRejectedValueOnce(new Error('409 conflict'))
    const res = await pullChanges('w1')
    expect(res).toEqual({ success: false, error: '409 conflict' })
  })
})
