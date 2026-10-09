import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { idle, success } from '@/lib/loadable'
import { useProjectDataStore } from '@/lib/store/projects'
import type { Project } from '@/lib/types'

const fetchHomeWorkspaceMock = vi.fn()

vi.mock('@/lib/api', () => ({
  fetchHomeWorkspace: (projectId: string) => fetchHomeWorkspaceMock(projectId),
}))

import {
  ensureHomeWorkspaceResolved,
  getKnownHomeWorkspaceIds,
  getHomeWorkspaceId,
  useHomeWorkspaceState,
  __resetHomeWorkspaceResolverForTest,
} from '@/features/workspace/lib/home-workspace-resolver'

beforeEach(() => {
  fetchHomeWorkspaceMock.mockReset()
  __resetHomeWorkspaceResolverForTest()
  useHostConfigStore.setState({ hosts: [], remoteEverPaired: false })
  useProjectDataStore.setState({ data: idle() })
})

afterEach(() => {
  __resetHomeWorkspaceResolverForTest()
  useHostConfigStore.setState({ hosts: [], remoteEverPaired: false })
  useProjectDataStore.setState({ data: idle() })
})

describe('home-workspace-resolver', () => {
  it('useHomeWorkspaceState starts unresolved (no wsId, no error) before anything is fetched', () => {
    const { result } = renderHook(() => useHomeWorkspaceState('p1'))
    expect(result.current).toEqual({
      wsId: null,
      owningChatId: null,
      localPath: null,
      error: false,
    })
  })

  it('resolves the home workspace id AND its on-disk path once the fetch settles, and known ids include it', async () => {
    fetchHomeWorkspaceMock.mockResolvedValueOnce({
      id: 'ws-home-1',
      projectId: 'p1',
      kind: 'home',
      owningChatId: 'chat-home-1',
      localPath: '/Users/mateo/projects/rabbyte-labs',
    })
    const { result } = renderHook(() => useHomeWorkspaceState('p1'))

    act(() => {
      ensureHomeWorkspaceResolved('p1')
    })

    await waitFor(() => {
      // owningChatId is carried through because chat-scoped routes (a terminal's)
      // are addressed by it and the home route supplies no chat id of its own.
      // localPath is the real on-disk project root — the editor pane/file
      // explorer resolve it from here, never from a borrowed repo path.
      expect(result.current).toEqual({
        wsId: 'ws-home-1',
        owningChatId: 'chat-home-1',
        localPath: '/Users/mateo/projects/rabbyte-labs',
        error: false,
      })
    })
    expect(getKnownHomeWorkspaceIds()).toContain('ws-home-1')
  })

  it('is idempotent: a second call for the same project id does not re-fetch', async () => {
    fetchHomeWorkspaceMock.mockResolvedValueOnce({
      id: 'ws-home-1',
      projectId: 'p1',
      kind: 'home',
      owningChatId: 'chat-home-1',
    })

    ensureHomeWorkspaceResolved('p1')
    ensureHomeWorkspaceResolved('p1') // in-flight — must not issue a second fetch
    await waitFor(() => expect(fetchHomeWorkspaceMock).toHaveBeenCalledTimes(1))

    ensureHomeWorkspaceResolved('p1') // already resolved — must not re-fetch
    expect(fetchHomeWorkspaceMock).toHaveBeenCalledTimes(1)
  })

  it('surfaces an error without caching a wsId, and re-fetches on the next call', async () => {
    fetchHomeWorkspaceMock.mockRejectedValueOnce(new Error('not found'))
    const { result } = renderHook(() => useHomeWorkspaceState('p1'))

    act(() => {
      ensureHomeWorkspaceResolved('p1')
    })

    await waitFor(() => {
      expect(result.current).toEqual({
        wsId: null,
        owningChatId: null,
        localPath: null,
        error: true,
      })
    })
    expect(getKnownHomeWorkspaceIds()).not.toContain('p1')

    // A failure is not terminal: the whole sidebar rides on this id, so one
    // lost GET at cold start must heal on the next mount rather than blank the
    // project's home for the session.
    fetchHomeWorkspaceMock.mockResolvedValueOnce({ id: 'ws-home-2', projectId: 'p1', kind: 'home' })
    act(() => {
      ensureHomeWorkspaceResolved('p1')
    })
    expect(fetchHomeWorkspaceMock).toHaveBeenCalledTimes(2)
    await waitFor(() => {
      expect(result.current).toEqual({
        wsId: 'ws-home-2',
        owningChatId: null,
        localPath: null,
        error: false,
      })
    })
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

  describe('getHomeWorkspaceId', () => {
    it('returns null before anything has resolved for that project', () => {
      expect(getHomeWorkspaceId('p1')).toBeNull()
    })

    it("returns exactly the ONE project's own resolved id, not another project's", async () => {
      fetchHomeWorkspaceMock.mockImplementation((projectId: string) =>
        Promise.resolve({ id: `ws-${projectId}`, projectId, kind: 'home' }),
      )
      ensureHomeWorkspaceResolved('p1')
      ensureHomeWorkspaceResolved('p2')

      await waitFor(() => {
        expect(getHomeWorkspaceId('p1')).toBe('ws-p1')
      })
      expect(getHomeWorkspaceId('p2')).toBe('ws-p2')
      expect(getHomeWorkspaceId('p1')).not.toBe(getHomeWorkspaceId('p2'))
    })

    it('returns null for a project that errored rather than the error sentinel', async () => {
      fetchHomeWorkspaceMock.mockRejectedValueOnce(new Error('not found'))
      ensureHomeWorkspaceResolved('p1')

      await waitFor(() => {
        expect(getKnownHomeWorkspaceIds()).not.toContain('p1')
      })
      expect(getHomeWorkspaceId('p1')).toBeNull()
    })
  })
})

// On a paired install a cold boot asks before the project list has named the project's host:
// that is pending, not "Project Home unavailable", and it heals when the list lands.
describe('home-workspace-resolver on an install that paired a remote', () => {
  const row = (id: string, hostId = 'local'): Project => ({
    id,
    name: id,
    path: `/work/${id}`,
    lastActivity: new Date(0),
    hostId,
  })
  beforeEach(() => {
    useHostConfigStore.getState().addHost({
      id: 'host-2',
      name: 'host-2',
      endpoint: 'host-2.example:3737',
      pairedFingerprint: 'SHA256:aaaa',
      clientId: 'client-host-2',
    })
    fetchHomeWorkspaceMock.mockImplementation((projectId: string) =>
      Promise.resolve({ id: `ws-${projectId}`, projectId, kind: 'home', owningChatId: 'c-home' }),
    )
  })

  it('shows no error and requests nothing until the project list places the project', async () => {
    const { result } = renderHook(() => useHomeWorkspaceState('p1'))

    act(() => ensureHomeWorkspaceResolved('p1'))
    expect(fetchHomeWorkspaceMock).not.toHaveBeenCalled()
    expect(result.current).toEqual({
      wsId: null,
      owningChatId: null,
      localPath: null,
      error: false,
    })

    act(() => useProjectDataStore.setState({ data: success([row('p1')]) }))
    await waitFor(() => expect(result.current.wsId).toBe('ws-p1'))
    expect(fetchHomeWorkspaceMock).toHaveBeenCalledExactlyOnceWith('p1')
    expect(result.current.error).toBe(false)
  })

  it('stays pending, not failed, while a loaded list lacks the project, and heals when it arrives', async () => {
    useProjectDataStore.setState({ data: success([row('other')]) })
    const { result } = renderHook(() => useHomeWorkspaceState('p1'))

    act(() => ensureHomeWorkspaceResolved('p1'))
    expect(result.current.error).toBe(false)
    expect(fetchHomeWorkspaceMock).not.toHaveBeenCalled()

    act(() => useProjectDataStore.setState({ data: success([row('other'), row('p1', 'host-2')]) }))
    await waitFor(() => expect(result.current.wsId).toBe('ws-p1'))
  })

  it('repeated calls while pending do not stack waits: one request when the host resolves', async () => {
    ensureHomeWorkspaceResolved('p1')
    ensureHomeWorkspaceResolved('p1')
    ensureHomeWorkspaceResolved('p1')

    act(() => useProjectDataStore.setState({ data: success([row('p1')]) }))
    await waitFor(() => expect(getHomeWorkspaceId('p1')).toBe('ws-p1'))
    expect(fetchHomeWorkspaceMock).toHaveBeenCalledTimes(1)
  })
})
