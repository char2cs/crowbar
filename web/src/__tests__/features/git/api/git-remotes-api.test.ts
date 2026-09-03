import { describe, it, expect, vi, beforeEach } from 'vitest'
import { pushChanges, pullChanges, fetchChanges } from '@/features/git/api/git-remotes-api'

const { apiFetch } = vi.hoisted(() => ({ apiFetch: vi.fn() }))
vi.mock('@/lib/api', () => ({ apiFetch }))
// git is addressed through the chat holding the worktree now, never through the
// workspace: the api module resolves the base via gitBaseForWorkspace.
vi.mock('@/lib/workspace-scope-url', () => ({
  gitBaseForWorkspace: (wsId: string) => `/v0/chats/chat-of-${wsId}/git`,
}))

beforeEach(() => {
  vi.clearAllMocks()
  apiFetch.mockResolvedValue(undefined) // daemon answers 202 with no body
})

describe('git-remotes-api remote ops', () => {
  it('pushChanges POSTs to the chat-scoped push route', async () => {
    const res = await pushChanges('w1')
    expect(apiFetch).toHaveBeenCalledWith('/v0/chats/chat-of-w1/git/push', { method: 'POST' })
    expect(res).toEqual({ success: true })
  })

  it('pullChanges POSTs to the chat-scoped pull route', async () => {
    const res = await pullChanges('w1')
    expect(apiFetch).toHaveBeenCalledWith('/v0/chats/chat-of-w1/git/pull', { method: 'POST' })
    expect(res).toEqual({ success: true })
  })

  it('fetchChanges POSTs to the chat-scoped fetch route', async () => {
    const res = await fetchChanges('w1')
    expect(apiFetch).toHaveBeenCalledWith('/v0/chats/chat-of-w1/git/fetch', { method: 'POST' })
    expect(res).toEqual({ success: true })
  })

  it('maps a rejected request to { success: false, error }', async () => {
    apiFetch.mockRejectedValueOnce(new Error('409 conflict'))
    const res = await pullChanges('w1')
    expect(res).toEqual({ success: false, error: '409 conflict' })
  })
})
