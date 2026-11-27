import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { reparentWorkspace, retryProvision, detachHolder } from '@/lib/api/workspace'
import { recordWorkspaceScope } from '@/lib/workspace-scope'
import { useWorkspaceScopeStore } from '@/lib/workspace-scope-store'

const WS = 'ws3'
const CHAT = 'chat-ws3'
const VERB_BASE = `/v0/projects/p1/repos/crowbar/chats/${CHAT}`

let fetchMock: ReturnType<typeof vi.fn>

beforeEach(() => {
  useWorkspaceScopeStore.setState(useWorkspaceScopeStore.getInitialState())
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

describe('reparentWorkspace', () => {
  it("POSTs to the reparent verb on the workspace's owning chat", async () => {
    await reparentWorkspace(WS, 'ws-develop', 'op-1')
    const [url, init] = calledWith()
    expect(url).toBe(`${VERB_BASE}/reparent`)
    expect(init.method).toBe('POST')
    expect(new Headers(init.headers).get('Idempotency-Key')).toBe('op-1')
    expect(JSON.parse(init.body as string)).toEqual({ newParentId: 'ws-develop' })
  })

  it('still sends a WORKSPACE id as the new parent, not a chat id', async () => {
    await reparentWorkspace(WS, 'ws-develop', 'op-1')
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
    await expect(reparentWorkspace(WS, 'ws-other', 'op-1')).rejects.toThrow('has children')
  })
})

describe('retryProvision', () => {
  it('POSTs to the retry-provision verb on the owning chat', async () => {
    await retryProvision(WS, 'op-1')
    const [url, init] = calledWith()
    expect(url).toBe(`${VERB_BASE}/retry-provision`)
    expect(init.method).toBe('POST')
  })
})

describe('detachHolder', () => {
  it('POSTs to the detach-holder verb on the owning chat', async () => {
    await detachHolder(WS, 'op-1')
    const [url, init] = calledWith()
    expect(url).toBe(`${VERB_BASE}/detach-holder`)
    expect(init.method).toBe('POST')
  })
})

describe('a workspace with no recorded owning chat', () => {
  it('throws rather than dialling a chat-less URL', async () => {
    useWorkspaceScopeStore.setState(useWorkspaceScopeStore.getInitialState())
    recordWorkspaceScope({ projectId: 'p1', repoId: 'crowbar', wsId: 'orphan' })
    await expect(retryProvision('orphan', 'op-1')).rejects.toThrow(/no owning chat/)
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
