import type { ReactNode } from 'react'
import { describe, it, expect, beforeAll, beforeEach, afterEach, vi } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { useWorkspaceGit } from '@/features/workspace/stores/hooks/use-workspace-git'
import { requestGitRefresh, useGitRefreshStore } from '@/features/git/stores/git-refresh'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { setWorkspaceScope } from '@/lib/workspace-scope'

const { fetchGitData, subscribe } = vi.hoisted(() => ({
  fetchGitData: vi.fn(),
  subscribe: vi.fn(() => () => {}),
}))

vi.mock('@/features/git/api/git-data-api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/features/git/api/git-data-api')>()
  return { ...actual, fetchGitData }
})
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
