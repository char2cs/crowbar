import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/lib/errors/api-error'

const apiFetch = vi.fn()
const apiFetchRaw = vi.fn()
vi.mock('@/lib/api/fetch', () => ({
  apiFetch: (...a: unknown[]) => apiFetch(...a),
  apiFetchRaw: (...a: unknown[]) => apiFetchRaw(...a),
}))
vi.mock('@/lib/workspace-scope', () => ({
  bindActiveWorkspaceId: () => {},
  getWorkspaceScope: (id: string) => {
    if (id === 'w1') return { projectId: 'p1', repoId: 'r1', wsId: 'w1' }
    if (id === 'ws-home') return { projectId: 'p1', repoId: '', wsId: 'ws-home' }
    return null
  },
  getOwningChatId: (id: string) => (id === 'w1' ? 'chat-w1' : null),
}))

import * as api from '@/features/agent/api/agent-chat-reads'
import { resetChatStore, writeChat } from '@/__tests__/__fixtures__/agent-chat'

describe('agent-chat-reads: chatBase URL shape', () => {
  beforeEach(() => {
    apiFetch.mockReset()
    apiFetchRaw.mockReset()
  })

  it('builds the repo-scoped chat URL for a non-home workspace', () => {
    expect(api.chatBase('w1')).toBe('/v0/projects/p1/repos/r1/chats')
  })

  it('keeps the home mount unchanged for a home workspace', () => {
    expect(api.chatBase('ws-home')).toBe('/v0/projects/p1/home/chats')
  })
})

describe('agent-chat-reads', () => {
  beforeEach(() => {
    apiFetch.mockReset()
    resetChatStore()
    writeChat({ id: 'chat/1' })
    writeChat({ id: 'c1' })
  })

  it('reads a payload a row names by its blob ref, and null once it is swept', async () => {
    apiFetchRaw
      .mockResolvedValueOnce(new Response('full output'))
      .mockRejectedValueOnce(new ApiError('gone', 404))
    const controller = new AbortController()
    await expect(api.readBlob('local', 'chat/1', 'sha256:ab', controller.signal)).resolves.toBe(
      'full output',
    )
    expect(apiFetchRaw).toHaveBeenCalledWith('/v0/chats/chat%2F1/blobs/sha256%3Aab', {
      signal: controller.signal,
    })
    await expect(api.readBlob('local', 'c1', 'sha256:gone')).resolves.toBeNull()
  })

  it("reads a blob from its chat's host", async () => {
    apiFetchRaw.mockResolvedValueOnce(new Response('remote output'))
    await expect(api.readBlob('host-1', 'c1', 'sha256:ab')).resolves.toBe('remote output')
    expect(apiFetchRaw).toHaveBeenCalledWith('crowbar://host-1/v0/chats/c1/blobs/sha256%3Aab', {
      signal: undefined,
    })
  })

  it('throws on a blob read that failed for any other reason', async () => {
    apiFetchRaw.mockRejectedValueOnce(new ApiError('boom', 500))
    await expect(api.readBlob('local', 'c1', 'sha256:ab')).rejects.toThrow('boom')
  })

  it('loads a cancellable slash catalog with no read retry/cache layer', async () => {
    const controller = new AbortController()
    apiFetch.mockResolvedValue({
      providerId: 'claude',
      completeness: 'plugin_only',
      items: [],
      warnings: ['one plugin failed'],
    })
    const catalog = await api.getSlashCatalog('w1', 'c1', controller.signal)
    expect(catalog.warnings).toEqual(['one plugin failed'])
    expect(apiFetch).toHaveBeenCalledWith(
      '/v0/projects/p1/repos/r1/chats/c1/slash-catalog',
      { signal: controller.signal },
      { attempts: 1, baseDelayMs: 0, maxDelayMs: 0 },
    )
  })

  it('mapChat grounds an absent parent, order, model and effort', () => {
    const chat = api.mapChat({ id: 'c1', workspaceId: 'w1' } as never)
    expect(chat).toMatchObject({ parentId: '', order: 0, model: '', effort: '' })
  })
})
