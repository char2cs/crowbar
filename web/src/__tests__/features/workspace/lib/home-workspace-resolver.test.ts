import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { idle, success } from '@/lib/loadable'
import { useProjectDataStore } from '@/lib/store/projects'
import type { Project } from '@/lib/types'
import { IDBFactory } from 'fake-indexeddb'
import { loadCache, saveCache } from '@/lib/persistence/cache-store'
import { resetDB } from '@/lib/persistence/idb'
import { __resetWorkspaceScopesForTest, getWorkspaceScope } from '@/lib/workspace-scope'

const fetchHomeWorkspaceMock = vi.fn()

vi.mock('@/lib/api', () => ({
  fetchHomeWorkspace: (projectId: string) => fetchHomeWorkspaceMock(projectId),
}))

import {
  ensureHomeWorkspaceResolved,
  getKnownHomeWorkspaceIds,
  getHomeWorkspaceId,
  hydrateHomeWorkspace,
  useHomeWorkspaceState,
  __resetHomeWorkspaceResolverForTest,
} from '@/features/workspace/lib/home-workspace-resolver'

beforeEach(() => {
  fetchHomeWorkspaceMock.mockReset()
  __resetHomeWorkspaceResolverForTest()
  __resetWorkspaceScopesForTest()
  useHostConfigStore.setState({ hosts: [], remoteEverPaired: false })
  useProjectDataStore.setState({ data: idle(), readHosts: new Set() })
})

afterEach(() => {
  __resetHomeWorkspaceResolverForTest()
  useHostConfigStore.setState({ hosts: [], remoteEverPaired: false })
  useProjectDataStore.setState({ data: idle(), readHosts: new Set() })
})

describe('home-workspace-resolver', () => {
  it('useHomeWorkspaceState starts unresolved (no wsId, no error) before anything is fetched', () => {
    const { result } = renderHook(() => useHomeWorkspaceState('p1'))
    expect(result.current).toEqual({
      wsId: null,
      owningChatId: null,
      localPath: null,
      error: false,
      stale: false,
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
        stale: false,
      })
    })
    expect(getKnownHomeWorkspaceIds()).toContain('ws-home-1')
  })

  // Whoever hears the id builds URLs with it at once, a chat-scoped one included.
  it('records the home workspace scope, owning chat included, before anyone hears it resolved', async () => {
    fetchHomeWorkspaceMock.mockResolvedValueOnce({
      id: 'ws-home-1',
      projectId: 'p1',
      kind: 'home',
      owningChatId: 'chat-home-1',
    })
    const { result } = renderHook(() => {
      const { wsId } = useHomeWorkspaceState('p1')
      return wsId ? getWorkspaceScope(wsId) : null
    })

    act(() => ensureHomeWorkspaceResolved('p1'))

    await waitFor(() =>
      expect(result.current).toEqual({
        projectId: 'p1',
        repoId: '',
        wsId: 'ws-home-1',
        owningChatId: 'chat-home-1',
      }),
    )
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
        stale: false,
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
        stale: false,
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
  const hostRecord = {
    id: 'host-2',
    name: 'host-2',
    endpoint: 'host-2.example:3737',
    pairedFingerprint: 'SHA256:aaaa',
    clientId: 'client-host-2',
  }
  beforeEach(() => {
    useHostConfigStore.getState().addHost(hostRecord)
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
      stale: false,
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

  // The rule boot placement gives up on: a held row on a host that is not configured, or a
  // project in no row once every paired host has said what it has.
  it('says Project Home is unavailable, asking nothing, once its host is not coming, and heals when it is back', async () => {
    useProjectDataStore.setState({ data: success([row('p1', 'host-gone')]) })
    const { result } = renderHook(() => useHomeWorkspaceState('p1'))

    act(() => ensureHomeWorkspaceResolved('p1'))
    expect(result.current.error).toBe(true)
    expect(fetchHomeWorkspaceMock).not.toHaveBeenCalled()

    act(() => useHostConfigStore.getState().addHost({ ...hostRecord, id: 'host-gone' }))
    await waitFor(() => expect(result.current).toMatchObject({ wsId: 'ws-p1', error: false }))
  })

  it('gives up on a project in no row only once every paired host was read', () => {
    useProjectDataStore.setState({ data: success([row('other')]) })
    const { result } = renderHook(() => useHomeWorkspaceState('p1'))

    act(() => ensureHomeWorkspaceResolved('p1'))
    expect(result.current.error).toBe(false)
    act(() => useProjectDataStore.setState({ readHosts: new Set(['host-2']) }))
    expect(result.current.error).toBe(true)
    expect(fetchHomeWorkspaceMock).not.toHaveBeenCalled()
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

// An unreachable host never answers GET /home, so the last answer, kept in
// IndexedDB, is all a restored home pane has to resolve its scope from.
describe('home-workspace-resolver with a cached home workspace', () => {
  const cached = {
    wsId: 'ws-cached',
    owningChatId: 'chat-cached',
    localPath: '/cached/path',
  }

  beforeEach(async () => {
    resetDB()
    globalThis.indexedDB = new IDBFactory()
    await saveCache('workspaces-data', 'home-workspace:p1', cached)
  })

  it('resolves from the cache, scope included, before any request', async () => {
    await hydrateHomeWorkspace('p1')

    expect(getHomeWorkspaceId('p1')).toBe('ws-cached')
    expect(getKnownHomeWorkspaceIds()).toContain('ws-cached')
    expect(getWorkspaceScope('ws-cached')).toEqual({
      projectId: 'p1',
      repoId: '',
      wsId: 'ws-cached',
      owningChatId: 'chat-cached',
    })
    expect(fetchHomeWorkspaceMock).not.toHaveBeenCalled()
  })

  it('is not an error when the GET fails; the cached workspace stays', async () => {
    await hydrateHomeWorkspace('p1')
    fetchHomeWorkspaceMock.mockRejectedValueOnce(new Error('host down'))
    const { result } = renderHook(() => useHomeWorkspaceState('p1'))

    act(() => ensureHomeWorkspaceResolved('p1'))

    await waitFor(() => expect(fetchHomeWorkspaceMock).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(result.current.wsId).toBe('ws-cached'))
    expect(result.current.error).toBe(false)
  })

  it('asks the network even though the cache resolved, and the answer replaces it', async () => {
    await hydrateHomeWorkspace('p1')
    fetchHomeWorkspaceMock.mockResolvedValueOnce({
      id: 'ws-fresh',
      projectId: 'p1',
      owningChatId: 'chat-fresh',
      localPath: '/fresh',
    })
    const { result } = renderHook(() => useHomeWorkspaceState('p1'))

    act(() => ensureHomeWorkspaceResolved('p1'))

    await waitFor(() => expect(result.current.wsId).toBe('ws-fresh'))
    expect(result.current.stale).toBe(false)
    await waitFor(async () =>
      expect((await loadCache('workspaces-data', 'home-workspace:p1'))?.data).toEqual({
        wsId: 'ws-fresh',
        owningChatId: 'chat-fresh',
        localPath: '/fresh',
      }),
    )
  })

  it('does not overwrite an answer the network already gave', async () => {
    fetchHomeWorkspaceMock.mockResolvedValueOnce({ id: 'ws-fresh', projectId: 'p1' })
    ensureHomeWorkspaceResolved('p1')
    await waitFor(() => expect(getHomeWorkspaceId('p1')).toBe('ws-fresh'))

    await hydrateHomeWorkspace('p1')

    expect(getHomeWorkspaceId('p1')).toBe('ws-fresh')
  })
})
