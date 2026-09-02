import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

import { terminalCreate, terminalClose, __getBridgeInternals } from '@/lib/crowbar-bridge'
import { chatBase, terminalsBaseForWorkspace, workspaceBase } from '@/lib/workspace-scope-url'
import {
  recordWorkspaceScope,
  setWorkspaceScope,
  getOwningChatId,
  __resetWorkspaceScopesForTest,
} from '@/lib/workspace-scope'

// The PTY routes are chat-scoped: `/v0/chats/:chatId/terminals[...]`. A terminal
// belongs to the CHAT that opened it, never to the worktree it runs in — sibling
// chats share worktrees and must not share shells. These tests pin the URL shape
// and the wsId→chatId bridge that gets a workspace-holding caller there.

class FakeWebSocket {
  static instances: FakeWebSocket[] = []
  onopen: (() => void) | null = null
  onmessage: ((e: { data: string }) => void) | null = null
  onclose: (() => void) | null = null
  onerror: (() => void) | null = null
  constructor(public url: string) {
    FakeWebSocket.instances.push(this)
    queueMicrotask(() => this.onopen?.())
  }
  send = vi.fn()
  close = vi.fn()
}

beforeEach(() => {
  FakeWebSocket.instances = []
  vi.stubGlobal('WebSocket', FakeWebSocket as unknown as typeof WebSocket)
  __resetWorkspaceScopesForTest()
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('chatBase', () => {
  it('builds the flat chat prefix with no project/repo nesting', () => {
    expect(chatBase('chat-1')).toBe('/v0/chats/chat-1')
  })

  it('encodes a chat id so a stray slash cannot forge extra path segments', () => {
    expect(chatBase('a/b')).toBe('/v0/chats/a%2Fb')
  })
})

describe('terminalsBaseForWorkspace', () => {
  it("resolves a workspace to its owning chat's terminals route", () => {
    recordWorkspaceScope({
      projectId: 'p1',
      repoId: 'r1',
      wsId: 'ws-1',
      owningChatId: 'chat-1',
    })

    expect(terminalsBaseForWorkspace('ws-1')).toBe('/v0/chats/chat-1/terminals')
  })

  it('never emits a workspace-scoped terminals path', () => {
    recordWorkspaceScope({
      projectId: 'p1',
      repoId: 'r1',
      wsId: 'ws-1',
      owningChatId: 'chat-1',
    })

    // The old shape is gone from the daemon; building it would 404 silently.
    expect(terminalsBaseForWorkspace('ws-1')).not.toContain('/workspaces/')
    expect(terminalsBaseForWorkspace('ws-1')).not.toContain('ws-1')
  })

  it('throws rather than falling back when no owning chat is recorded', () => {
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-2' })

    expect(() => terminalsBaseForWorkspace('ws-2')).toThrow(/no owning chat/)
  })

  it('leaves the hierarchical workspaceBase alone for the routes that still nest', () => {
    // files/git/lsp have not moved yet; this proves the terminal cutover did not
    // drag them along.
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-1', owningChatId: 'c1' })

    expect(workspaceBase('ws-1')).toBe('/v0/projects/p1/repos/r1/workspaces/ws-1')
  })
})

describe('owning-chat recording', () => {
  // The IDE route (/ide/:projectId/:repoId/:wsId) carries no chat id, so the
  // route-derived write must MERGE rather than overwrite. Without this, merely
  // navigating to a workspace would erase the chat the sidebar had recorded and
  // every terminal URL for it would start throwing.
  it('a later route-derived scope write does not erase the sidebar-recorded chat', () => {
    recordWorkspaceScope({
      projectId: 'p1',
      repoId: 'r1',
      wsId: 'ws-1',
      owningChatId: 'chat-1',
    })

    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-1' })

    expect(getOwningChatId('ws-1')).toBe('chat-1')
    expect(terminalsBaseForWorkspace('ws-1')).toBe('/v0/chats/chat-1/terminals')
  })

  it('a fresh owning chat from the daemon replaces the old one', () => {
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-1', owningChatId: 'chat-1' })
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-1', owningChatId: 'chat-2' })

    expect(getOwningChatId('ws-1')).toBe('chat-2')
  })

  it('reports null for a workspace the daemon resolved no chat for', () => {
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-3', owningChatId: '' })

    expect(getOwningChatId('ws-3')).toBeNull()
  })
})

describe('terminalCreate', () => {
  it('POSTs to the chat-scoped base it was handed and records it for later verbs', async () => {
    const fetchSpy = vi.fn().mockResolvedValue({
      ok: true,
      status: 201,
      json: async () => ({ success: true, error: null, data: { sessionId: 'sess-1' } }),
    })
    vi.stubGlobal('fetch', fetchSpy)

    const base = '/v0/chats/chat-1/terminals'
    const sessionId = await terminalCreate(base)

    expect(sessionId).toBe('sess-1')

    const requestedUrl = String(fetchSpy.mock.calls[0][0])
    expect(requestedUrl).toContain('/v0/chats/chat-1/terminals')
    expect(requestedUrl).not.toContain('/workspaces/')
    expect(fetchSpy.mock.calls[0][1]).toMatchObject({ method: 'POST' })

    // The base is remembered so DELETE / the PTY WS cannot drift onto a
    // different scope than the one that created the session.
    expect(__getBridgeInternals().sessionBases.get('sess-1')).toBe(base)
    expect(FakeWebSocket.instances[0].url).toContain('/v0/chats/chat-1/terminals/sess-1/ws')
  })

  it('DELETEs under the same chat base the session was created on', async () => {
    const fetchSpy = vi.fn().mockResolvedValue({
      ok: true,
      status: 201,
      json: async () => ({ success: true, error: null, data: { sessionId: 'sess-2' } }),
    })
    vi.stubGlobal('fetch', fetchSpy)

    await terminalCreate('/v0/chats/chat-7/terminals')
    fetchSpy.mockClear()
    fetchSpy.mockResolvedValue({
      ok: true,
      status: 202,
      json: async () => ({ success: true, error: null, data: { id: 'sess-2' } }),
    })

    await terminalClose('sess-2')

    const deletedUrl = String(fetchSpy.mock.calls[0][0])
    expect(deletedUrl).toContain('/v0/chats/chat-7/terminals/sess-2')
    expect(fetchSpy.mock.calls[0][1]).toMatchObject({ method: 'DELETE' })
  })
})
