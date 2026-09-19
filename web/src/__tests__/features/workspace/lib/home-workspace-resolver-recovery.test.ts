import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'

// REGRESSION (restyle v2): a failed `GET /v0/projects/:p/home` is cached as a
// TERMINAL state (`states.set(projectId, {wsId: null, error: true})`) and the
// `states.has(projectId)` guard then refuses every later
// `ensureHomeWorkspaceResolved` for the rest of the session. That was
// tolerable when only HomeRoute read it; now the whole sidebar does:
// `space-scroller.tsx`'s `homeSeeded` (no home chat/folder rows at all),
// its header Thread button ("Can't start a new thread yet"),
// `sidebar-tree-surface.tsx`'s `homeRows` (no context menu for any home row)
// and `home-tree.ts`'s `resolveHomeRowScope` (every home row unresolvable)
// all ride on `getHomeWorkspaceId(projectId)`. One transport failure past
// apiFetch's ~4.5s retry budget — the daemon still replaying its ledger at a
// cold start, or being respawned by the desktop shell — blanks a project's
// home for the session, with no reseed path (the WS reconnect sentinel
// reseeds every entity stream but never this).
const fetchHomeWorkspaceMock = vi.fn()

vi.mock('@/lib/api', () => ({
  fetchHomeWorkspace: (projectId: string) => fetchHomeWorkspaceMock(projectId),
}))

import {
  ensureHomeWorkspaceResolved,
  getHomeWorkspaceId,
  useHomeWorkspaceState,
  __resetHomeWorkspaceResolverForTest,
} from '@/features/workspace/lib/home-workspace-resolver'

beforeEach(() => {
  fetchHomeWorkspaceMock.mockReset()
  __resetHomeWorkspaceResolverForTest()
})

describe('home-workspace-resolver recovers from a failed resolve', () => {
  it('re-fetches on the next ensure after a failure, and resolves once the daemon answers', async () => {
    fetchHomeWorkspaceMock.mockRejectedValueOnce(new Error('connection refused'))
    const { result } = renderHook(() => useHomeWorkspaceState('p1'))

    act(() => {
      ensureHomeWorkspaceResolved('p1')
    })
    await waitFor(() => expect(result.current.error).toBe(true))
    expect(getHomeWorkspaceId('p1')).toBeNull()

    // The daemon is back. The very next mount/render that asks again (every
    // SpacePanel does, on every mount) must get a real answer, not the
    // cached failure.
    fetchHomeWorkspaceMock.mockResolvedValueOnce({
      id: 'ws-home-1',
      projectId: 'p1',
      kind: 'home',
      owningChatId: 'chat-home-1',
    })
    act(() => {
      ensureHomeWorkspaceResolved('p1')
    })
    expect(fetchHomeWorkspaceMock).toHaveBeenCalledTimes(2)
    await waitFor(() => expect(getHomeWorkspaceId('p1')).toBe('ws-home-1'))
    expect(result.current).toEqual({
      wsId: 'ws-home-1',
      owningChatId: 'chat-home-1',
      localPath: null,
      error: false,
    })
  })
})
