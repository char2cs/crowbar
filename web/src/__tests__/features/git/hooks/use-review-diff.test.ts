import { renderHook, waitFor, act } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { GitDiff } from '@/features/git/types/git-types'
import type { ReviewState } from '@/features/git/api/review-api'

// Mocks must be hoisted before imports that use them
const mocks = vi.hoisted(() => {
  const setBranchReviewDiff = vi.fn()
  const getReview = vi.fn<() => Promise<ReviewState>>()
  const store = { getState: () => ({ setBranchReviewDiff }) }
  return { setBranchReviewDiff, getReview, store }
})

vi.mock('@/features/git/api/review-api', () => ({
  getReview: mocks.getReview,
}))

vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getOrCreateWorkspaceStore: () => mocks.store,
}))

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

describe('useReviewDiff', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('returns empty state with no loading when wsId is null', async () => {
    const { result } = renderHook(() => useReviewDiff(null))
    expect(result.current.files).toEqual([])
    expect(result.current.uncommittedCount).toBe(0)
    expect(result.current.loading).toBe(false)
    expect(mocks.getReview).not.toHaveBeenCalled()
  })

  it('fetches review on mount and surfaces files', async () => {
    const files = [makeDiff('src/foo.ts'), makeDiff('src/bar.ts')]
    mocks.getReview.mockResolvedValue(makeReview(files))

    const { result } = renderHook(() => useReviewDiff('ws-1'))

    await waitFor(() => expect(result.current.loading).toBe(false))

    expect(mocks.getReview).toHaveBeenCalledWith('ws-1')
    expect(result.current.files).toHaveLength(2)
    expect(result.current.files[0].file_path).toBe('src/foo.ts')
    expect(mocks.setBranchReviewDiff).toHaveBeenCalledTimes(1)
  })

  it('derives uncommittedCount from files with uncommitted=true', async () => {
    const files = [makeDiff('a.ts', true), makeDiff('b.ts', false), makeDiff('c.ts', true)]
    mocks.getReview.mockResolvedValue(makeReview(files))

    const { result } = renderHook(() => useReviewDiff('ws-1'))

    await waitFor(() => expect(result.current.loading).toBe(false))
    expect(result.current.uncommittedCount).toBe(2)
  })

  it('re-fetches when git-status-changed fires', async () => {
    const initial = [makeDiff('src/a.ts')]
    const updated = [makeDiff('src/a.ts'), makeDiff('src/b.ts')]
    mocks.getReview.mockResolvedValueOnce(makeReview(initial)).mockResolvedValueOnce(makeReview(updated))

    const { result } = renderHook(() => useReviewDiff('ws-1'))
    await waitFor(() => expect(result.current.files).toHaveLength(1))

    act(() => {
      window.dispatchEvent(new Event('git-status-changed'))
    })

    await waitFor(() => expect(result.current.files).toHaveLength(2))
    expect(mocks.getReview).toHaveBeenCalledTimes(2)
  })
})
