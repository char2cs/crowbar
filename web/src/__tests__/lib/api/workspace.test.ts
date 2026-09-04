import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import {
  reparentWorkspace,
  rebaseOntoParent,
  retryProvision,
  detachHolder,
} from '@/lib/api/workspace'
import { recordWorkspaceScope, __resetWorkspaceScopesForTest } from '@/lib/workspace-scope'

// The worktree lifecycle verbs are addressed to the CHAT that holds the
// worktree (backend spec §4.3), on the repo-scoped chat prefix the chat's own
// verbs already use. These run against the REAL scope registry rather than a
// mocked URL builder: the whole point of the move is that the owning chat id
// ends up in the path, and a mocked builder would assert nothing about that.
const WS = 'ws3'
const CHAT = 'chat-ws3'
const VERB_BASE = `/v0/projects/p1/repos/crowbar/chats/${CHAT}`

let fetchMock: ReturnType<typeof vi.fn>

beforeEach(() => {
  __resetWorkspaceScopesForTest()
  recordWorkspaceScope({
    projectId: 'p1',
    repoId: 'crowbar',
    wsId: WS,
    owningChatId: CHAT,
  })
  fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 202 }))
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

function calledWith(): [string, RequestInit] {
  expect(fetchMock).toHaveBeenCalledTimes(1)
  return fetchMock.mock.calls[0] as [string, RequestInit]
}

// §3: reparent is a hierarchical 202 mutation. The new parentId arrives on the
// WorkspaceDTO over the scoped WS stream, so this function no longer mutates
// local state — it just dials the reparent verb on the holding chat.
describe('reparentWorkspace', () => {
  it("POSTs to the reparent verb on the workspace's owning chat", async () => {
    await reparentWorkspace(WS, 'ws-develop')
    const [url, init] = calledWith()
    expect(url).toBe(`${VERB_BASE}/reparent`)
    expect(init.method).toBe('POST')
    expect(JSON.parse(init.body as string)).toEqual({ newParentId: 'ws-develop' })
  })

  // newParentId names the FORK parent, which is workspace lineage — a different
  // field from the chat tree's own ParentID. Only the addressed row moved onto a
  // chat id; sending a chat id as the new parent would rebase onto nothing.
  it('still sends a WORKSPACE id as the new parent, not a chat id', async () => {
    await reparentWorkspace(WS, 'ws-develop')
    const [, init] = calledWith()
    expect(JSON.parse(init.body as string).newParentId).toBe('ws-develop')
  })

  it('propagates a backend failure to the caller', async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ success: false, error: 'has children' }), {
        status: 409,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    await expect(reparentWorkspace(WS, 'ws-other')).rejects.toThrow('has children')
  })
})

// §3: user-initiated "finish the move" after a reparent left the branch behind.
describe('rebaseOntoParent', () => {
  it('POSTs to the rebase-onto-parent verb on the owning chat', async () => {
    await rebaseOntoParent(WS)
    const [url, init] = calledWith()
    expect(url).toBe(`${VERB_BASE}/rebase-onto-parent`)
    expect(init.method).toBe('POST')
  })
})

// §3.3: retryProvision re-provisions a held placeholder branch in place. It's a
// 202 mutation whose outcome rides the WS broadcast, so the FE call just fires a
// POST to the verb route.
describe('retryProvision', () => {
  it('POSTs to the retry-provision verb on the owning chat', async () => {
    await retryProvision(WS)
    const [url, init] = calledWith()
    expect(url).toBe(`${VERB_BASE}/retry-provision`)
    expect(init.method).toBe('POST')
  })
})

// §3.5/§3.7: detachHolder evicts the branch's holder (with the user's consent)
// then re-provisions in place. Same 202-over-WS shape.
describe('detachHolder', () => {
  it('POSTs to the detach-holder verb on the owning chat', async () => {
    await detachHolder(WS)
    const [url, init] = calledWith()
    expect(url).toBe(`${VERB_BASE}/detach-holder`)
    expect(init.method).toBe('POST')
  })
})

// A placeholder row the sidebar recorded a scope for but no owning chat is a
// scope-recording bug. Throwing names it at the call site; guessing a URL would
// surface it as a 404 far from the cause, and — worse — a `/chats//lock` built
// from an empty id would 404 identically whatever went wrong.
describe('a workspace with no recorded owning chat', () => {
  it('throws rather than dialling a chat-less URL', async () => {
    __resetWorkspaceScopesForTest()
    recordWorkspaceScope({ projectId: 'p1', repoId: 'crowbar', wsId: 'orphan' })
    await expect(retryProvision('orphan')).rejects.toThrow(/no owning chat/)
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
