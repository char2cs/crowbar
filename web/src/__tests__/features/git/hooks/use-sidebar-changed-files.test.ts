import { renderHook, act, waitFor, cleanup } from '@testing-library/react'
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import type { GitDiff, GitFile, GitStatus } from '@/features/git/types/git-types'
import type { ReviewState, ReviewFileSummary } from '@/features/git/api/review-api'
import type { BranchReviewContent } from '@/features/panes/types/pane-content'
import { useGitStore } from '@/features/git/stores/git-store'
import {
  getOrCreateWorkspaceStore,
  destroyWorkspaceStore,
  getAllActiveWorkspaceIds,
} from '@/features/workspace/stores/workspace-store-registry'

// Mock only the network layer; the real workspace + git stores drive the wiring.
// getReview is the FULL line-level diff (gated behind the open pane);
// getReviewFiles is the cheap always-on files-only summary (Task 27).
const mocks = vi.hoisted(() => ({
  getReview: vi.fn<() => Promise<ReviewState>>(),
  getReviewFiles: vi.fn<() => Promise<ReviewFileSummary[]>>(),
}))
vi.mock('@/features/git/api/review-api', () => ({
  getReview: mocks.getReview,
  getReviewFiles: mocks.getReviewFiles,
}))

import { useSidebarChangedFiles } from '@/features/git/hooks/use-sidebar-changed-files'

function sf(path: string, status: GitFile['status'], staged = false): GitFile {
  return { path, status, staged }
}

function makeStatus(files: GitFile[]): GitStatus {
  return { branch: 'feature', ahead: 0, behind: 0, files }
}

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

function summaryFile(
  path: string,
  status: ReviewFileSummary['status'],
  extra: Partial<ReviewFileSummary> = {},
): ReviewFileSummary {
  return {
    path,
    status,
    additions: 0,
    deletions: 0,
    uncommitted: false,
    staged: false,
    ...extra,
  }
}

function branchReviewBuffer(wsId: string): BranchReviewContent {
  return {
    id: `br-${wsId}`,
    type: 'branchReview',
    wsId,
    name: 'Branch Review',
    path: `branch-review://${wsId}`,
    isPinned: false,
    isPreview: false,
    isActive: false,
  }
}

function openReviewPane(wsId: string): void {
  getOrCreateWorkspaceStore(wsId).setState({ buffers: [branchReviewBuffer(wsId)] })
}

describe('useSidebarChangedFiles', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Default: the summary resolves empty so a test that does not care about it
    // never leaves the fetch pending; tests that assert list content override it.
    mocks.getReviewFiles.mockResolvedValue([])
    useGitStore.setState({ gitStatus: null })
  })

  afterEach(() => {
    // Unmount hooks BEFORE tearing down the stores they subscribe to, so a
    // store reset never updates a still-mounted component (act warning).
    cleanup()
    getAllActiveWorkspaceIds().forEach((id) => destroyWorkspaceStore(id))
    useGitStore.setState({ gitStatus: null })
  })

  it('returns empty and does not fetch when wsId is null', async () => {
    const { result } = renderHook(() => useSidebarChangedFiles(null))
    await act(async () => {})
    expect(result.current.files).toEqual([])
    expect(result.current.uncommittedCount).toBe(0)
    expect(mocks.getReview).not.toHaveBeenCalled()
    expect(mocks.getReviewFiles).not.toHaveBeenCalled()
  })

  it('paints the status projection instantly, then upgrades to the summary (with +N/-N counts)', async () => {
    useGitStore.setState({
      gitStatus: makeStatus([sf('src/a.ts', 'modified'), sf('src/b.ts', 'added')]),
    })
    // Hold the summary so we can observe the instant-first-paint projection.
    let resolveSummary!: (v: ReviewFileSummary[]) => void
    mocks.getReviewFiles.mockImplementationOnce(() => new Promise((r) => (resolveSummary = r)))

    const { result } = renderHook(() => useSidebarChangedFiles('ws-closed'))

    // Instant first paint: the cheap projection, no per-file counts yet.
    expect(result.current.files.map((f) => f.file_path)).toEqual(['src/a.ts', 'src/b.ts'])
    expect(result.current.files[1].is_new).toBe(true)
    expect(result.current.files[0].additions).toBeUndefined()

    // The full line-level diff is NEVER fetched while the pane is closed.
    expect(mocks.getReview).not.toHaveBeenCalled()
    expect(mocks.getReviewFiles).toHaveBeenCalledWith('ws-closed')

    // The summary lands → the list upgrades in place, now carrying counts.
    await act(async () => {
      resolveSummary([
        summaryFile('src/a.ts', 'modified', { additions: 3, deletions: 1, uncommitted: true }),
        summaryFile('src/b.ts', 'added', { additions: 5, uncommitted: true }),
      ])
    })
    expect(result.current.files.map((f) => f.file_path)).toEqual(['src/a.ts', 'src/b.ts'])
    expect(result.current.files[0].additions).toBe(3)
    expect(result.current.files[0].deletions).toBe(1)
    expect(result.current.uncommittedCount).toBe(2)
  })

  it('shows committed-only files (no uncommitted flag) that the status projection lacks', async () => {
    // Working tree only knows about the dirty file; the summary adds the file
    // that was committed on the branch but is clean in the working tree.
    useGitStore.setState({ gitStatus: makeStatus([sf('src/wip.ts', 'modified')]) })
    mocks.getReviewFiles.mockResolvedValue([
      summaryFile('src/committed.ts', 'added', { additions: 10 }),
      summaryFile('src/wip.ts', 'modified', { additions: 2, uncommitted: true }),
    ])

    const { result } = renderHook(() => useSidebarChangedFiles('ws-committed'))

    await waitFor(() =>
      expect(result.current.files.map((f) => f.file_path)).toEqual([
        'src/committed.ts',
        'src/wip.ts',
      ]),
    )
    const committed = result.current.files.find((f) => f.file_path === 'src/committed.ts')!
    expect(committed.uncommitted).toBeFalsy()
    expect(committed.additions).toBe(10)
    // Only the working-tree file counts as uncommitted.
    expect(result.current.uncommittedCount).toBe(1)
  })

  it('fetches and mirrors the full diff when the review pane is already open', async () => {
    useGitStore.setState({ gitStatus: makeStatus([sf('src/status-only.ts', 'modified')]) })
    mocks.getReview.mockResolvedValue(makeReview([makeDiff('src/from-diff.ts', true)]))
    mocks.getReviewFiles.mockResolvedValue([summaryFile('src/status-only.ts', 'modified')])

    openReviewPane('ws-open')
    const { result } = renderHook(() => useSidebarChangedFiles('ws-open'))

    await waitFor(() => expect(mocks.getReview).toHaveBeenCalledWith('ws-open'))
    await waitFor(() =>
      expect(result.current.files.map((f) => f.file_path)).toEqual(['src/from-diff.ts']),
    )
    expect(result.current.uncommittedCount).toBe(1)
  })

  it('switches from the summary to the full diff when the pane opens', async () => {
    useGitStore.setState({ gitStatus: makeStatus([sf('src/status.ts', 'modified')]) })
    mocks.getReviewFiles.mockResolvedValue([summaryFile('src/status.ts', 'modified')])
    mocks.getReview.mockResolvedValue(
      makeReview([makeDiff('src/diff-a.ts'), makeDiff('src/diff-b.ts')]),
    )

    const store = getOrCreateWorkspaceStore('ws-switch')
    const { result } = renderHook(() => useSidebarChangedFiles('ws-switch'))

    // Closed: the summary source, and the full diff is not fetched.
    await waitFor(() =>
      expect(result.current.files.map((f) => f.file_path)).toEqual(['src/status.ts']),
    )
    expect(mocks.getReview).not.toHaveBeenCalled()

    // Open the pane → the gate enables and the full diff takes over.
    await act(async () => {
      store.setState({ buffers: [branchReviewBuffer('ws-switch')] })
    })

    await waitFor(() =>
      expect(result.current.files.map((f) => f.file_path)).toEqual([
        'src/diff-a.ts',
        'src/diff-b.ts',
      ]),
    )
    expect(mocks.getReview).toHaveBeenCalledWith('ws-switch')
  })

  it('mirrors an empty-but-loaded diff (fully merged branch) instead of falling back', async () => {
    useGitStore.setState({ gitStatus: makeStatus([sf('src/wt.ts', 'modified')]) })
    mocks.getReview.mockResolvedValue(makeReview([]))
    mocks.getReviewFiles.mockResolvedValue([
      summaryFile('src/wt.ts', 'modified', { uncommitted: true }),
    ])

    openReviewPane('ws-empty-diff')
    const { result } = renderHook(() => useSidebarChangedFiles('ws-empty-diff'))

    await waitFor(() => expect(mocks.getReview).toHaveBeenCalledWith('ws-empty-diff'))
    // Once the empty diff has LOADED, the sidebar mirrors the pane's
    // "no changes between branch and parent" state.
    await waitFor(() => expect(result.current.files).toEqual([]))
    expect(result.current.uncommittedCount).toBe(0)
  })

  it('never shows the stale pre-close diff on reopen: fallback until the fresh fetch lands, then upgrades', async () => {
    vi.useFakeTimers()
    try {
      useGitStore.setState({ gitStatus: makeStatus([sf('src/status.ts', 'modified')]) })
      // Keep the summary pending for the whole test so the fallback stays the
      // status projection — this test is about the full-diff reopen behavior.
      mocks.getReviewFiles.mockImplementation(() => new Promise(() => {}))
      // D1: the diff cached during the first open session.
      mocks.getReview.mockResolvedValueOnce(makeReview([makeDiff('src/d1.ts')]))

      const store = getOrCreateWorkspaceStore('ws-reopen')
      const { result } = renderHook(() => useSidebarChangedFiles('ws-reopen'))

      // Open → D1 loads and is mirrored.
      await act(async () => {
        store.setState({ buffers: [branchReviewBuffer('ws-reopen')] })
      })
      await act(async () => {
        await vi.advanceTimersByTimeAsync(0)
      })
      expect(result.current.files.map((f) => f.file_path)).toEqual(['src/d1.ts'])

      // Close → immediately back to the status projection (D1 stays cached, but
      // the sidebar must not show it).
      await act(async () => {
        store.setState({ buffers: [] })
      })
      expect(result.current.files.map((f) => f.file_path)).toEqual(['src/status.ts'])

      // The working tree changes while the pane is closed.
      act(() => {
        useGitStore.setState({
          gitStatus: makeStatus([sf('src/status.ts', 'modified'), sf('src/new.ts', 'added')]),
        })
      })
      expect(result.current.files.map((f) => f.file_path)).toEqual(['src/status.ts', 'src/new.ts'])

      // Reopen with the fresh fetch (D2) still in flight: show the CURRENT
      // projection — never the stale pre-close D1 list.
      let resolveSecond!: (v: ReviewState) => void
      mocks.getReview.mockImplementationOnce(() => new Promise((r) => (resolveSecond = r)))
      await act(async () => {
        store.setState({ buffers: [branchReviewBuffer('ws-reopen')] })
      })
      expect(mocks.getReview).toHaveBeenCalledTimes(2)
      expect(result.current.files.map((f) => f.file_path)).toEqual(['src/status.ts', 'src/new.ts'])

      // Fresh D2 resolves → the sidebar upgrades to the new diff.
      await act(async () => {
        resolveSecond(makeReview([makeDiff('src/d2-a.ts'), makeDiff('src/d2-b.ts')]))
      })
      expect(result.current.files.map((f) => f.file_path)).toEqual(['src/d2-a.ts', 'src/d2-b.ts'])
    } finally {
      vi.useRealTimers()
    }
  })
})
