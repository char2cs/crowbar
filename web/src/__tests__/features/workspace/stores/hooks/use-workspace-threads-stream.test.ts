import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { setWorkspaceScope } from '@/lib/workspace-scope'

// Hoisted fakes — must be declared before any vi.mock calls.
const { subscribe, listThreadsFn, upsertReviewThread, removeReviewThread } = vi.hoisted(() => ({
  subscribe: vi.fn(() => () => {}),
  listThreadsFn: vi.fn(),
  upsertReviewThread: vi.fn(),
  removeReviewThread: vi.fn(),
}))

vi.mock('@/lib/ws/manager', () => ({
  wsManager: { subscribe, send: vi.fn() },
}))

vi.mock('@/features/git/api/review-api', async (importOriginal) => {
  // Keep mapThread from the real module (it is pure); mock only listThreads.
  const actual = await importOriginal<typeof import('@/features/git/api/review-api')>()
  return {
    ...actual,
    listThreads: (...args: unknown[]) => listThreadsFn(...args),
  }
})

vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getOrCreateWorkspaceStore: () => ({
    getState: () => ({ upsertReviewThread, removeReviewThread }),
  }),
}))

import { useWorkspaceThreadsStream } from '@/features/workspace/stores/hooks/use-workspace-threads-stream'

const WS_BASE = '/v0/projects/p1/repos/r1/workspaces/ws1'

const WIRE_THREAD = {
  id: 'thread-1',
  projectId: 'p1',
  repoId: 'r1',
  workspaceId: 'ws1',
  filePath: 'src/foo.ts',
  line: 10,
  startLine: 8,
  endLine: 12,
  side: 'new' as const,
  body: 'Looks good',
  author: 'alice',
  isAgent: false,
  resolved: false,
  createdAt: '2026-01-01T00:00:00Z',
  replies: [
    {
      id: 'msg-1',
      threadId: 'thread-1',
      author: 'bob',
      isAgent: false,
      body: 'Thanks',
      createdAt: '2026-01-02T00:00:00Z',
    },
  ],
}

beforeEach(() => {
  vi.clearAllMocks()
  setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws1' })
  subscribe.mockReturnValue(() => {})
  listThreadsFn.mockResolvedValue([])
})

describe('useWorkspaceThreadsStream', () => {
  it('subscribes to the workspace-scoped /threads WS endpoint', () => {
    renderHook(() => useWorkspaceThreadsStream('ws1'))
    expect(subscribe).toHaveBeenCalledWith(`${WS_BASE}/threads`, expect.any(Function))
  })

  it('seeds threads via listThreads on mount', async () => {
    listThreadsFn.mockResolvedValue([
      {
        id: 'thread-1',
        filePath: 'src/foo.ts',
        lineNumber: 10,
        startLine: 8,
        endLine: 12,
        side: 'new' as const,
        messages: [],
        isResolved: false,
      },
    ])

    renderHook(() => useWorkspaceThreadsStream('ws1'))

    await waitFor(() => {
      expect(listThreadsFn).toHaveBeenCalledWith('ws1')
    })
    await waitFor(() => {
      expect(upsertReviewThread).toHaveBeenCalledTimes(1)
      expect(upsertReviewThread.mock.calls[0][0]).toMatchObject({
        id: 'thread-1',
        filePath: 'src/foo.ts',
      })
    })
  })

  it('maps a pushed ThreadDTO frame and calls upsertReviewThread', async () => {
    renderHook(() => useWorkspaceThreadsStream('ws1'))

    // Grab the callback registered with wsManager.subscribe
    const [, onFrame] = subscribe.mock.calls[0] as unknown as [string, (frame: unknown) => void]

    onFrame(WIRE_THREAD)

    expect(upsertReviewThread).toHaveBeenCalledWith(
      expect.objectContaining({
        id: 'thread-1',
        filePath: 'src/foo.ts',
        lineNumber: 10,
        startLine: 8,
        endLine: 12,
        side: 'new',
        isResolved: false,
        messages: [
          expect.objectContaining({
            id: 'thread-1:root',
            author: 'alice',
            isAgent: false,
            body: 'Looks good',
          }),
          expect.objectContaining({
            id: 'msg-1',
            author: 'bob',
            isAgent: false,
            body: 'Thanks',
          }),
        ],
      }),
    )
  })

  it('uses the real root message id when the frame carries messageId', () => {
    renderHook(() => useWorkspaceThreadsStream('ws1'))
    const [, onFrame] = subscribe.mock.calls[0] as unknown as [string, (frame: unknown) => void]

    onFrame({ ...WIRE_THREAD, messageId: 'root-msg-1' })

    expect(upsertReviewThread).toHaveBeenCalledWith(
      expect.objectContaining({
        messages: [
          expect.objectContaining({ id: 'root-msg-1', body: 'Looks good' }),
          expect.objectContaining({ id: 'msg-1', body: 'Thanks' }),
        ],
      }),
    )
  })

  it('removes a thread on a tombstone frame (deleted=true) without upserting', () => {
    renderHook(() => useWorkspaceThreadsStream('ws1'))
    const [, onFrame] = subscribe.mock.calls[0] as unknown as [string, (frame: unknown) => void]

    onFrame({ id: 'thread-1', deleted: true })

    expect(removeReviewThread).toHaveBeenCalledWith('thread-1')
    expect(upsertReviewThread).not.toHaveBeenCalled()
  })

  it('re-seeds on reconnect sentinel frame', async () => {
    listThreadsFn.mockResolvedValue([])
    renderHook(() => useWorkspaceThreadsStream('ws1'))

    // Wait for initial seed
    await waitFor(() => expect(listThreadsFn).toHaveBeenCalledTimes(1))

    const [, onFrame] = subscribe.mock.calls[0] as unknown as [string, (frame: unknown) => void]
    onFrame({ reconnected: true })

    await waitFor(() => expect(listThreadsFn).toHaveBeenCalledTimes(2))
  })

  it('unsubscribes on unmount', () => {
    const unsub = vi.fn()
    subscribe.mockReturnValueOnce(unsub)

    const { unmount } = renderHook(() => useWorkspaceThreadsStream('ws1'))
    unmount()

    expect(unsub).toHaveBeenCalledTimes(1)
  })

  it('tears down and re-subscribes when wsId changes', () => {
    const unsubW1 = vi.fn()
    const unsubW2 = vi.fn()
    subscribe.mockReturnValueOnce(unsubW1).mockReturnValueOnce(unsubW2)

    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws1' })
    const { rerender } = renderHook(({ w }: { w: string }) => useWorkspaceThreadsStream(w), {
      initialProps: { w: 'ws1' },
    })
    expect(subscribe).toHaveBeenCalledTimes(1)

    // Register scope for ws2 so workspaceBase resolves
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws2' })
    rerender({ w: 'ws2' })

    expect(unsubW1).toHaveBeenCalledTimes(1)
    expect(subscribe).toHaveBeenCalledTimes(2)
    const calls = subscribe.mock.calls as unknown as [string, (frame: unknown) => void][]
    expect(calls[1][0]).toBe('/v0/projects/p1/repos/r1/workspaces/ws2/threads')
  })
})
