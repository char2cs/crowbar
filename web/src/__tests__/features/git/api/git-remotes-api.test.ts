import { describe, it, expect, vi, beforeEach } from 'vitest'
import { pushChanges, pullChanges, fetchChanges } from '@/features/git/api/git-remotes-api'
import { ApiError } from '@/lib/api'

const { apiFetch } = vi.hoisted(() => ({ apiFetch: vi.fn() }))
// Preserve the real ApiError (so `instanceof ApiError` in gitRemoteOp works)
// while stubbing only apiFetch.
vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiFetch,
}))
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

  it('surfaces the not_fast_forwardable refusal as a code (409 + frozen error string)', async () => {
    apiFetch.mockRejectedValueOnce(new ApiError('not_fast_forwardable', 409))
    const res = await pullChanges('w1')
    expect(res).toEqual({ success: false, code: 'not_fast_forwardable' })
  })

  it('does NOT set the code for other 409s (message must match the contract)', async () => {
    apiFetch.mockRejectedValueOnce(new ApiError('some_other_conflict', 409))
    const res = await pullChanges('w1')
    expect(res).toEqual({ success: false, error: 'some_other_conflict' })
    expect(res.code).toBeUndefined()
  })

  it('does NOT set the code for non-ApiError failures', async () => {
    apiFetch.mockRejectedValueOnce(new Error('boom'))
    const res = await pullChanges('w1')
    expect(res).toEqual({ success: false, error: 'boom' })
    expect(res.code).toBeUndefined()
  })
})
