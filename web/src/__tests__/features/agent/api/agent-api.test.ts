import { beforeEach, describe, expect, it, vi } from 'vitest'

const apiFetch = vi.fn()
vi.mock('@/lib/api', () => ({ apiFetch: (...a: unknown[]) => apiFetch(...a) }))
vi.mock('@/lib/workspace-scope-url', () => ({ workspaceBase: (id: string) => `/v0/ws/${id}` }))

import * as api from '@/features/agent/api/agent-api'

describe('agent-api', () => {
  beforeEach(() => apiFetch.mockReset())

  it('listChats GETs the workspace-scoped chats list, carrying the live runner and its PTY', async () => {
    apiFetch.mockResolvedValue([
      {
        id: 'c1',
        workspaceId: 'w1',
        title: 'T',
        liveRunnerId: 'r1',
        terminalSessionId: 'pty1',
        activeProviderId: 'claude',
        createdAt: '2026-01-01T00:00:00Z',
      },
    ])
    const chats = await api.listChats('w1')
    expect(apiFetch).toHaveBeenCalledWith('/v0/ws/w1/agent/chats')
    // liveRunnerId + its PTY must survive the mapper — they ARE the attach contract.
    expect(chats[0]).toMatchObject({
      id: 'c1',
      liveRunnerId: 'r1',
      terminalSessionId: 'pty1',
      activeProviderId: 'claude',
    })
  })

  it('listChats carries a DORMANT chat through as empty runner/PTY (not undefined)', async () => {
    // '' is a meaningful value here — "no runner is on this chat" — and it is the
    // whole liveness answer. It must not arrive as undefined and read as falsy-by-luck.
    apiFetch.mockResolvedValue([
      {
        id: 'c1',
        workspaceId: 'w1',
        title: 'T',
        liveRunnerId: '',
        terminalSessionId: '',
        activeProviderId: 'claude',
        createdAt: '2026-01-01T00:00:00Z',
      },
    ])
    const chats = await api.listChats('w1')
    expect(chats[0].liveRunnerId).toBe('')
    expect(chats[0].terminalSessionId).toBe('')
    // A dormant chat still names the provider Resume brings back.
    expect(chats[0].activeProviderId).toBe('claude')
  })

  it('listChats returns [] when the backend responds with no body', async () => {
    apiFetch.mockResolvedValue(undefined)
    const chats = await api.listChats('w1')
    expect(chats).toEqual([])
  })

  it('getChat GETs the single chat and includes the conversations it has hosted', async () => {
    apiFetch.mockResolvedValue({
      id: 'c1',
      workspaceId: 'w1',
      title: 'T',
      liveRunnerId: 'r1',
      terminalSessionId: 'pty1',
      activeProviderId: 'claude',
      createdAt: '2026-01-01T00:00:00Z',
      conversations: [
        {
          chatId: 'c1',
          providerId: 'claude',
          sessionId: 'sess-1',
          firstSeenAt: '2026-01-01T00:00:00Z',
        },
      ],
    })
    const chat = await api.getChat('w1', 'c1')
    expect(apiFetch).toHaveBeenCalledWith('/v0/ws/w1/agent/chats/c1')
    expect(chat.liveRunnerId).toBe('r1')
    expect(chat.terminalSessionId).toBe('pty1')
    // Conversations succeed the deleted `segments`: pure append-only history, with
    // nothing in them that describes a process (no status, no PTY, no runner id).
    expect(chat.conversations).toHaveLength(1)
    expect(chat.conversations[0]).toMatchObject({ providerId: 'claude', sessionId: 'sess-1' })
  })

  it('getChat defaults conversations to [] when the backend omits them', async () => {
    apiFetch.mockResolvedValue({
      id: 'c1',
      workspaceId: 'w1',
      title: 'T',
      liveRunnerId: '',
      terminalSessionId: '',
      activeProviderId: 'claude',
      createdAt: '2026-01-01T00:00:00Z',
    })
    const chat = await api.getChat('w1', 'c1')
    expect(chat.conversations).toEqual([])
  })

  it('createChat POSTs the provider and returns the new id', async () => {
    apiFetch.mockResolvedValue({ id: 'c9' })
    const id = await api.createChat('w1', 'codex')
    expect(id).toBe('c9')
    const [url, init] = apiFetch.mock.calls[0]
    expect(url).toBe('/v0/ws/w1/agent/chats')
    expect(init).toMatchObject({ method: 'POST', body: JSON.stringify({ provider: 'codex' }) })
  })

  it('switchProvider POSTs to /switch and returns the NEW RUNNER id', async () => {
    apiFetch.mockResolvedValue({ id: 'r2' })
    const runnerId = await api.switchProvider('w1', 'c1', 'claude')
    expect(runnerId).toBe('r2')
    expect(apiFetch.mock.calls[0][0]).toBe('/v0/ws/w1/agent/chats/c1/switch')
    expect(apiFetch.mock.calls[0][1]).toMatchObject({
      method: 'POST',
      body: JSON.stringify({ provider: 'claude' }),
    })
  })

  it('resumeChat POSTs to /resume and returns the id of the RUNNER now on the chat', async () => {
    apiFetch.mockResolvedValue({ id: 'r9' })
    const runnerId = await api.resumeChat('w1', 'c1')
    expect(runnerId).toBe('r9')
    expect(apiFetch.mock.calls[0]).toEqual(['/v0/ws/w1/agent/chats/c1/resume', { method: 'POST' }])
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
