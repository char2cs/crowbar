import type { ReactNode } from 'react'
import { describe, it, expect, beforeAll, beforeEach, afterEach, onTestFinished, vi } from 'vitest'
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
import { toastManager } from '@/lib/toast-manager'
import { heldScopes, streamDaemon } from '@/__tests__/__fixtures__/stream-daemon'
import { holding } from '@/core/stream/stream'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'
import { ApiError } from '@/lib/errors/api-error'

const { fetchGitData, getReviewFiles } = vi.hoisted(() => ({
  fetchGitData: vi.fn(),
  getReviewFiles: vi.fn(),
}))

vi.mock('@/features/git/api/git-data-api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/features/git/api/git-data-api')>()
  return { ...actual, fetchGitData }
})
vi.mock('@/features/git/api/review-api', () => ({ getReviewFiles }))

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

const held = () => heldScopes('git:')
const onGitFrame = () => (data: unknown) =>
  streamDaemon.deliver({ t: 'signal', scope: 'git:ws-test', kind: 'git_status', data })

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
  it('holds git:<ws> while mounted and applies git_status signals', async () => {
    const { store, reloadStatusAndLog, unmount } = setup()
    await waitFor(() => expect(store.getState().gitLoad).toBe('ready'))
    expect(fetchGitData).toHaveBeenCalledWith('ws-test')
    await waitFor(() => expect(held()).toEqual(['git:ws-test']))

    onGitFrame()({ ...status, ahead: 3 })
    await waitFor(() => expect(reloadStatusAndLog).toHaveBeenCalledTimes(1))
    unmount()
    expect(held()).toEqual([])
  })

  it('does nothing while the workspace has never been shown', () => {
    const store = createWorkspaceStore('ws-test')
    renderHook(() => useWorkspaceGit('ws-test', false, 'chat-test', false), {
      wrapper: ({ children }: { children: ReactNode }) => (
        <WorkspaceStoreContext.Provider value={store}>{children}</WorkspaceStoreContext.Provider>
      ),
    })
    expect(fetchGitData).not.toHaveBeenCalled()
    expect(held()).toEqual([])
  })

  it('has no git surface for the home workspace', () => {
    setup({ home: true, owningChatId: null })
    expect(fetchGitData).not.toHaveBeenCalled()
    expect(held()).toEqual([])
  })

  it('waits for the owning chat id, then loads and subscribes', async () => {
    const { store, rerender } = setup({ owningChatId: null })
    expect(fetchGitData).not.toHaveBeenCalled()
    expect(held()).toEqual([])

    rerender({ active: true, chat: 'chat-test' })
    await waitFor(() => expect(store.getState().gitLoad).toBe('ready'))
    await waitFor(() => expect(held()).toEqual(['git:ws-test']))
  })

  describe('status stream', () => {
    beforeEach(() => vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] }))
    afterEach(() => vi.useRealTimers())

    async function ready() {
      const ctx = setup()
      await vi.waitFor(() => expect(ctx.store.getState().gitLoad).toBe('ready'))
      await vi.waitFor(() => expect(held()).toEqual(['git:ws-test']))
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

    // The daemon says git can no longer read the worktree (its repo moved), then that it can again,
    // with the very status it had: each is news, so the panel shows and then clears the error.
    it('reloads on an unreadable frame and again on the recovery that repeats the old status', async () => {
      const { reloadStatusAndLog } = await ready()

      onGitFrame()({ error: 'not a git repository' })
      await vi.advanceTimersByTimeAsync(500)
      expect(reloadStatusAndLog).toHaveBeenCalledTimes(1)

      onGitFrame()({ ...status })
      await vi.advanceTimersByTimeAsync(500)
      expect(reloadStatusAndLog).toHaveBeenCalledTimes(2)
    })

    it('a failed push-driven reload reports why', async () => {
      const { reloadStatusAndLog } = await ready()
      const toastAdd = vi.spyOn(toastManager, 'add')
      onTestFinished(() => toastAdd.mockRestore())
      reloadStatusAndLog.mockRejectedValueOnce(new ApiError('git status failed', 500))

      onGitFrame()({ ...status, ahead: 1 })
      await vi.advanceTimersByTimeAsync(500)

      expect(toastAdd).toHaveBeenCalledWith(
        expect.objectContaining({ title: 'Refresh git status', description: 'git status failed' }),
      )
    })

    it('tells open diff views to refetch after the push-driven reload', async () => {
      const { reloadStatusAndLog } = await ready()
      const before = useGitRefreshStore.getState().changed['ws-test'] ?? 0

      onGitFrame()({ ...status, ahead: 1 })
      await vi.advanceTimersByTimeAsync(500)

      expect(reloadStatusAndLog).toHaveBeenCalled()
      expect(useGitRefreshStore.getState().changed['ws-test']).toBe(before + 1)
    })

    // An edit to a file already modified leaves the status frame as it was; the edit is the news.
    it('reloads when a worktree file changes, though the status frame does not', async () => {
      const { reloadStatusAndLog } = await ready()
      await vi.waitFor(() => expect(heldScopes('files:')).toEqual(['files:ws-test']))
      streamDaemon.deliver({
        t: 'signal',
        scope: 'files:ws-test',
        kind: 'file_change',
        data: { type: 'modified', path: 'calc.py' },
      })
      await vi.advanceTimersByTimeAsync(500)
      expect(reloadStatusAndLog).toHaveBeenCalledTimes(1)
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
  const signal = (wsId: string, data: unknown) =>
    streamDaemon.deliver({ t: 'signal', scope: `git:${wsId}`, kind: 'git_status', data })

  function mount(wsId: 'ws-remote' | 'ws-local') {
    const store = createWorkspaceStore(wsId)
    const reloadStatusAndLog = vi.fn(() => Promise.resolve())
    store.setState({ gitActions: { ...store.getState().gitActions, reloadStatusAndLog } })
    const chat = wsId === 'ws-remote' ? 'chat-remote' : 'chat-local'
    const hook = renderHook(() => useWorkspaceGit(wsId, true, chat, false), {
      wrapper: ({ children }: { children: ReactNode }) => (
        <WorkspaceStoreContext.Provider value={store}>{children}</WorkspaceStoreContext.Provider>
      ),
    })
    return { store, reloadStatusAndLog, ...hook }
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

  it('loads nothing and holds no scope until the project list says whose workspace it is', async () => {
    const remote = mount('ws-remote')
    const local = mount('ws-local')
    expect(fetchGitData).not.toHaveBeenCalled()
    expect(held()).toEqual([])
    expect(remote.store.getState().gitLoad).not.toBe('ready')

    // Each workspace waits for its OWN project's row, not merely for a list.
    act(() => loadProjects(projectOn('p-remote', 'host-1')))
    await waitFor(() => expect(remote.store.getState().gitLoad).toBe('ready'))
    expect(fetchGitData.mock.calls).toEqual([['ws-remote']])
    expect(local.store.getState().gitLoad).toBe('idle')

    act(() => loadProjects(projectOn('p-remote', 'host-1'), projectOn('p-local', 'local')))
    await waitFor(() => expect(local.store.getState().gitLoad).toBe('ready'))
    await waitFor(() => expect(held()).toEqual(['git:ws-local']))
    expect(heldScopes('git:', 'host-1')).toEqual(['git:ws-remote'])
  })

  it('switching local to remote releases the old scope and a late signal on it reloads nothing', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    try {
      loadProjects(projectOn('p-remote', 'host-1'), projectOn('p-local', 'local'))
      const local = mount('ws-local')
      await vi.waitFor(() => expect(held()).toEqual(['git:ws-local']))

      local.unmount()
      const remote = mount('ws-remote')
      await vi.waitFor(() => expect(heldScopes('git:', 'host-1')).toEqual(['git:ws-remote']))
      expect(held()).toEqual([])

      signal('ws-local', { ...status, ahead: 9 })
      await vi.advanceTimersByTimeAsync(500)
      expect(remote.reloadStatusAndLog).not.toHaveBeenCalled()
    } finally {
      vi.useRealTimers()
    }
  })

  it('revoking the host releases its scope and loads nothing more', async () => {
    loadProjects(projectOn('p-remote', 'host-1'))
    const { store } = mount('ws-remote')
    await waitFor(() => expect(store.getState().gitLoad).toBe('ready'))
    await waitFor(() => expect(heldScopes('git:', 'host-1')).toEqual(['git:ws-remote']))
    fetchGitData.mockClear()

    act(() => useHostConfigStore.getState().removeHost('host-1'))

    expect(holding('host-1', 'git:ws-remote')).toBe(false)
    expect(fetchGitData).not.toHaveBeenCalled()
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
    await waitFor(() => expect(held()).toEqual(['git:ws-test']))
    getReviewFiles.mockClear()

    act(() => markGitStatusChanged('ws-test'))

    await waitFor(() => expect(getReviewFiles).toHaveBeenCalled())
    await act(async () => {})
    expect(getReviewFiles).toHaveBeenCalledTimes(1)
  })
})
