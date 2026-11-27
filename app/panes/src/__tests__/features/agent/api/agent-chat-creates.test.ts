import { beforeEach, describe, expect, it, vi } from 'vitest'

const apiFetch = vi.fn()
vi.mock('@/lib/api/fetch', () => ({ apiFetch: (...a: unknown[]) => apiFetch(...a) }))
vi.mock('@/lib/workspace-scope', () => ({
  bindActiveWorkspaceId: () => {},
  getWorkspaceScope: (id: string) => {
    if (id === 'w1') return { projectId: 'p1', repoId: 'r1', wsId: 'w1' }
    if (id === 'ws-home') return { projectId: 'p1', repoId: '', wsId: 'ws-home' }
    return null
  },
  getOwningChatId: (id: string) => (id === 'w1' ? 'chat-w1' : null),
}))

import * as api from '@/features/agent/api/agent-chat-creates'

describe('agent-chat-creates', () => {
  beforeEach(() => apiFetch.mockReset())

  it('createChat sends the client-minted id under its operation key', async () => {
    apiFetch.mockResolvedValue({ id: 'c9' })
    await api.createChat('c9', 'op1', 'w1', 'codex')
    expect(apiFetch.mock.calls[0][1]).toMatchObject({ headers: { 'Idempotency-Key': 'op1' } })
  })

  it('createChat POSTs the provider and returns the new id', async () => {
    apiFetch.mockResolvedValue({ id: 'c9' })
    const id = await api.createChat('c9', 'op1', 'w1', 'codex')
    expect(id).toBe('c9')
    const [url, init] = apiFetch.mock.calls[0]
    expect(url).toBe('/v0/projects/p1/repos/r1/chats')
    expect(init).toMatchObject({
      method: 'POST',
      body: JSON.stringify({ id: 'c9', provider: 'codex', parentId: '', workspaceId: 'w1' }),
    })
  })

  it('createChat sends workspaceId in the body, not just the URL', async () => {
    apiFetch.mockResolvedValue({ id: 'c9' })
    await api.createChat('c9', 'op1', 'w1', 'codex')
    const body = JSON.parse((apiFetch.mock.calls[0][1] as RequestInit).body as string)
    expect(body.workspaceId).toBe('w1')
  })

  it('createChat carries the PARENT, so the edge exists before the runner starts', async () => {
    apiFetch.mockResolvedValue({ id: 'c9' })
    await api.createChat('c9', 'op1', 'w1', 'codex', 'parent-chat')
    expect(apiFetch.mock.calls[0][1]).toMatchObject({
      body: JSON.stringify({
        id: 'c9',
        provider: 'codex',
        parentId: 'parent-chat',
        workspaceId: 'w1',
      }),
    })
    expect(apiFetch).toHaveBeenCalledTimes(1)
  })

  it('createChat carries the SURFACE the chat is born on', async () => {
    apiFetch.mockResolvedValue({ id: 'c9' })
    await api.createChat('c9', 'op1', 'w1', 'codex', '', 'terminal')
    const body = JSON.parse((apiFetch.mock.calls[0][1] as RequestInit).body as string)
    expect(body.surface).toBe('terminal')
  })

  it('createChat omits surface entirely when none is asked for', async () => {
    apiFetch.mockResolvedValue({ id: 'c9' })
    await api.createChat('c9', 'op1', 'w1', 'codex')
    const body = JSON.parse((apiFetch.mock.calls[0][1] as RequestInit).body as string)
    expect('surface' in body).toBe(false)
  })

  it('createChatWithOwnWorktree POSTs ownWorktree:true with no workspaceId, and returns the chat and its workspace', async () => {
    apiFetch.mockResolvedValue({ id: 'c9', workspaceId: 'ws9' })
    const created = await api.createChatWithOwnWorktree('c9', 'op1', 'p1', 'r1', 'codex', 'home-1')
    expect(created).toEqual({ id: 'c9', workspaceId: 'ws9' })
    const [url, init] = apiFetch.mock.calls[0]
    expect(url).toBe('/v0/projects/p1/repos/r1/chats')
    expect(init).toMatchObject({
      method: 'POST',
      body: JSON.stringify({ id: 'c9', provider: 'codex', parentId: 'home-1', ownWorktree: true }),
    })
    const body = JSON.parse((init as RequestInit).body as string)
    expect(body).not.toHaveProperty('workspaceId')
  })

  it('createChatWithOwnWorktree defaults parentId to the panel root', async () => {
    apiFetch.mockResolvedValue({ id: 'c9' })
    await api.createChatWithOwnWorktree('c9', 'op1', 'p1', 'r1', 'codex')
    expect(apiFetch.mock.calls[0][1]).toMatchObject({
      body: JSON.stringify({ id: 'c9', provider: 'codex', parentId: '', ownWorktree: true }),
    })
  })

  it('createChatWithOwnWorktree sends the caller-supplied branch name', async () => {
    apiFetch.mockResolvedValue({ id: 'c9' })
    await api.createChatWithOwnWorktree(
      'c9',
      'op1',
      'p1',
      'r1',
      'codex',
      'home-1',
      'feature/typed-name',
    )
    const body = JSON.parse((apiFetch.mock.calls[0][1] as RequestInit).body as string)
    expect(body).toMatchObject({ branch: 'feature/typed-name' })
  })

  it('createChatWithOwnWorktree omits branch entirely when not given', async () => {
    apiFetch.mockResolvedValue({ id: 'c9' })
    await api.createChatWithOwnWorktree('c9', 'op1', 'p1', 'r1', 'codex', 'home-1')
    const body = JSON.parse((apiFetch.mock.calls[0][1] as RequestInit).body as string)
    expect(body).not.toHaveProperty('branch')
  })
})
