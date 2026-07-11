import { beforeEach, describe, expect, it, vi } from 'vitest'

const apiFetch = vi.fn()
vi.mock('@/lib/api', () => ({ apiFetch: (...a: unknown[]) => apiFetch(...a) }))
vi.mock('@/lib/workspace-scope-url', () => ({ workspaceBase: (id: string) => `/v0/ws/${id}` }))

import * as api from '@/features/agent/api/agent-api'

describe('agent-api', () => {
  beforeEach(() => apiFetch.mockReset())

  it('listChats GETs the workspace-scoped chats list', async () => {
    apiFetch.mockResolvedValue([
      {
        id: 'c1',
        workspaceId: 'w1',
        title: 'T',
        activeSegmentId: 's1',
        activeProviderId: 'claude',
        createdAt: '2026-01-01T00:00:00Z',
      },
    ])
    const chats = await api.listChats('w1')
    expect(apiFetch).toHaveBeenCalledWith('/v0/ws/w1/agent/chats')
    expect(chats[0]).toMatchObject({ id: 'c1', activeProviderId: 'claude' })
  })

  it('listChats returns [] when the backend responds with no body', async () => {
    apiFetch.mockResolvedValue(undefined)
    const chats = await api.listChats('w1')
    expect(chats).toEqual([])
  })

  it('getChat GETs the single chat and includes its segments', async () => {
    apiFetch.mockResolvedValue({
      id: 'c1',
      workspaceId: 'w1',
      title: 'T',
      activeSegmentId: 's1',
      activeProviderId: 'claude',
      createdAt: '2026-01-01T00:00:00Z',
      segments: [
        {
          id: 's1',
          providerId: 'claude',
          crowbarSegmentId: 'cs1',
          terminalSessionId: 't1',
          startedAt: '2026-01-01T00:00:00Z',
          status: 'active',
        },
      ],
    })
    const chat = await api.getChat('w1', 'c1')
    expect(apiFetch).toHaveBeenCalledWith('/v0/ws/w1/agent/chats/c1')
    expect(chat.segments).toHaveLength(1)
    expect(chat.segments[0]).toMatchObject({ id: 's1', status: 'active' })
  })

  it('getChat defaults segments to [] when the backend omits them', async () => {
    apiFetch.mockResolvedValue({
      id: 'c1',
      workspaceId: 'w1',
      title: 'T',
      activeSegmentId: 's1',
      activeProviderId: 'claude',
      createdAt: '2026-01-01T00:00:00Z',
    })
    const chat = await api.getChat('w1', 'c1')
    expect(chat.segments).toEqual([])
  })

  it('createChat POSTs the provider and returns the new id', async () => {
    apiFetch.mockResolvedValue({ id: 'c9' })
    const id = await api.createChat('w1', 'codex')
    expect(id).toBe('c9')
    const [url, init] = apiFetch.mock.calls[0]
    expect(url).toBe('/v0/ws/w1/agent/chats')
    expect(init).toMatchObject({ method: 'POST', body: JSON.stringify({ provider: 'codex' }) })
  })

  it('switchProvider POSTs to /switch and returns the new segment id', async () => {
    apiFetch.mockResolvedValue({ id: 'seg2' })
    const seg = await api.switchProvider('w1', 'c1', 'claude')
    expect(seg).toBe('seg2')
    expect(apiFetch.mock.calls[0][0]).toBe('/v0/ws/w1/agent/chats/c1/switch')
    expect(apiFetch.mock.calls[0][1]).toMatchObject({
      method: 'POST',
      body: JSON.stringify({ provider: 'claude' }),
    })
  })

  it('renameChat POSTs the title; deleteChat DELETEs; listProviders GETs', async () => {
    apiFetch.mockResolvedValue(undefined)
    await api.renameChat('w1', 'c1', 'New')
    expect(apiFetch.mock.calls[0][0]).toBe('/v0/ws/w1/agent/chats/c1/rename')
    expect(apiFetch.mock.calls[0][1]).toMatchObject({
      method: 'POST',
      body: JSON.stringify({ title: 'New' }),
    })
    await api.deleteChat('w1', 'c1')
    expect(apiFetch.mock.calls[1]).toEqual(['/v0/ws/w1/agent/chats/c1', { method: 'DELETE' }])
    apiFetch.mockResolvedValue([{ id: 'claude', displayName: 'Claude', icon: '<svg/>' }])
    const p = await api.listProviders('w1')
    expect(apiFetch.mock.calls[2][0]).toBe('/v0/ws/w1/agent/providers')
    expect(p[0]).toMatchObject({ id: 'claude', displayName: 'Claude' })
  })

  it('listProviders returns [] when the backend responds with no body', async () => {
    apiFetch.mockResolvedValue(undefined)
    const p = await api.listProviders('w1')
    expect(p).toEqual([])
  })

  it('propagates apiFetch errors to the caller', async () => {
    apiFetch.mockRejectedValueOnce(new Error('boom'))
    await expect(api.listChats('w1')).rejects.toThrow('boom')
  })
})
