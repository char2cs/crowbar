import { describe, it, expect, vi, beforeEach } from 'vitest'
import { pushChanges, pullChanges } from '@/features/git/api/git-remotes-api'

const { apiFetch } = vi.hoisted(() => ({ apiFetch: vi.fn() }))
vi.mock('@/lib/api/fetch', () => ({ apiFetch }))
// git is addressed through the chat holding the worktree now, never through the
// workspace: the api module resolves the base via gitBaseForWorkspace.
vi.mock('@/lib/workspace-scope-url', () => ({
  gitBaseForWorkspace: (wsId: string) => `/v0/chats/chat-of-${wsId}/git`,
}))

const reply = { operation: { id: 'op1' } }

beforeEach(() => {
  vi.clearAllMocks()
  apiFetch.mockResolvedValue(reply)
})

describe('git-remotes-api remote ops', () => {
  it('pushChanges POSTs to the chat-scoped push route under the operation id', async () => {
    const res = await pushChanges('w1', 'op1')
    expect(apiFetch).toHaveBeenCalledWith('/v0/chats/chat-of-w1/git/push', {
      method: 'POST',
      headers: { 'Idempotency-Key': 'op1' },
    })
    expect(res).toBe(reply)
  })

  it('pullChanges POSTs to the chat-scoped pull route under the operation id', async () => {
    const res = await pullChanges('w1', 'op2')
    expect(apiFetch).toHaveBeenCalledWith('/v0/chats/chat-of-w1/git/pull', {
      method: 'POST',
      headers: { 'Idempotency-Key': 'op2' },
    })
    expect(res).toBe(reply)
  })

  it('a refused request rejects with the refusal', async () => {
    apiFetch.mockRejectedValueOnce(new Error('409 conflict'))
    await expect(pullChanges('w1', 'op3')).rejects.toThrow('409 conflict')
  })
})
