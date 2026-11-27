import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

import { terminalCreate, terminalKill } from '@/lib/crowbar-bridge'
import {
  chatBase,
  filesBaseForWorkspace,
  gitBaseForWorkspace,
  terminalsBaseForWorkspace,
  workspaceBase,
  worktreeVerbBaseForWorkspace,
  repoChatsBaseForWorkspace,
} from '@/lib/workspace-scope-url'
import { recordWorkspaceScope, setWorkspaceScope, getOwningChatId } from '@/lib/workspace-scope'
import { useWorkspaceScopeStore } from '@/lib/workspace-scope-store'

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
  useWorkspaceScopeStore.setState(useWorkspaceScopeStore.getInitialState())
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

    expect(terminalsBaseForWorkspace('ws-1')).not.toContain('/workspaces/')
    expect(terminalsBaseForWorkspace('ws-1')).not.toContain('ws-1')
  })

  it('throws rather than falling back when no owning chat is recorded', () => {
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-2' })

    expect(() => terminalsBaseForWorkspace('ws-2')).toThrow(/no owning chat/)
  })

  it('keeps the project home on its own /home/terminals mount, owner or not', () => {
    recordWorkspaceScope({ projectId: 'p1', repoId: '', wsId: 'home-ws' })

    expect(terminalsBaseForWorkspace('home-ws')).toBe('/v0/projects/p1/home/terminals')
  })

  it('leaves the hierarchical workspaceBase alone for the routes that still nest', () => {
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-1', owningChatId: 'c1' })

    expect(workspaceBase('ws-1')).toBe('/v0/projects/p1/repos/r1/workspaces/ws-1')
  })
})

describe('filesBaseForWorkspace', () => {
  it('builds the flat chat-scoped files base for the chat holding the worktree', () => {
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-1', owningChatId: 'chat-1' })

    expect(filesBaseForWorkspace('ws-1')).toBe('/v0/chats/chat-1/files')
  })

  it('names no workspace anywhere in the URL', () => {
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-1', owningChatId: 'chat-1' })

    expect(filesBaseForWorkspace('ws-1')).not.toContain('/workspaces/')
    expect(filesBaseForWorkspace('ws-1')).not.toContain('ws-1')
  })

  it('gives each chat sharing a worktree its own base', () => {
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-a', owningChatId: 'chat-a' })
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-b', owningChatId: 'chat-b' })

    expect(filesBaseForWorkspace('ws-a')).toBe('/v0/chats/chat-a/files')
    expect(filesBaseForWorkspace('ws-b')).toBe('/v0/chats/chat-b/files')
  })

  it('keeps the project home on its own project-level base', () => {
    recordWorkspaceScope({ projectId: 'p1', repoId: '', wsId: 'home-ws' })

    expect(filesBaseForWorkspace('home-ws')).toBe('/v0/projects/p1/home/files')
  })

  it('throws rather than falling back to the retiring workspace-scoped route', () => {
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-2' })

    expect(() => filesBaseForWorkspace('ws-2')).toThrow(/no owning chat/)
  })
})

describe('gitBaseForWorkspace', () => {
  it('builds the flat chat-scoped git base for the chat holding the worktree', () => {
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-1', owningChatId: 'chat-1' })

    expect(gitBaseForWorkspace('ws-1')).toBe('/v0/chats/chat-1/git')
  })

  it('names no workspace anywhere in the URL', () => {
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-1', owningChatId: 'chat-1' })

    expect(gitBaseForWorkspace('ws-1')).not.toContain('/workspaces/')
    expect(gitBaseForWorkspace('ws-1')).not.toContain('ws-1')
  })

  it('gives each chat sharing a worktree its own base', () => {
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-a', owningChatId: 'chat-a' })
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-b', owningChatId: 'chat-b' })

    expect(gitBaseForWorkspace('ws-a')).toBe('/v0/chats/chat-a/git')
    expect(gitBaseForWorkspace('ws-b')).toBe('/v0/chats/chat-b/git')
  })

  it('throws rather than falling back to the retiring workspace-scoped route', () => {
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-2' })

    expect(() => gitBaseForWorkspace('ws-2')).toThrow(/no owning chat/)
  })
})

describe('owning-chat recording', () => {
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

    expect(FakeWebSocket.instances).toHaveLength(0)
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

    await terminalKill('sess-2')

    const deletedUrl = String(fetchSpy.mock.calls[0][0])
    expect(deletedUrl).toContain('/v0/chats/chat-7/terminals/sess-2')
    expect(fetchSpy.mock.calls[0][1]).toMatchObject({ method: 'DELETE' })
  })
})

describe('worktreeVerbBaseForWorkspace', () => {
  it('addresses the verb to the chat HOLDING the worktree, under its repo', () => {
    recordWorkspaceScope({
      projectId: 'p1',
      repoId: 'r1',
      wsId: 'ws-1',
      owningChatId: 'chat-1',
    })
    expect(worktreeVerbBaseForWorkspace('ws-1')).toBe('/v0/projects/p1/repos/r1/chats/chat-1')
  })

  it('keeps the repo nesting rather than collapsing onto the flat chat prefix', () => {
    recordWorkspaceScope({
      projectId: 'p1',
      repoId: 'r1',
      wsId: 'ws-1',
      owningChatId: 'chat-1',
    })
    const base = worktreeVerbBaseForWorkspace('ws-1')
    expect(base).not.toBe(chatBase('chat-1'))
    expect(base).toContain('/repos/r1/')
  })

  it('throws when the workspace has no recorded owning chat', () => {
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-orphan' })
    expect(() => worktreeVerbBaseForWorkspace('ws-orphan')).toThrow(/no owning chat/)
  })

  it('encodes the chat id so a stray slash cannot forge extra path segments', () => {
    recordWorkspaceScope({
      projectId: 'p1',
      repoId: 'r1',
      wsId: 'ws-2',
      owningChatId: 'a/b',
    })
    expect(worktreeVerbBaseForWorkspace('ws-2')).toBe('/v0/projects/p1/repos/r1/chats/a%2Fb')
  })
})

describe('repoChatsBaseForWorkspace', () => {
  it('nests under the workspace’s own project and repo', () => {
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-1', owningChatId: 'c1' })
    expect(repoChatsBaseForWorkspace('ws-1')).toBe('/v0/projects/p1/repos/r1/chats')
  })

  it('falls back to the project-home mount for a repo-less workspace', () => {
    setWorkspaceScope({ projectId: 'p1', repoId: '', wsId: 'ws-home' })
    expect(repoChatsBaseForWorkspace('ws-home')).toBe('/v0/projects/p1/home/chats')
  })
})
