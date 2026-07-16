import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'

const fetchHomeWorkspaceMock = vi.fn()

vi.mock('@/lib/api', () => ({
  fetchHomeWorkspace: (projectId: string) => fetchHomeWorkspaceMock(projectId),
}))

import {
  ensureHomeWorkspaceResolved,
  getKnownHomeWorkspaceIds,
  useHomeWorkspaceState,
  __resetHomeWorkspaceResolverForTest,
} from '@/features/workspace/lib/home-workspace-resolver'

beforeEach(() => {
  fetchHomeWorkspaceMock.mockReset()
  __resetHomeWorkspaceResolverForTest()
})

describe('home-workspace-resolver', () => {
  it('useHomeWorkspaceState starts unresolved (no wsId, no error) before anything is fetched', () => {
    const { result } = renderHook(() => useHomeWorkspaceState('p1'))
    expect(result.current).toEqual({ wsId: null, error: false })
  })

  it('resolves the home workspace id once the fetch settles, and known ids include it', async () => {
    fetchHomeWorkspaceMock.mockResolvedValueOnce({ id: 'ws-home-1', projectId: 'p1', kind: 'home' })
    const { result } = renderHook(() => useHomeWorkspaceState('p1'))

    act(() => {
      ensureHomeWorkspaceResolved('p1')
    })

    await waitFor(() => {
      expect(result.current).toEqual({ wsId: 'ws-home-1', error: false })
    })
    expect(getKnownHomeWorkspaceIds()).toContain('ws-home-1')
  })

  it('is idempotent: a second call for the same project id does not re-fetch', async () => {
    fetchHomeWorkspaceMock.mockResolvedValueOnce({ id: 'ws-home-1', projectId: 'p1', kind: 'home' })

    ensureHomeWorkspaceResolved('p1')
    ensureHomeWorkspaceResolved('p1') // in-flight — must not issue a second fetch
    await waitFor(() => expect(fetchHomeWorkspaceMock).toHaveBeenCalledTimes(1))

    ensureHomeWorkspaceResolved('p1') // already resolved — must not re-fetch
    expect(fetchHomeWorkspaceMock).toHaveBeenCalledTimes(1)
  })

  it('surfaces an error without caching a wsId, and does not auto-retry on a later call', async () => {
    fetchHomeWorkspaceMock.mockRejectedValueOnce(new Error('not found'))
    const { result } = renderHook(() => useHomeWorkspaceState('p1'))

    act(() => {
      ensureHomeWorkspaceResolved('p1')
    })

    await waitFor(() => {
      expect(result.current).toEqual({ wsId: null, error: true })
    })
    expect(getKnownHomeWorkspaceIds()).not.toContain('p1')

    // A resolved (even errored) project id is cached — see the `states.has`
    // guard — so a later call is a no-op rather than retrying on its own.
    // That matches the pre-existing behavior this replaces (HomeRoute's old
    // per-mount fetch also never retried a failed lookup on its own); this
    // resolver's job is caching the common (success) path, not a retry policy.
    fetchHomeWorkspaceMock.mockResolvedValueOnce({ id: 'ws-home-2', projectId: 'p1', kind: 'home' })
    ensureHomeWorkspaceResolved('p1')
    expect(fetchHomeWorkspaceMock).toHaveBeenCalledTimes(1)
  })

  it('tracks multiple projects independently', async () => {
    fetchHomeWorkspaceMock.mockImplementation((projectId: string) =>
      Promise.resolve({ id: `ws-${projectId}`, projectId, kind: 'home' }),
    )
    ensureHomeWorkspaceResolved('p1')
    ensureHomeWorkspaceResolved('p2')

    await waitFor(() => {
      expect(getKnownHomeWorkspaceIds().sort()).toEqual(['ws-p1', 'ws-p2'])
    })
  })
})
