import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { reparentWorkspace } from '@/lib/api/workspace'

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
