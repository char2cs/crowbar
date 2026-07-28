import { renderHook, act, waitFor, cleanup } from '@testing-library/react'
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import type { GitFile, GitStatus } from '@/features/git/types/git-types'
import type { ReviewState, ReviewFileSummary } from '@/features/git/api/review-api'
import { useGitStore } from '@/features/git/stores/git-store'
import {
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

describe('useSidebarChangedFiles', () => {
  // The sidebar used to switch from the summary to the full line-level branch
  // diff whenever the review pane was open. That source is gone: the pane reads
  // /review/files + /review/outline and fetches patches per file, and the
  // summary already carries the same file set with the same ± counts. The four
  // tests covering that takeover were removed with it.
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

})
