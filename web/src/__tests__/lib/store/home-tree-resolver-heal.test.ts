import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'

// REGRESSION (restyle v2 follow-up): the home-workspace resolver can now be
// re-asked after a failed GET /home, but the only place that re-asks it on
// a live signal is `subscribeHomeWorkspace`'s reconnect branch — and that
// tracker is opened for the ACTIVE project alone (app-sync-engine.ts
// `homeKey`). Every VISIBLE project also holds a `subscribeHomeTree`
// subscription on the same `/home/chats/ws` socket; it reseeds the tree on
// the reconnect sentinel but never re-asks the resolver, so a non-active
// project whose first GET /home lost the cold-start race keeps
// `getHomeWorkspaceId() === null` (no home rows, no header Thread button)
// until the user swipes to it and the home route's own effect asks again.
const subscribers = new Set<(data: unknown) => void>()

vi.mock('@/lib/ws/manager', () => ({
  wsManager: {
    subscribe: (_endpoint: string, cb: (data: unknown) => void) => {
      subscribers.add(cb)
      return () => subscribers.delete(cb)
    },
    send: vi.fn(),
  },
}))

const fetchHomeWorkspace = vi.fn()
vi.mock('@/lib/api', () => ({
  fetchHomeChats: vi.fn().mockResolvedValue([]),
  fetchHomeFolders: vi.fn().mockResolvedValue([]),
  fetchRepos: vi.fn().mockResolvedValue([]),
  fetchHomeWorkspace: (projectId: string) => fetchHomeWorkspace(projectId) as unknown,
  assetURL: (path: string) => path,
}))

const { subscribeHomeTree } = await import('@/lib/store/home-tree')
const {
  ensureHomeWorkspaceResolved,
  getHomeWorkspaceId,
  useHomeWorkspaceState,
  __resetHomeWorkspaceResolverForTest,
} = await import('@/features/workspace/lib/home-workspace-resolver')

beforeEach(() => {
  subscribers.clear()
  fetchHomeWorkspace.mockReset()
  __resetHomeWorkspaceResolverForTest()
})

describe('a visible, non-active project whose GET /home failed at cold start', () => {
  it('re-asks the resolver when its home tree subscription sees the reconnect sentinel', async () => {
    fetchHomeWorkspace.mockRejectedValueOnce(new Error('daemon_unavailable'))
    const { result } = renderHook(() => useHomeWorkspaceState('p2'))
    act(() => {
      ensureHomeWorkspaceResolved('p2')
    })
    await waitFor(() => expect(result.current.error).toBe(true))
    expect(getHomeWorkspaceId('p2')).toBeNull()

    // The engine holds this for every VISIBLE project (homeTreeKey), active or not.
    const dispose = subscribeHomeTree('p2')

    // The daemon is back: every channel on the socket gets the sentinel.
    fetchHomeWorkspace.mockResolvedValueOnce({
      id: 'ws-home-2',
      projectId: 'p2',
      owningChatId: 'c',
    })
    act(() => {
      subscribers.forEach((cb) => cb({ reconnected: true }))
    })

    expect(fetchHomeWorkspace).toHaveBeenCalledTimes(2)
    await waitFor(() => expect(getHomeWorkspaceId('p2')).toBe('ws-home-2'))
    dispose()
  })
})
