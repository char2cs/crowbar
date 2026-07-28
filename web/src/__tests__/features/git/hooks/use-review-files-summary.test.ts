import { renderHook, act, waitFor } from '@testing-library/react'
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import type { ReviewFileSummary } from '@/features/git/api/review-api'

const mocks = vi.hoisted(() => ({
  getReviewFiles: vi.fn<() => Promise<ReviewFileSummary[]>>(),
}))
vi.mock('@/features/git/api/review-api', () => ({
  getReviewFiles: mocks.getReviewFiles,
}))

import { useReviewFilesSummary } from '@/features/git/hooks/use-review-files-summary'

function summaryFile(
  path: string,
  status: ReviewFileSummary['status'] = 'modified',
  extra: Partial<ReviewFileSummary> = {},
): ReviewFileSummary {
  return { path, status, additions: 1, deletions: 0, uncommitted: true, staged: false, ...extra }
}

function fireGitStatusChanged(): void {
  window.dispatchEvent(new Event('git-status-changed'))
}

describe('useReviewFilesSummary', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.getReviewFiles.mockResolvedValue([])
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('does not fetch and stays empty when wsId is null', async () => {
    const { result } = renderHook(() => useReviewFilesSummary(null))
    await act(async () => {})
    expect(result.current.files).toEqual([])
    expect(result.current.loaded).toBe(false)
    expect(mocks.getReviewFiles).not.toHaveBeenCalled()
  })

  it('fetches on mount and maps the summary into GitDiff shape', async () => {
    mocks.getReviewFiles.mockResolvedValue([
      summaryFile('src/a.ts', 'added', { additions: 4, uncommitted: false }),
      summaryFile('src/b.ts', 'renamed', { old_path: 'src/old.ts', additions: 2, deletions: 3 }),
    ])

    const { result } = renderHook(() => useReviewFilesSummary('ws1'))

    await waitFor(() => expect(result.current.loaded).toBe(true))
    expect(mocks.getReviewFiles).toHaveBeenCalledWith({ wsId: 'ws1', commit: undefined })
    expect(result.current.files).toEqual([
      {
        file_path: 'src/a.ts',
        old_path: undefined,
        is_new: true,
        is_deleted: false,
        is_renamed: false,
        additions: 4,
        deletions: 0,
        uncommitted: false,
        lines: [],
      },
      {
        file_path: 'src/b.ts',
        old_path: 'src/old.ts',
        is_new: false,
        is_deleted: false,
        is_renamed: true,
        additions: 2,
        deletions: 3,
        uncommitted: true,
        lines: [],
      },
    ])
  })

  it('coalesces a burst of git-status-changed events into a single debounced refetch', async () => {
    vi.useFakeTimers()
    mocks.getReviewFiles.mockResolvedValue([summaryFile('src/a.ts')])

    renderHook(() => useReviewFilesSummary('ws1'))
    // Initial mount fetch (not debounced).
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0)
    })
    expect(mocks.getReviewFiles).toHaveBeenCalledTimes(1)

    // A burst of ticks within the debounce window.
    act(() => {
      fireGitStatusChanged()
      fireGitStatusChanged()
      fireGitStatusChanged()
    })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(200)
    })
    expect(mocks.getReviewFiles).toHaveBeenCalledTimes(1)

    // Crossing 250ms fires exactly one refetch for the whole burst.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(60)
    })
    expect(mocks.getReviewFiles).toHaveBeenCalledTimes(2)
  })

  it('keeps a stable files reference when a refetch returns an identical list', async () => {
    vi.useFakeTimers()
    mocks.getReviewFiles.mockResolvedValue([summaryFile('src/a.ts', 'modified', { additions: 1 })])

    const { result } = renderHook(() => useReviewFilesSummary('ws1'))
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0)
    })
    const first = result.current.files
    expect(first).toHaveLength(1)

    // An identical refetch must be skipped by the deep-equal gate.
    act(() => {
      fireGitStatusChanged()
    })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(300)
    })
    expect(mocks.getReviewFiles).toHaveBeenCalledTimes(2)
    expect(result.current.files).toBe(first)
  })

  it('resets to empty/unloaded when the workspace changes', async () => {
    mocks.getReviewFiles.mockResolvedValueOnce([summaryFile('src/a.ts')])

    const { result, rerender } = renderHook(({ ws }) => useReviewFilesSummary(ws), {
      initialProps: { ws: 'ws1' as string | null },
    })
    await waitFor(() => expect(result.current.loaded).toBe(true))
    expect(result.current.files).toHaveLength(1)

    // Second workspace: the summary fetch is left pending, so the reset must be
    // visible synchronously — no stale list from ws1.
    mocks.getReviewFiles.mockImplementationOnce(() => new Promise(() => {}))
    rerender({ ws: 'ws2' })
    expect(result.current.loaded).toBe(false)
    expect(result.current.files).toEqual([])
  })

  it('swallows a fetch failure and keeps an empty, unloaded result', async () => {
    mocks.getReviewFiles.mockRejectedValue(new Error('boom'))

    const { result } = renderHook(() => useReviewFilesSummary('ws1'))
    await act(async () => {})

    expect(result.current.files).toEqual([])
    expect(result.current.loaded).toBe(false)
  })
})
