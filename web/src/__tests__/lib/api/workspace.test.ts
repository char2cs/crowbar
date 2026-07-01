import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { reparentWorkspace, retryProvision, detachHolder } from '@/lib/api/workspace'

// §3.3/§3.5: placeholder recovery routes resolve their project/repo from the
// recorded workspace scope, which isn't set up in a unit test — mock the base
// builder to a deterministic scoped URL so we can assert the exact route.
vi.mock('@/lib/workspace-scope-url', () => ({
  workspaceBase: (id: string) => `/v0/projects/p/repos/r/workspaces/${id}`,
}))

// §3: reparent is a hierarchical 202 mutation. The new parentId arrives on the
// WorkspaceDTO over the scoped WS stream, so this function no longer mutates
// local state — it just dials the hierarchical reparent route.
describe('reparentWorkspace', () => {
  let fetchMock: ReturnType<typeof vi.fn>

  beforeEach(() => {
    fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 202 }))
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('POSTs to the hierarchical reparent route with the new parent id', async () => {
    await reparentWorkspace('p1', 'crowbar', 'ws3', 'ws-develop')
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/v0/projects/p1/repos/crowbar/workspaces/ws3/reparent')
    expect(init.method).toBe('POST')
    expect(JSON.parse(init.body as string)).toEqual({ newParentId: 'ws-develop' })
  })

  it('propagates a backend failure to the caller', async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ success: false, error: 'has children' }), {
        status: 409,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    await expect(reparentWorkspace('p1', 'crowbar', 'ws3', 'ws-other')).rejects.toThrow(
      'has children',
    )
  })
})

// §3.3: retryProvision re-provisions a held placeholder branch in place. It's a
// 202 mutation whose outcome rides the WS broadcast, so the FE call just fires a
// POST to the scoped retry-provision route.
describe('retryProvision', () => {
  let fetchMock: ReturnType<typeof vi.fn>

  beforeEach(() => {
    fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 202 }))
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('POSTs to the retry-provision route', async () => {
    await retryProvision('w1')
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/v0/projects/p/repos/r/workspaces/w1/retry-provision')
    expect(init.method).toBe('POST')
  })
})

// §3.5/§3.7: detachHolder evicts the branch's holder (with the user's consent)
// then re-provisions in place. Same 202-over-WS shape — fire a POST to the
// scoped detach-holder route.
describe('detachHolder', () => {
  let fetchMock: ReturnType<typeof vi.fn>

  beforeEach(() => {
    fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 202 }))
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('POSTs to the detach-holder route', async () => {
    await detachHolder('w1')
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/v0/projects/p/repos/r/workspaces/w1/detach-holder')
    expect(init.method).toBe('POST')
  })
})
