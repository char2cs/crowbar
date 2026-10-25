import type { ReactNode } from 'react'
import { describe, it, expect, beforeAll, beforeEach, afterEach, vi } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { useWorkspaceGit } from '@/features/workspace/stores/hooks/use-workspace-git'
import { useReviewFilesSummary } from '@/features/git/hooks/use-review-files-summary'
import { useSidebarChangedFiles } from '@/features/git/hooks/use-sidebar-changed-files'
import {
  markGitStatusChanged,
  requestGitRefresh,
  useGitRefreshStore,
} from '@/features/git/stores/git-refresh'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { __resetWorkspaceScopesForTest, setWorkspaceScope } from '@/lib/workspace-scope'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

const { fetchGitData, getReviewFiles, subscribe } = vi.hoisted(() => ({
  fetchGitData: vi.fn(),
  getReviewFiles: vi.fn(),
  subscribe: vi.fn(() => () => {}),
}))

vi.mock('@/features/git/api/git-data-api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/features/git/api/git-data-api')>()
  return { ...actual, fetchGitData }
})
vi.mock('@/features/git/api/review-api', () => ({ getReviewFiles }))
vi.mock('@/lib/ws/manager', () => ({ wsManager: { subscribe, send: vi.fn() } }))

const status = { branch: 'main', ahead: 0, behind: 0, files: [] }

function setup(opts: { owningChatId?: string | null; home?: boolean } = {}) {
  const store = createWorkspaceStore('ws-test')
  const reloadStatusAndLog = vi.fn(() => Promise.resolve())
  store.setState({ gitActions: { ...store.getState().gitActions, reloadStatusAndLog } })
  const wrapper = ({ children }: { children: ReactNode }) => (
    <WorkspaceStoreContext.Provider value={store}>{children}</WorkspaceStoreContext.Provider>
  )
  const hook = renderHook(
    ({ active, chat }) => useWorkspaceGit('ws-test', active, chat, opts.home ?? false),
    {
      wrapper,
      initialProps: {
        active: true,
        chat: 'owningChatId' in opts ? (opts.owningChatId ?? null) : ('chat-test' as string | null),
      },
    },
  )
  return { store, reloadStatusAndLog, ...hook }
}

const onGitFrame = () => {
  const calls = subscribe.mock.calls as unknown as [string, (frame: unknown) => void][]
  return calls.find(([ep]) => ep === '/v0/chats/chat-test/git/status')![1]
}

// The slice loads its API modules on first use; transform them once, outside the waitFor budget.
beforeAll(async () => {
  await Promise.all([
    import('@/features/git/api/git-data-api'),
    import('@/features/git/api/git-commits-api'),
    import('@/features/git/api/git-status-api'),
    import('fast-deep-equal'),
  ])
})

beforeEach(() => {
  vi.clearAllMocks()
  setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-test', owningChatId: 'chat-test' })
  fetchGitData.mockResolvedValue({ status, commits: [] })
})

describe('useWorkspaceGit', () => {
  it('loads once the workspace is shown, then subscribes to the git status topic', async () => {
    const { store } = setup()
    await waitFor(() => expect(store.getState().gitLoad).toBe('ready'))
    expect(fetchGitData).toHaveBeenCalledWith('ws-test')
    await waitFor(() =>
      expect(subscribe).toHaveBeenCalledWith(
        '/v0/chats/chat-test/git/status',
        expect.any(Function),
      ),
    )
  })

  it('does nothing while the workspace has never been shown', () => {
    const store = createWorkspaceStore('ws-test')
    renderHook(() => useWorkspaceGit('ws-test', false, 'chat-test', false), {
      wrapper: ({ children }: { children: ReactNode }) => (
        <WorkspaceStoreContext.Provider value={store}>{children}</WorkspaceStoreContext.Provider>
      ),
    })
    expect(fetchGitData).not.toHaveBeenCalled()
    expect(subscribe).not.toHaveBeenCalled()
  })

  it('has no git surface for the home workspace', () => {
    setup({ home: true, owningChatId: null })
    expect(fetchGitData).not.toHaveBeenCalled()
    expect(subscribe).not.toHaveBeenCalled()
  })

  it('waits for the owning chat id, then loads and subscribes', async () => {
    const { store, rerender } = setup({ owningChatId: null })
    expect(fetchGitData).not.toHaveBeenCalled()
    expect(subscribe).not.toHaveBeenCalled()

    rerender({ active: true, chat: 'chat-test' })
    await waitFor(() => expect(store.getState().gitLoad).toBe('ready'))
    await waitFor(() => expect(subscribe).toHaveBeenCalled())
  })

  describe('status stream', () => {
    beforeEach(() => vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] }))
    afterEach(() => vi.useRealTimers())

    async function ready() {
      const ctx = setup()
      await vi.waitFor(() => expect(ctx.store.getState().gitLoad).toBe('ready'))
      await vi.waitFor(() => expect(subscribe).toHaveBeenCalled())
      return ctx
    }

    // The stream's first push repeats the status the load just returned.
    it('does not reload for a frame equal to the loaded status, but does for a changed one', async () => {
      const { reloadStatusAndLog } = await ready()

      onGitFrame()({ ...status })
      await vi.advanceTimersByTimeAsync(500)
      expect(reloadStatusAndLog).not.toHaveBeenCalled()

      onGitFrame()({ ...status, ahead: 1 })
      await vi.advanceTimersByTimeAsync(500)
      expect(reloadStatusAndLog).toHaveBeenCalledTimes(1)
    })

    // Frames arrive faster than the debounce; a resetting timer would starve.
    it('reloads despite a continuous stream of frames, once per distinct change', async () => {
      const { reloadStatusAndLog } = await ready()
      const frame = onGitFrame()

      for (let i = 0; i < 10; i++) {
        frame({ ...status, ahead: 1 })
        await vi.advanceTimersByTimeAsync(150)
      }
      expect(reloadStatusAndLog).toHaveBeenCalledTimes(1)

      frame({ ...status, ahead: 2 })
      await vi.advanceTimersByTimeAsync(500)
      expect(reloadStatusAndLog).toHaveBeenCalledTimes(2)
    })

    it('tells open diff views to refetch after the push-driven reload', async () => {
      const { reloadStatusAndLog } = await ready()
      const before = useGitRefreshStore.getState().changed['ws-test'] ?? 0

      onGitFrame()({ ...status, ahead: 1 })
      await vi.advanceTimersByTimeAsync(500)

      expect(reloadStatusAndLog).toHaveBeenCalled()
      expect(useGitRefreshStore.getState().changed['ws-test']).toBe(before + 1)
    })

    it('reloads on an editor-save refresh request for this workspace only', async () => {
      const { reloadStatusAndLog } = await ready()

      requestGitRefresh('ws-other')
      await vi.advanceTimersByTimeAsync(500)
      expect(reloadStatusAndLog).not.toHaveBeenCalled()

      act(() => requestGitRefresh('ws-test'))
      await vi.advanceTimersByTimeAsync(500)
      expect(reloadStatusAndLog).toHaveBeenCalledTimes(1)
    })
  })
})

describe('on an install that has paired a remote host', () => {
  const REMOTE_STATUS = 'crowbar://host-1/v0/chats/chat-remote/git/status'
  const LOCAL_STATUS = '/v0/chats/chat-local/git/status'
  const endpoints = () =>
    (subscribe.mock.calls as unknown as [string][]).map(([endpoint]) => endpoint)

  function mount(wsId: 'ws-remote' | 'ws-local') {
    const store = createWorkspaceStore(wsId)
    const chat = wsId === 'ws-remote' ? 'chat-remote' : 'chat-local'
    const hook = renderHook(() => useWorkspaceGit(wsId, true, chat, false), {
      wrapper: ({ children }: { children: ReactNode }) => (
        <WorkspaceStoreContext.Provider value={store}>{children}</WorkspaceStoreContext.Provider>
      ),
    })
    return { store, ...hook }
  }

  beforeEach(() => {
    resetHostWorld()
    pairRemoteHost('host-1')
    __resetWorkspaceScopesForTest()
    setWorkspaceScope({
      projectId: 'p-remote',
      repoId: 'r1',
      wsId: 'ws-remote',
      owningChatId: 'chat-remote',
    })
    setWorkspaceScope({
      projectId: 'p-local',
      repoId: 'r1',
      wsId: 'ws-local',
      owningChatId: 'chat-local',
    })
  })

  afterEach(() => resetHostWorld())

  it('loads nothing and opens no channel until the project list says whose workspace it is', async () => {
    const remote = mount('ws-remote')
    const local = mount('ws-local')
    expect(fetchGitData).not.toHaveBeenCalled()
    expect(subscribe).not.toHaveBeenCalled()
    expect(remote.store.getState().gitLoad).not.toBe('ready')

    // Each workspace waits for its OWN project's row, not merely for a list.
    act(() => loadProjects(projectOn('p-remote', 'host-1')))
    await waitFor(() => expect(remote.store.getState().gitLoad).toBe('ready'))
    expect(fetchGitData.mock.calls).toEqual([['ws-remote']])
    expect(local.store.getState().gitLoad).toBe('idle')

    act(() => loadProjects(projectOn('p-remote', 'host-1'), projectOn('p-local', 'local')))
    await waitFor(() => expect(local.store.getState().gitLoad).toBe('ready'))
    await waitFor(() => expect(endpoints().sort()).toEqual([LOCAL_STATUS, REMOTE_STATUS].sort()))
  })

  it('switching local to remote closes the old channel and a late frame from it reloads nothing', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    try {
      loadProjects(projectOn('p-remote', 'host-1'), projectOn('p-local', 'local'))
      const unsubLocal = vi.fn()
      subscribe.mockReturnValueOnce(unsubLocal)
      const local = mount('ws-local')
      await vi.waitFor(() => expect(subscribe).toHaveBeenCalledTimes(1))
      const fromLocalChannel = (
        subscribe.mock.calls as unknown as [string, (frame: unknown) => void][]
      )[0][1]

      local.unmount()
      const remote = mount('ws-remote')
      const reloadRemote = vi.fn(() => Promise.resolve())
      remote.store.setState({
        gitActions: { ...remote.store.getState().gitActions, reloadStatusAndLog: reloadRemote },
      })
      await vi.waitFor(() => expect(subscribe).toHaveBeenCalledTimes(2))

      expect(unsubLocal).toHaveBeenCalledTimes(1)
      expect(endpoints()).toEqual([LOCAL_STATUS, REMOTE_STATUS])
      fromLocalChannel({ ...status, ahead: 9 })
      await vi.advanceTimersByTimeAsync(500)
      expect(reloadRemote).not.toHaveBeenCalled()
    } finally {
      vi.useRealTimers()
    }
  })

  it('revoking the host closes its channel and never reopens it against the local daemon', async () => {
    loadProjects(projectOn('p-remote', 'host-1'))
    const unsubscribe = vi.fn()
    subscribe.mockReturnValueOnce(unsubscribe)
    const { store } = mount('ws-remote')
    await waitFor(() => expect(store.getState().gitLoad).toBe('ready'))
    await waitFor(() => expect(subscribe).toHaveBeenCalledTimes(1))
    fetchGitData.mockClear()

    act(() => useHostConfigStore.getState().removeHost('host-1'))

    expect(unsubscribe).toHaveBeenCalledTimes(1)
    expect(subscribe).toHaveBeenCalledTimes(1)
    expect(fetchGitData).not.toHaveBeenCalled()
  })

  it('does not let a refused channel escape the effect', async () => {
    loadProjects(projectOn('p-remote', 'host-1'))
    subscribe.mockImplementationOnce(() => {
      throw new Error('remote hosts need the desktop app')
    })

    // The effect runs after an async load, so a throw surfaces as an uncaught error.
    const uncaught = vi.fn()
    window.addEventListener('error', uncaught)

    const { store } = mount('ws-remote')
    await waitFor(() => expect(store.getState().gitLoad).toBe('ready'))
    await waitFor(() => expect(subscribe).toHaveBeenCalled())

    window.removeEventListener('error', uncaught)
    expect(uncaught).not.toHaveBeenCalled()
  })
})

describe('files summary ownership', () => {
  afterEach(() => destroyWorkspaceStore('ws-test'))

  // The sidebar tree and the Files changed tab both read the workspace store's
  // reviewFiles; the store alone fetches it when the git tick fires.
  it('fetches the summary once per git tick for every consumer of the workspace', async () => {
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-test', owningChatId: 'chat-test' })
    fetchGitData.mockResolvedValue({ status, commits: [], reviewFiles: [] })
    getReviewFiles.mockResolvedValue([])
    const store = getOrCreateWorkspaceStore('ws-test')
    const wrapper = ({ children }: { children: ReactNode }) => (
      <WorkspaceStoreContext.Provider value={store}>{children}</WorkspaceStoreContext.Provider>
    )
    const { result } = renderHook(
      () => {
        useWorkspaceGit('ws-test', true, 'chat-test', false)
        return { sidebar: useSidebarChangedFiles('ws-test'), tab: useReviewFilesSummary('ws-test') }
      },
      { wrapper },
    )
    await waitFor(() => expect(result.current.tab.loaded).toBe(true))
    await waitFor(() => expect(subscribe).toHaveBeenCalled())
    getReviewFiles.mockClear()

    act(() => markGitStatusChanged('ws-test'))

    await waitFor(() => expect(getReviewFiles).toHaveBeenCalled())
    await act(async () => {})
    expect(getReviewFiles).toHaveBeenCalledTimes(1)
  })
})
