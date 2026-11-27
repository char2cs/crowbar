import { describe, it, expect, vi, beforeEach } from 'vitest'
import { getIdentity } from '@/features/git/api/identity-api'

const { apiFetch } = vi.hoisted(() => ({ apiFetch: vi.fn() }))
vi.mock('@/lib/api/fetch', () => ({ apiFetch }))
vi.mock('@/lib/workspace-scope-url', () => ({
  identityBaseForWorkspace: (wsId: string) => `/v0/chats/chat-of-${wsId}/identity`,
}))

beforeEach(() => {
  vi.clearAllMocks()
})

describe('identity-api', () => {
  it('getIdentity GETs the chat-scoped identity route', async () => {
    const dto = { login: 'octocat', displayName: 'The Octocat', avatarUrl: 'https://x/a.png' }
    apiFetch.mockResolvedValue(dto)

    const result = await getIdentity('w1')

    expect(apiFetch).toHaveBeenCalledWith('/v0/chats/chat-of-w1/identity')
    expect(result).toEqual(dto)
  })
})
