import { renderHook, act, waitFor } from '@testing-library/react'
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import type { ReviewFileSummary } from '@/features/git/api/review-api'
import type { GitDiff } from '@/features/git/types/git-types'

const mocks = vi.hoisted(() => ({
  getReviewFiles: vi.fn<() => Promise<ReviewFileSummary[]>>(),
}))
vi.mock('@/features/git/api/review-api', () => ({
  getReviewFiles: mocks.getReviewFiles,
}))

import { useReviewFilesSummary } from '@/features/git/hooks/use-review-files-summary'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
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

function summaryFile(
  path: string,
  status: ReviewFileSummary['status'] = 'modified',
  extra: Partial<ReviewFileSummary> = {},
): ReviewFileSummary {
  return { path, status, additions: 1, deletions: 0, uncommitted: true, staged: false, ...extra }
}

function changed(path: string): GitDiff {
  return {
    file_path: path,
    is_new: false,
    is_deleted: false,
    is_renamed: false,
    additions: 1,
    deletions: 0,
    lines: [],
  }
}

describe('useReviewFilesSummary', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.getReviewFiles.mockResolvedValue([])
    __resetWorkspaceScopesForTest()
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws1', owningChatId: 'chat1' })
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws2', owningChatId: 'chat2' })
  })

  afterEach(() => {
    destroyWorkspaceStore('ws1')
    destroyWorkspaceStore('ws2')
  })

  describe('workspace scope', () => {
    it('is empty and unloaded for a null wsId, without fetching', () => {
      const { result } = renderHook(() => useReviewFilesSummary(null))
      expect(result.current).toEqual({ files: [], loaded: false })
      expect(mocks.getReviewFiles).not.toHaveBeenCalled()
    })

    it("reads the workspace store's summary and never fetches itself", async () => {
      const store = getOrCreateWorkspaceStore('ws1')
      const { result } = renderHook(() => useReviewFilesSummary('ws1'))
      expect(result.current.loaded).toBe(false)

      act(() => store.setState({ reviewFiles: [changed('src/a.ts')] }))

      expect(result.current.loaded).toBe(true)
      expect(result.current.files).toEqual([changed('src/a.ts')])
      expect(mocks.getReviewFiles).not.toHaveBeenCalled()
    })

    it('follows the workspace it is given', () => {
      getOrCreateWorkspaceStore('ws1').setState({ reviewFiles: [changed('src/a.ts')] })
      const { result, rerender } = renderHook(({ ws }) => useReviewFilesSummary(ws), {
        initialProps: { ws: 'ws1' as string | null },
      })
      expect(result.current.files).toHaveLength(1)

      rerender({ ws: 'ws2' })
      expect(result.current).toEqual({ files: [], loaded: false })
    })
  })

  describe('commit scope', () => {
    it('fetches once and maps the summary into GitDiff shape', async () => {
      mocks.getReviewFiles.mockResolvedValue([
        summaryFile('src/a.ts', 'added', { additions: 4, uncommitted: false }),
        summaryFile('src/b.ts', 'renamed', { old_path: 'src/old.ts', additions: 2, deletions: 3 }),
      ])

      const { result } = renderHook(() => useReviewFilesSummary('ws1', 'abc'))

      await waitFor(() => expect(result.current.loaded).toBe(true))
      expect(mocks.getReviewFiles).toHaveBeenCalledTimes(1)
      expect(mocks.getReviewFiles).toHaveBeenCalledWith({ wsId: 'ws1', commit: 'abc' })
      expect(result.current.files.map((f) => f.file_path)).toEqual(['src/a.ts', 'src/b.ts'])
      expect(result.current.files[0]).toMatchObject({ is_new: true, additions: 4 })
      expect(result.current.files[1]).toMatchObject({
        old_path: 'src/old.ts',
        is_renamed: true,
        deletions: 3,
      })
    })

    it('resets to empty/unloaded when the commit changes', async () => {
      mocks.getReviewFiles.mockResolvedValueOnce([summaryFile('src/a.ts')])
      const { result, rerender } = renderHook(({ c }) => useReviewFilesSummary('ws1', c), {
        initialProps: { c: 'abc' },
      })
      await waitFor(() => expect(result.current.loaded).toBe(true))

      mocks.getReviewFiles.mockImplementationOnce(() => new Promise(() => {}))
      rerender({ c: 'def' })
      expect(result.current).toEqual({ files: [], loaded: false })
    })

    it('swallows a fetch failure and keeps an empty, unloaded result', async () => {
      mocks.getReviewFiles.mockRejectedValue(new Error('boom'))

      const { result } = renderHook(() => useReviewFilesSummary('ws1', 'abc'))
      await act(async () => {})

      expect(result.current).toEqual({ files: [], loaded: false })
    })

    // The chat-scoped route throws without an owning chat id, which the sidebar
    // records after the route does.
    it('waits for the owning chat id, then fetches', async () => {
      setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-race' })
      mocks.getReviewFiles.mockResolvedValue([summaryFile('src/a.ts')])

      const { result } = renderHook(() => useReviewFilesSummary('ws-race', 'abc'))
      await act(async () => {})
      expect(mocks.getReviewFiles).not.toHaveBeenCalled()

      act(() => {
        recordWorkspaceScope({
          projectId: 'p1',
          repoId: 'r1',
          wsId: 'ws-race',
          owningChatId: 'chat-race',
        })
      })

      await waitFor(() => expect(result.current.loaded).toBe(true))
      expect(mocks.getReviewFiles).toHaveBeenCalledWith({ wsId: 'ws-race', commit: 'abc' })
    })
  })
})

describe('useReviewFilesSummary on an install that has paired a remote host', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.getReviewFiles.mockResolvedValue([summaryFile('src/a.ts')])
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
    const { result } = renderHook(() => useReviewFilesSummary('ws-remote'))
    await act(async () => {})
    expect(mocks.getReviewFiles).not.toHaveBeenCalled()
    expect(result.current.loaded).toBe(false)

    act(() => loadProjects(projectOn('p-remote', 'host-1')))

    await waitFor(() => expect(result.current.loaded).toBe(true))
    expect(mocks.getReviewFiles).toHaveBeenCalledWith({ wsId: 'ws-remote', commit: undefined })
  })
})
