import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/lib/errors/api-error'

const apiFetch = vi.fn()
const apiFetchRaw = vi.fn()
vi.mock('@/lib/api/fetch', () => ({
  apiFetch: (...a: unknown[]) => apiFetch(...a),
  apiFetchRaw: (...a: unknown[]) => apiFetchRaw(...a),
}))
// Only the scope REGISTRY is faked; the URL builders themselves run for real,
// so these assertions pin the actual paths this client dials rather than a
// stand-in shape. 'w1' carries a real repo, so chatBase builds the Task-17
// repo-scoped shape; 'ws-home' has repoId '' (a project-home workspace) and
// falls back to the still-live /home mount; anything else is unrecorded scope.
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

describe('agent-chat-reads: chatBase URL shape', () => {
  beforeEach(() => {
    apiFetch.mockReset()
    apiFetchRaw.mockReset()
  })

  it('builds the repo-scoped chat URL for a non-home workspace', async () => {
    apiFetch.mockResolvedValue([])
    await api.listChats('w1')
    expect(apiFetch).toHaveBeenCalledWith('/v0/projects/p1/repos/r1/chats')
    expect(apiFetch.mock.calls[0][0]).not.toContain('/workspaces/')
  })

  it('keeps the home mount unchanged for a home workspace', async () => {
    apiFetch.mockResolvedValue([])
    await api.listChats('ws-home')
    expect(apiFetch).toHaveBeenCalledWith('/v0/projects/p1/home/chats')
    expect(apiFetch.mock.calls[0][0]).not.toContain('/repos/')
  })
})

describe('agent-chat-reads', () => {
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
    expect(apiFetch).toHaveBeenCalledWith('/v0/projects/p1/repos/r1/chats')
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

  // The chat's sticky selection. Omitted on the wire means UNSET — "whatever this
  // provider defaults to" — and it is grounded to '' so an absent key and a
  // cleared one are the same thing everywhere downstream.
  it('listChats carries the sticky model/effort selection, grounding an absent one', async () => {
    apiFetch.mockResolvedValue([
      {
        id: 'c1',
        workspaceId: 'w1',
        title: 'T',
        liveRunnerId: '',
        terminalSessionId: '',
        activeProviderId: 'codex',
        createdAt: '2026-01-01T00:00:00Z',
        model: 'gpt-5.6-luna',
        effort: 'max',
      },
      {
        id: 'c2',
        workspaceId: 'w1',
        title: 'T2',
        liveRunnerId: '',
        terminalSessionId: '',
        activeProviderId: 'codex',
        createdAt: '2026-01-01T00:00:00Z',
      },
    ])
    const chats = await api.listChats('w1')
    expect(chats[0]).toMatchObject({ model: 'gpt-5.6-luna', effort: 'max' })
    expect(chats[1].model).toBe('')
    expect(chats[1].effort).toBe('')
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
    expect(apiFetch).toHaveBeenCalledWith('/v0/projects/p1/repos/r1/chats/c1', {
      signal: undefined,
    })
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

  // A blob read goes through the shared raw fetch: it retries a transport
  // failure and reports a daemon refusal like every other read.
  it('reads a payload a row names by its blob ref, and null once it is swept', async () => {
    apiFetchRaw
      .mockResolvedValueOnce(new Response('full output'))
      .mockRejectedValueOnce(new ApiError('gone', 404))
    const controller = new AbortController()
    await expect(api.readBlob('chat/1', 'sha256:ab', controller.signal)).resolves.toBe(
      'full output',
    )
    expect(apiFetchRaw).toHaveBeenCalledWith('/v0/chats/chat%2F1/blobs/sha256%3Aab', {
      signal: controller.signal,
    })
    await expect(api.readBlob('c1', 'sha256:gone')).resolves.toBeNull()
  })

  it('throws on a blob read that failed for any other reason', async () => {
    apiFetchRaw.mockRejectedValueOnce(new ApiError('boom', 500))
    await expect(api.readBlob('c1', 'sha256:ab')).rejects.toThrow('boom')
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

  // getPendingPrompt: recovers a prompt submission the backend has not yet
  // confirmed the provider accepted. 204 (no body) means nothing to recover,
  // not an error — the same "no body" convention as listChats/listProviders
  // above, and the same cancellable single-attempt read as getSlashCatalog.
  describe('getPendingPrompt', () => {
    it('returns null when the backend has nothing pending (204)', async () => {
      apiFetch.mockResolvedValueOnce(undefined)
      const result = await api.getPendingPrompt('w1', 'c1')
      expect(result).toBeNull()
    })

    it('returns the recovered text, state and request id when something is pending', async () => {
      apiFetch.mockResolvedValueOnce({
        text: 'please rename this function',
        state: 'dispatching',
        requestId: '5c1b1c8a-2f3e-4a9b-9d1e-6a2b3c4d5e6f',
      })
      const result = await api.getPendingPrompt('w1', 'c1')
      expect(result).toEqual({
        text: 'please rename this function',
        state: 'dispatching',
        requestId: '5c1b1c8a-2f3e-4a9b-9d1e-6a2b3c4d5e6f',
      })
    })

    it('GETs the cancellable pending-prompt route with no read retry/cache layer', async () => {
      const controller = new AbortController()
      apiFetch.mockResolvedValueOnce(undefined)
      await api.getPendingPrompt('w1', 'c1', controller.signal)
      expect(apiFetch).toHaveBeenCalledWith(
        '/v0/projects/p1/repos/r1/chats/c1/pending-prompt',
        { signal: controller.signal },
        { attempts: 1, baseDelayMs: 0, maxDelayMs: 0 },
      )
    })
  })

  it('propagates apiFetch errors to the caller', async () => {
    apiFetch.mockRejectedValueOnce(new Error('boom'))
    await expect(api.listChats('w1')).rejects.toThrow('boom')
  })

  // ── AgentChat.parentId / .order: the tree grounding on mapChat ──────────
  // Every chat read is grounded here, once, so nothing downstream has to
  // remember that an absent parent and a root parent are the same thing.

  it('listChats grounds a chat with no parentId/order to root ("") / 0', async () => {
    apiFetch.mockResolvedValueOnce([
      {
        id: 'c1',
        workspaceId: 'w1',
        title: 'T',
        liveRunnerId: '',
        terminalSessionId: '',
        activeProviderId: 'claude',
        createdAt: '2026-01-01T00:00:00Z',
        // parentId/order omitted, as a pre-tree daemon would send
      },
    ])
    const [chat] = await api.listChats('w1')
    expect(chat.parentId).toBe('')
    expect(chat.order).toBe(0)
  })

  it('listChats carries an explicit parentId/order through untouched', async () => {
    apiFetch.mockResolvedValueOnce([
      {
        id: 'c1',
        workspaceId: 'w1',
        title: 'T',
        liveRunnerId: '',
        terminalSessionId: '',
        activeProviderId: 'claude',
        createdAt: '2026-01-01T00:00:00Z',
        parentId: 'f1',
        order: 4,
      },
    ])
    const [chat] = await api.listChats('w1')
    expect(chat.parentId).toBe('f1')
    expect(chat.order).toBe(4)
  })
})
