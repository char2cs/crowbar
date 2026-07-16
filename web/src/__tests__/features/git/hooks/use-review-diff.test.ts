import { renderHook, waitFor, act, render } from '@testing-library/react'
import { createElement, memo } from 'react'
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import type { GitDiff } from '@/features/git/types/git-types'
import type { ReviewState } from '@/features/git/api/review-api'
import {
  getOrCreateWorkspaceStore,
  destroyWorkspaceStore,
  getAllActiveWorkspaceIds,
} from '@/features/workspace/stores/workspace-store-registry'

// Mocks must be hoisted before imports that use them
const mocks = vi.hoisted(() => {
  const getReview = vi.fn<() => Promise<ReviewState>>()
  return { getReview }
})

vi.mock('@/features/git/api/review-api', () => ({
  getReview: mocks.getReview,
}))

// The workspace store registry is NOT mocked: these tests rely on the real
// branch-review-slice equality gate (setBranchReviewDiff) to prove reference
// stability, so `getOrCreateWorkspaceStore` must return a real store.

// Import hook after mocks are wired
import { useReviewDiff } from '@/features/git/hooks/use-review-diff'

function makeDiff(filePath: string, uncommitted = false): GitDiff {
  return {
    file_path: filePath,
    is_new: false,
    is_deleted: false,
    is_renamed: false,
    lines: [],
    uncommitted,
  }
}

function makeReview(files: GitDiff[]): ReviewState {
  return {
    description: '',
    mergeStrategy: 'merge',
    diff: {
      commitHash: 'branch-head',
      files,
      totalFiles: files.length,
      totalAdditions: 0,
      totalDeletions: 0,
    },
    threads: [],
    conversations: [],
  }
}

/** Advance fake timers AND flush the resulting microtask/React-update chain,
 * wrapped in `act` so `result.current` / child renders reflect the update. */
async function tick(ms: number): Promise<void> {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms)
  })
}

describe('useReviewDiff', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    getAllActiveWorkspaceIds().forEach((id) => destroyWorkspaceStore(id))
  })

  it('returns empty state with no loading when wsId is null', async () => {
    const { result } = renderHook(() => useReviewDiff(null))
    expect(result.current.files).toEqual([])
    expect(result.current.uncommittedCount).toBe(0)
    expect(result.current.loading).toBe(false)
    expect(mocks.getReview).not.toHaveBeenCalled()
  })

  it('makes zero fetches and registers no git-status listener when disabled', async () => {
    const addSpy = vi.spyOn(window, 'addEventListener')

    const { result } = renderHook(() => useReviewDiff('ws-disabled', { enabled: false }))
    // Flush any (erroneously scheduled) async mount effect.
    await act(async () => {})

    expect(mocks.getReview).not.toHaveBeenCalled()
    expect(result.current.files).toEqual([])
    expect(result.current.uncommittedCount).toBe(0)
    expect(result.current.loading).toBe(false)
    expect(result.current.stale).toBe(true)

    const gitStatusAdds = addSpy.mock.calls.filter(
      ([evt]) => (evt as string) === 'git-status-changed',
    )
    expect(gitStatusAdds).toHaveLength(0)

    addSpy.mockRestore()
  })

  it('adds and removes the git-status-changed listener symmetrically across the enabled lifecycle', async () => {
    mocks.getReview.mockResolvedValue(makeReview([makeDiff('a.ts')]))
    const addSpy = vi.spyOn(window, 'addEventListener')
    const removeSpy = vi.spyOn(window, 'removeEventListener')
    const count = (spy: typeof addSpy) =>
      spy.mock.calls.filter(([evt]) => (evt as string) === 'git-status-changed').length

    const { rerender, unmount } = renderHook(
      ({ enabled }: { enabled: boolean }) => useReviewDiff('ws-symmetry', { enabled }),
      { initialProps: { enabled: false } },
    )
    await act(async () => {})
    expect(count(addSpy)).toBe(0)

    // Enable → exactly one listener registered, none removed yet.
    rerender({ enabled: true })
    await act(async () => {})
    expect(count(addSpy)).toBe(1)
    expect(count(removeSpy)).toBe(0)

    // Disable → the effect cleanup removes the one listener it added.
    rerender({ enabled: false })
    await act(async () => {})
    expect(count(removeSpy)).toBe(1)

    // Already disabled: unmount adds/removes nothing more — add/remove stay in balance.
    unmount()
    expect(count(addSpy)).toBe(1)
    expect(count(removeSpy)).toBe(1)

    addSpy.mockRestore()
    removeSpy.mockRestore()
  })

  it('marks the result stale on re-enable until a fresh fetch completes (cached files are exposed but flagged)', async () => {
    vi.useFakeTimers()
    try {
      mocks.getReview.mockResolvedValueOnce(makeReview([makeDiff('src/d1.ts')]))

      const { result, rerender } = renderHook(
        ({ enabled }: { enabled: boolean }) => useReviewDiff('ws-stale', { enabled }),
        { initialProps: { enabled: true } },
      )
      await tick(0) // flush the initial mount fetch
      expect(result.current.stale).toBe(false)
      expect(result.current.files.map((f) => f.file_path)).toEqual(['src/d1.ts'])

      // Disable → empty state, stale (no fresh data for a disabled session).
      rerender({ enabled: false })
      expect(result.current.files).toEqual([])
      expect(result.current.stale).toBe(true)

      // Re-enable with a deferred second fetch: the cached diff files
      // reappear immediately (diffCache survives the disable by design) but
      // stay flagged stale until THIS session's fetch completes.
      let resolveSecond!: (v: ReviewState) => void
      mocks.getReview.mockImplementationOnce(() => new Promise((r) => (resolveSecond = r)))
      rerender({ enabled: true })
      await act(async () => {}) // flush the re-enable effect (fetch starts, unresolved)
      expect(mocks.getReview).toHaveBeenCalledTimes(2)
      expect(result.current.files.map((f) => f.file_path)).toEqual(['src/d1.ts'])
      expect(result.current.stale).toBe(true)

      // Fresh fetch lands → stale clears and the new payload is exposed.
      await act(async () => {
        resolveSecond(makeReview([makeDiff('src/d2-a.ts'), makeDiff('src/d2-b.ts')]))
      })
      expect(result.current.stale).toBe(false)
      expect(result.current.files.map((f) => f.file_path)).toEqual(['src/d2-a.ts', 'src/d2-b.ts'])
    } finally {
      vi.useRealTimers()
    }
  })

  it('stays fresh across debounced refetches while enabled (no stale flicker on git ticks)', async () => {
    vi.useFakeTimers()
    try {
      mocks.getReview
        .mockResolvedValueOnce(makeReview([makeDiff('src/a.ts')]))
        .mockImplementationOnce(() => new Promise(() => {})) // refetch never resolves

      const { result } = renderHook(() => useReviewDiff('ws-fresh-hold'))
      await tick(0)
      expect(result.current.stale).toBe(false)

      // A git tick triggers the debounced refetch; while it is in flight the
      // already-validated session data must NOT flip back to stale.
      act(() => {
        window.dispatchEvent(new Event('git-status-changed'))
      })
      await tick(250)
      expect(mocks.getReview).toHaveBeenCalledTimes(2)
      expect(result.current.loading).toBe(true)
      expect(result.current.stale).toBe(false)
    } finally {
      vi.useRealTimers()
    }
  })

  it('fetches review on mount, surfaces files, and caches the diff on the workspace store', async () => {
    const files = [makeDiff('src/foo.ts'), makeDiff('src/bar.ts')]
    mocks.getReview.mockResolvedValue(makeReview(files))

    const { result } = renderHook(() => useReviewDiff('ws-mount'))

    await waitFor(() => expect(result.current.loading).toBe(false))

    expect(mocks.getReview).toHaveBeenCalledWith('ws-mount')
    expect(result.current.files).toHaveLength(2)
    expect(result.current.files[0].file_path).toBe('src/foo.ts')

    const { diffCache, diffStatus } = getOrCreateWorkspaceStore('ws-mount').getState().branchReview
    expect(diffStatus).toBe('loaded')
    expect(diffCache?.files).toHaveLength(2)
  })

  it('derives uncommittedCount from files with uncommitted=true', async () => {
    const files = [makeDiff('a.ts', true), makeDiff('b.ts', false), makeDiff('c.ts', true)]
    mocks.getReview.mockResolvedValue(makeReview(files))

    const { result } = renderHook(() => useReviewDiff('ws-uncommitted'))

    await waitFor(() => expect(result.current.loading).toBe(false))
    expect(result.current.uncommittedCount).toBe(2)
  })

  it('re-fetches (only after the 250ms trailing debounce window) when git-status-changed fires', async () => {
    vi.useFakeTimers()
    try {
      const initial = [makeDiff('src/a.ts')]
      const updated = [makeDiff('src/a.ts'), makeDiff('src/b.ts')]
      mocks.getReview
        .mockResolvedValueOnce(makeReview(initial))
        .mockResolvedValueOnce(makeReview(updated))

      const { result } = renderHook(() => useReviewDiff('ws-refetch'))
      await tick(0) // flush the initial (un-debounced) mount fetch
      expect(result.current.files).toHaveLength(1)

      act(() => {
        window.dispatchEvent(new Event('git-status-changed'))
      })

      // Still inside the 250ms trailing debounce window: no second fetch yet.
      await tick(200)
      expect(mocks.getReview).toHaveBeenCalledTimes(1)

      // Past the debounce window: the trailing refetch has now fired.
      await tick(100)
      expect(mocks.getReview).toHaveBeenCalledTimes(2)
      expect(result.current.files).toHaveLength(2)
    } finally {
      vi.useRealTimers()
    }
  })

  it('coalesces a burst of git-status-changed events into a single trailing-debounced refetch', async () => {
    vi.useFakeTimers()
    try {
      mocks.getReview.mockResolvedValue(makeReview([makeDiff('src/a.ts')]))

      renderHook(() => useReviewDiff('ws-burst'))
      await tick(0) // flush the initial mount fetch
      expect(mocks.getReview).toHaveBeenCalledTimes(1)

      // 5 events spread over 200ms — each one arrives well within the prior
      // event's 250ms trailing window, so the timer keeps resetting.
      for (let i = 0; i < 5; i++) {
        act(() => {
          window.dispatchEvent(new Event('git-status-changed'))
        })
        await tick(50)
      }
      // Only 50ms have elapsed since the LAST event in the burst — still debounced.
      expect(mocks.getReview).toHaveBeenCalledTimes(1)

      // Past the debounce window measured from the last event: exactly one
      // additional (trailing) refetch fires for the whole burst.
      await tick(250)
      expect(mocks.getReview).toHaveBeenCalledTimes(2)
    } finally {
      vi.useRealTimers()
    }
  })

  it('skips the store write — diffCache keeps the same reference — when a refetch returns an unchanged payload', async () => {
    vi.useFakeTimers()
    try {
      // Two calls resolve with DIFFERENT object instances but identical
      // content, mirroring two independent responses for an unchanged diff.
      mocks.getReview
        .mockResolvedValueOnce(makeReview([makeDiff('src/a.ts')]))
        .mockResolvedValueOnce(makeReview([makeDiff('src/a.ts')]))

      renderHook(() => useReviewDiff('ws-dedupe'))
      await tick(0)

      const store = getOrCreateWorkspaceStore('ws-dedupe')
      const diffCacheAfterFirst = store.getState().branchReview.diffCache
      expect(diffCacheAfterFirst?.files).toHaveLength(1)

      act(() => {
        window.dispatchEvent(new Event('git-status-changed'))
      })
      await tick(250)

      expect(mocks.getReview).toHaveBeenCalledTimes(2)
      expect(store.getState().branchReview.diffCache).toBe(diffCacheAfterFirst)
    } finally {
      vi.useRealTimers()
    }
  })

  it('re-renders a files-consuming child only when the files actually changed', async () => {
    vi.useFakeTimers()
    try {
      mocks.getReview
        .mockResolvedValueOnce(makeReview([makeDiff('src/a.ts')]))
        .mockResolvedValueOnce(makeReview([makeDiff('src/a.ts')])) // unchanged content
        .mockResolvedValueOnce(makeReview([makeDiff('src/a.ts'), makeDiff('src/b.ts')])) // changed

      const childRenders = vi.fn()
      const Child = memo(function Child({ files }: { files: GitDiff[] }) {
        childRenders(files.length)
        return null
      })
      function Harness({ wsId }: { wsId: string }) {
        const { files } = useReviewDiff(wsId)
        return createElement(Child, { files })
      }

      render(createElement(Harness, { wsId: 'ws-render' }))
      await tick(0)
      // Whatever it takes to settle the initial mount + fetch (mount render,
      // loading flip(s), first files write) is the baseline — what matters
      // below is the DELTA from here, not this absolute number.
      const baseline = childRenders.mock.calls.length
      expect(baseline).toBeGreaterThan(0)

      // Unchanged refetch: the store write is skipped (same diffCache
      // reference), so the memoized child must NOT re-render even though the
      // hook's `loading` flag still toggles true/false around the fetch.
      act(() => {
        window.dispatchEvent(new Event('git-status-changed'))
      })
      await tick(250)
      expect(mocks.getReview).toHaveBeenCalledTimes(2)
      expect(childRenders.mock.calls.length).toBe(baseline)

      // Changed refetch: files reference changes, so the child renders
      // exactly once more.
      act(() => {
        window.dispatchEvent(new Event('git-status-changed'))
      })
      await tick(250)
      expect(mocks.getReview).toHaveBeenCalledTimes(3)
      expect(childRenders.mock.calls.length).toBe(baseline + 1)
    } finally {
      vi.useRealTimers()
    }
  })
})
