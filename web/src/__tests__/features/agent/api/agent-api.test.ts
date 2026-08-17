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

  it('reads the newest hook-ledger page and grounds cursor fields', async () => {
    apiFetch.mockResolvedValue({
      cursor: 12,
      oldestCursor: 11,
      hasMore: true,
      items: [
        {
          sequence: 11,
          role: 'user',
          providerId: 'codex',
          text: 'Explain this',
          at: '2026-08-16T00:00:00Z',
        },
        {
          sequence: 12,
          role: 'assistant',
          providerId: 'codex',
          text: 'Sure.',
          at: '2026-08-16T00:00:01Z',
        },
      ],
    })
    const page = await api.listChatMessages('w1', 'chat/1')
    expect(apiFetch).toHaveBeenCalledWith('/v0/ws/w1/agent/chats/chat%2F1/messages?limit=100', {
      signal: undefined,
    })
    expect(page).toMatchObject({ cursor: 12, oldestCursor: 11, hasMore: true })
    expect(page.items.map((item) => item.sequence)).toEqual([11, 12])
  })

  it('carries incremental and older paging cursors plus a cancellation signal', async () => {
    const controller = new AbortController()
    apiFetch.mockResolvedValue({ cursor: 9, oldestCursor: 4, hasMore: false, items: [] })
    await api.listChatMessages('w1', 'c1', {
      after: 4,
      limit: 25,
      signal: controller.signal,
    })
    expect(apiFetch).toHaveBeenCalledWith('/v0/ws/w1/agent/chats/c1/messages?after=4&limit=25', {
      signal: controller.signal,
    })
    await api.listChatMessages('w1', 'c1', { before: 10, limit: 25 })
    expect(apiFetch).toHaveBeenLastCalledWith(
      '/v0/ws/w1/agent/chats/c1/messages?before=10&limit=25',
      { signal: undefined },
    )
  })

  it('submits one completed prompt with a stable client request identity', async () => {
    apiFetch.mockResolvedValue({ runnerId: 'r2', terminalSessionId: 'pty2' })
    const result = await api.submitAgentPrompt('w1', 'c1', 'Line one\nline two', 'request-1')
    expect(result).toEqual({ runnerId: 'r2', terminalSessionId: 'pty2' })
    expect(apiFetch).toHaveBeenCalledWith(
      '/v0/ws/w1/agent/chats/c1/prompts',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ text: 'Line one\nline two', clientRequestId: 'request-1' }),
      }),
    )
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
      '/v0/ws/w1/agent/chats/c1/slash-catalog',
      { signal: controller.signal },
      { attempts: 1, baseDelayMs: 0, maxDelayMs: 0 },
    )
  })

  it('createChat POSTs the provider and returns the new id', async () => {
    apiFetch.mockResolvedValue({ id: 'c9' })
    const id = await api.createChat('w1', 'codex')
    expect(id).toBe('c9')
    const [url, init] = apiFetch.mock.calls[0]
    expect(url).toBe('/v0/ws/w1/agent/chats')
    // No parent given is the workspace root, said explicitly rather than by
    // omission — one shape on the wire whether or not the chat is a thread.
    expect(init).toMatchObject({
      method: 'POST',
      body: JSON.stringify({ provider: 'codex', parentId: '' }),
    })
  })

  it('createChat carries the PARENT, so the edge exists before the runner starts', async () => {
    // Placement rides on the create. It used to be a second call, which meant
    // the thread's first CLI was spawned before its lineage existed — the very
    // turn the user asked the thread for ran with the agent as a stranger.
    apiFetch.mockResolvedValue({ id: 'c9' })
    await api.createChat('w1', 'codex', 'parent-chat')
    expect(apiFetch.mock.calls[0][1]).toMatchObject({
      body: JSON.stringify({ provider: 'codex', parentId: 'parent-chat' }),
    })
    // ONE call: no follow-up placement to leave an orphan at the root if it
    // failed.
    expect(apiFetch).toHaveBeenCalledTimes(1)
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

  // The enriched read carries the three provider facts the FE leans on:
  // `connected` (is the CLI installed), `enabled` (is the provider offered) and
  // `mcpEnabled` (may its agent use Crowbar's own tools).
  it('listProviders maps connected + enabled + mcpEnabled', async () => {
    apiFetch.mockResolvedValueOnce([
      {
        id: 'codex',
        displayName: 'Codex',
        icon: '<svg/>',
        connected: true,
        enabled: true,
        mcpEnabled: false,
      },
    ])
    const out = await api.listProviders('w1')
    expect(out[0]).toMatchObject({ id: 'codex', connected: true, enabled: true, mcpEnabled: false })
  })

  // A provider row that omits the flags defaults to enabled (spec §3.1: a
  // provider with no stored preference is enabled), NOT connected (install is
  // never assumed), and with its tool surface ON — so the FE never treats an
  // unknown provider as disabled, and never silently strips Crowbar's tools from
  // a daemon whose payload predates the field. The backend stores the NEGATIVE
  // (mcpDisabled), so absent means on.
  it('listProviders defaults missing enabled + mcpEnabled to true and connected to false', async () => {
    apiFetch.mockResolvedValueOnce([{ id: 'claude', displayName: 'Claude', icon: '<svg/>' }])
    const out = await api.listProviders('w1')
    expect(out[0]).toMatchObject({
      id: 'claude',
      connected: false,
      enabled: true,
      mcpEnabled: true,
    })
  })

  // The global preferences write: PUTs the FULL ordered set (index = priority) to
  // the settings route and hands back the freshly resolved list, so the caller
  // reconciles from server truth without a second GET.
  it('updateProviderPreferences PUTs the ordered list and returns the resolved providers', async () => {
    apiFetch.mockResolvedValueOnce([
      { id: 'codex', displayName: 'Codex', icon: '', connected: true, enabled: true },
      { id: 'claude', displayName: 'Claude', icon: '', connected: true, enabled: false },
    ])
    const out = await api.updateProviderPreferences([
      { id: 'codex', disabled: false, mcpDisabled: true },
      { id: 'claude', disabled: true, mcpDisabled: false },
    ])
    expect(apiFetch).toHaveBeenCalledWith(
      '/v0/settings/agent/providers',
      expect.objectContaining({
        method: 'PUT',
        body: JSON.stringify({
          providers: [
            { id: 'codex', disabled: false, mcpDisabled: true },
            { id: 'claude', disabled: true, mcpDisabled: false },
          ],
        }),
      }),
    )
    expect(out.map((p) => p.id)).toEqual(['codex', 'claude'])
    expect(out[1].enabled).toBe(false)
  })

  it('updateProviderPreferences returns [] when the backend responds with no body', async () => {
    apiFetch.mockResolvedValueOnce(undefined)
    const out = await api.updateProviderPreferences([
      { id: 'codex', disabled: false, mcpDisabled: false },
    ])
    expect(out).toEqual([])
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

  // ── The chats tree: folders + placement ──────────────────────────────────
  // A second aggregate with its own route (agent-api.ts's "Writes" section
  // comment), so it gets its own describe rather than folding into the chat
  // tests above.

  describe('chat folders + placement', () => {
    it('listChatFolders GETs the workspace-scoped folders list and grounds parentId/order', async () => {
      apiFetch.mockResolvedValueOnce([
        { id: 'f1', workspaceId: 'w1', name: 'Folder 1', parentId: 'f0', order: 2 },
        { id: 'f2', workspaceId: 'w1', name: 'Folder 2' }, // parentId/order omitted
      ])
      const folders = await api.listChatFolders('w1')
      expect(apiFetch).toHaveBeenCalledWith('/v0/ws/w1/agent/folders')
      expect(folders[0]).toMatchObject({ id: 'f1', parentId: 'f0', order: 2 })
      // A root/never-placed folder grounds the same way mapChat does.
      expect(folders[1]).toMatchObject({ id: 'f2', parentId: '', order: 0 })
    })

    it('listChatFolders returns [] when the backend responds with no body', async () => {
      apiFetch.mockResolvedValueOnce(undefined)
      expect(await api.listChatFolders('w1')).toEqual([])
    })

    it('createChatFolder POSTs {name, parentId} and unwraps {folder, shifted}', async () => {
      apiFetch.mockResolvedValueOnce({
        folder: { id: 'f9', workspaceId: 'w1', name: 'New folder', parentId: '', order: 0 },
        shifted: [{ id: 'f1', workspaceId: 'w1', name: 'F1', parentId: '', order: 1 }],
      })
      const { folder, shifted } = await api.createChatFolder('w1', 'New folder', '')
      expect(apiFetch.mock.calls[0][0]).toBe('/v0/ws/w1/agent/folders')
      expect(apiFetch.mock.calls[0][1]).toMatchObject({
        method: 'POST',
        body: JSON.stringify({ name: 'New folder', parentId: '' }),
      })
      expect(folder).toMatchObject({ id: 'f9', order: 0 })
      // The dense renumber's collateral — every sibling it displaced.
      expect(shifted).toEqual([{ id: 'f1', workspaceId: 'w1', name: 'F1', parentId: '', order: 1 }])
    })

    it('createChatFolder defaults shifted to [] when the backend omits the field', async () => {
      apiFetch.mockResolvedValueOnce({
        folder: { id: 'f9', workspaceId: 'w1', name: 'X', parentId: '', order: 0 },
      })
      const { shifted } = await api.createChatFolder('w1', 'X', '')
      expect(shifted).toEqual([])
    })

    it('createChatFolder defaults shifted to [] when the backend sends shifted: null', async () => {
      apiFetch.mockResolvedValueOnce({
        folder: { id: 'f9', workspaceId: 'w1', name: 'X', parentId: '', order: 0 },
        shifted: null,
      })
      const { shifted } = await api.createChatFolder('w1', 'X', '')
      expect(shifted).toEqual([])
    })

    it('updateChatFolder PATCHes only the named fields (a partial patch) and unwraps the same envelope', async () => {
      apiFetch.mockResolvedValueOnce({
        folder: { id: 'f1', workspaceId: 'w1', name: 'Renamed', parentId: '', order: 0 },
        shifted: [],
      })
      const { folder } = await api.updateChatFolder('w1', 'f1', { name: 'Renamed' })
      expect(apiFetch.mock.calls[0][0]).toBe('/v0/ws/w1/agent/folders/f1')
      expect(apiFetch.mock.calls[0][1]).toMatchObject({
        method: 'PATCH',
        body: JSON.stringify({ name: 'Renamed' }),
      })
      expect(folder.name).toBe('Renamed')
    })

    it('updateChatFolder PATCHes a re-parent + reorder patch verbatim', async () => {
      apiFetch.mockResolvedValueOnce({
        folder: { id: 'f1', workspaceId: 'w1', name: 'F1', parentId: 'f2', order: 3 },
        shifted: [],
      })
      await api.updateChatFolder('w1', 'f1', { parentId: 'f2', order: 3 })
      expect(apiFetch.mock.calls[0][1]).toMatchObject({
        body: JSON.stringify({ parentId: 'f2', order: 3 }),
      })
    })

    it('deleteChatFolder DELETEs and defaults to [] when the backend responds with a null body', async () => {
      apiFetch.mockResolvedValueOnce(null)
      const shifted = await api.deleteChatFolder('w1', 'f1')
      expect(apiFetch.mock.calls[0]).toEqual(['/v0/ws/w1/agent/folders/f1', { method: 'DELETE' }])
      expect(shifted).toEqual([])
    })

    it('deleteChatFolder returns the promoted-children shift the delete triggered', async () => {
      apiFetch.mockResolvedValueOnce({
        shifted: [{ id: 'f2', workspaceId: 'w1', name: 'F2', parentId: '', order: 0 }],
      })
      const shifted = await api.deleteChatFolder('w1', 'f1')
      expect(shifted).toEqual([{ id: 'f2', workspaceId: 'w1', name: 'F2', parentId: '', order: 0 }])
    })

    it('setChatPlacement PATCHes {parentId, order} to the placement route and unwraps {chat, shifted}', async () => {
      apiFetch.mockResolvedValueOnce({
        chat: {
          id: 'c1',
          workspaceId: 'w1',
          title: 'T',
          liveRunnerId: '',
          terminalSessionId: '',
          activeProviderId: 'claude',
          createdAt: '2026-01-01T00:00:00Z',
          // parentId/order omitted on the wire chat — grounded by mapChat below.
        },
        shifted: [{ id: 'f1', workspaceId: 'w1', name: 'F1', parentId: '', order: 3 }],
      })
      const { chat, shifted } = await api.setChatPlacement('w1', 'c1', { parentId: 'f1', order: 2 })
      expect(apiFetch.mock.calls[0][0]).toBe('/v0/ws/w1/agent/chats/c1/placement')
      expect(apiFetch.mock.calls[0][1]).toMatchObject({
        method: 'PATCH',
        body: JSON.stringify({ parentId: 'f1', order: 2 }),
      })
      expect(chat).toMatchObject({ id: 'c1', parentId: '', order: 0 })
      expect(shifted[0]).toMatchObject({ id: 'f1', order: 3 })
    })

    it('setChatPlacement defaults shifted to [] when the backend omits it', async () => {
      apiFetch.mockResolvedValueOnce({
        chat: {
          id: 'c1',
          workspaceId: 'w1',
          title: 'T',
          liveRunnerId: '',
          terminalSessionId: '',
          activeProviderId: 'claude',
          createdAt: '2026-01-01T00:00:00Z',
        },
      })
      const { shifted } = await api.setChatPlacement('w1', 'c1', { parentId: '', order: 0 })
      expect(shifted).toEqual([])
    })
  })
})
