import { renderHook, act, waitFor } from '@testing-library/react'
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import type { FileOutline } from '@/features/git/api/review-window-api'

const mocks = vi.hoisted(() => ({
  getReviewOutline: vi.fn<() => Promise<FileOutline[]>>(),
}))
vi.mock('@/features/git/api/review-window-api', () => ({
  getReviewOutline: mocks.getReviewOutline,
}))

import { useReviewOutline } from '@/features/git/hooks/use-review-outline'
import { markGitStatusChanged } from '@/features/git/stores/git-refresh'
import {
  __resetWorkspaceScopesForTest,
  recordWorkspaceScope,
  setWorkspaceScope,
} from '@/lib/workspace-scope'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

function outlineFile(path: string): FileOutline {
  return { path, hunks: [], isPartial: false, isBinary: false }
}

function fireGitStatusChanged(wsId = 'ws1'): void {
  markGitStatusChanged(wsId)
}

describe('useReviewOutline', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.getReviewOutline.mockResolvedValue([])
    __resetWorkspaceScopesForTest()
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws1', owningChatId: 'chat1' })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('does not fetch and stays empty when wsId is null', async () => {
    const { result } = renderHook(() => useReviewOutline(null))
    await act(async () => {})
    expect(result.current.outline).toEqual([])
    expect(result.current.loaded).toBe(false)
    expect(mocks.getReviewOutline).not.toHaveBeenCalled()
  })

  it('fetches on mount', async () => {
    mocks.getReviewOutline.mockResolvedValue([outlineFile('src/a.ts')])

    const { result } = renderHook(() => useReviewOutline('ws1'))

    await waitFor(() => expect(result.current.loaded).toBe(true))
    expect(mocks.getReviewOutline).toHaveBeenCalledWith({ wsId: 'ws1', commit: undefined })
    expect(result.current.outline).toEqual([outlineFile('src/a.ts')])
  })

  it('refetches on a git status change', async () => {
    vi.useFakeTimers()
    mocks.getReviewOutline.mockResolvedValue([outlineFile('src/a.ts')])

    renderHook(() => useReviewOutline('ws1'))
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0)
    })
    expect(mocks.getReviewOutline).toHaveBeenCalledTimes(1)

    act(() => {
      fireGitStatusChanged()
    })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(300)
    })
    expect(mocks.getReviewOutline).toHaveBeenCalledTimes(2)
  })

  it('does not refetch on a git status change when scoped to a commit', async () => {
    vi.useFakeTimers()
    mocks.getReviewOutline.mockResolvedValue([outlineFile('src/a.ts')])

    renderHook(() => useReviewOutline('ws1', 'abc123'))
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0)
    })
    expect(mocks.getReviewOutline).toHaveBeenCalledWith({ wsId: 'ws1', commit: 'abc123' })

    act(() => {
      fireGitStatusChanged()
    })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(300)
    })
    expect(mocks.getReviewOutline).toHaveBeenCalledTimes(1)
  })

  it('swallows a fetch failure and keeps an empty, unloaded result', async () => {
    mocks.getReviewOutline.mockRejectedValue(new Error('boom'))

    const { result } = renderHook(() => useReviewOutline('ws1'))
    await act(async () => {})

    expect(result.current.outline).toEqual([])
    expect(result.current.loaded).toBe(false)
  })

  // Regression: the route records a workspace's scope with NO chat id; only
  // the sidebar's own async chat-list fetch later attaches owningChatId.
  // getReviewOutline resolves through reviewBaseForWorkspace, which throws
  // without one; firing anyway hit the throw, landed in the swallowing catch,
  // and left the outline empty until an UNRELATED git status change
  // happened to retry it.
  describe('owning chat id not yet recorded (route-vs-sidebar race)', () => {
    it('does not fetch before an owning chat id is recorded', async () => {
      setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-race' })

      const { result } = renderHook(() => useReviewOutline('ws-race'))
      await act(async () => {})

      expect(mocks.getReviewOutline).not.toHaveBeenCalled()
      expect(result.current.outline).toEqual([])
      expect(result.current.loaded).toBe(false)
    })

    it('fetches once the owning chat id arrives after mount', async () => {
      setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-race' })
      mocks.getReviewOutline.mockResolvedValue([outlineFile('src/a.ts')])

      const { result } = renderHook(() => useReviewOutline('ws-race'))
      await act(async () => {})
      expect(mocks.getReviewOutline).not.toHaveBeenCalled()

      act(() => {
        recordWorkspaceScope({
          projectId: 'p1',
          repoId: 'r1',
          wsId: 'ws-race',
          owningChatId: 'chat-race',
        })
      })

      await waitFor(() => expect(result.current.loaded).toBe(true))
      expect(mocks.getReviewOutline).toHaveBeenCalledWith({ wsId: 'ws-race', commit: undefined })
      expect(result.current.outline).toEqual([outlineFile('src/a.ts')])
    })
  })
})

// The sidebar can draw a workspace from its cache before the project list says
// whose host it is; a failed fetch there is swallowed and would never be retried.
describe('useReviewOutline on an install that has paired a remote host', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.getReviewOutline.mockResolvedValue([outlineFile('src/a.ts')])
    resetHostWorld()
    pairRemoteHost('host-1')
    __resetWorkspaceScopesForTest()
    setWorkspaceScope({
      projectId: 'p-remote',
      repoId: 'r1',
      wsId: 'ws-remote',
      owningChatId: 'chat-remote',
    })
  })
  afterEach(() => resetHostWorld())

  it('asks nothing until the project list names the host, then fetches', async () => {
    const { result } = renderHook(() => useReviewOutline('ws-remote'))
    await act(async () => {})
    expect(mocks.getReviewOutline).not.toHaveBeenCalled()
    expect(result.current.loaded).toBe(false)

    act(() => loadProjects(projectOn('p-remote', 'host-1')))

    await waitFor(() => expect(result.current.loaded).toBe(true))
    expect(mocks.getReviewOutline).toHaveBeenCalledWith({ wsId: 'ws-remote', commit: undefined })
  })
})
