import { useMemo, useRef } from 'react'
import { createStore, useStore } from 'zustand'
import deepEqual from 'fast-deep-equal'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { useGitStore } from '@/features/git/stores/git-store'
import { gitStatusToChangedFiles } from '@/features/git/utils/git-status-to-changed-files'
import { useReviewDiff } from '@/features/git/hooks/use-review-diff'
import { useReviewFilesSummary } from '@/features/git/hooks/use-review-files-summary'
import type { GitDiff } from '@/features/git/types/git-types'
import type { WorkspaceState } from '@/features/workspace/stores/workspace-store.types'

export interface UseSidebarChangedFilesResult {
  files: GitDiff[]
  uncommittedCount: number
}

const EMPTY_FILES: GitDiff[] = []

// Inert, never-shared store so the buffers subscription below can run
// unconditionally (rules of hooks) when there is no active workspace. Only
// `.buffers` is ever read off it. Mirrors NULL_REVIEW_STORE in use-review-diff.
const NULL_WS_STORE = createStore<WorkspaceState>(
  () => ({ buffers: [] }) as unknown as WorkspaceState,
)

/**
 * Data source for the always-mounted sidebar changed-files tree (P2b, Task 27).
 *
 * The sidebar shows the FULL branch picture (committed-vs-parent files AND
 * working-tree changes, with +N/-N counts) from three layers, cheapest first:
 *
 *   - The status projection (`gitStatusToChangedFiles`) is the instant
 *     first-paint fallback: it is already in the git store, so it renders with
 *     zero network latency, but it has no committed-only files and no counts.
 *   - The files-only summary (`useReviewFilesSummary`) is the always-on source
 *     once it loads: it is O(file count) — no line content — so it can be
 *     fetched on every git-status tick, and it restores the committed files and
 *     +N/-N badges the projection lacks WITHOUT resurrecting the killed
 *     line-level refetch loop.
 *   - The full line-level diff (`useReviewDiff`) is fetched ONLY while the
 *     Branch Review pane is open (unchanged from Task 7), and takes over the
 *     sidebar then so it mirrors exactly what the pane renders.
 *
 * "Open" is a cheap, store-derivable predicate: a `branchReview` buffer exists
 * in the workspace store (openContent dedupes these to at most one per ws).
 */
export function useSidebarChangedFiles(wsId: string | null): UseSidebarChangedFilesResult {
  const reviewStore = wsId ? getOrCreateWorkspaceStore(wsId) : NULL_WS_STORE
  const reviewOpen = useStore(reviewStore, (s) => s.buffers.some((b) => b.type === 'branchReview'))

  // Full diff — fetched ONLY while the review pane is open.
  const { files: diffFiles, stale } = useReviewDiff(wsId, { enabled: reviewOpen })

  // Files-only summary — always fetched (cheap), the full-picture source.
  const { files: summaryFiles, loaded: summaryLoaded } = useReviewFilesSummary(wsId)

  // Cheap working-tree status — the instant first-paint fallback.
  const statusFiles = useGitStore((s) => s.gitStatus?.files)

  // Recompute the projection only when the status files reference changes (a
  // git reload), then hold the previous array when the projected content is
  // unchanged so an identical reload does not churn the memoized tree — mirrors
  // the diff path's Task-6 reference stability.
  const rawProjected = useMemo(() => gitStatusToChangedFiles(statusFiles), [statusFiles])
  const projectedRef = useRef<GitDiff[]>(rawProjected)
  if (rawProjected !== projectedRef.current && !deepEqual(rawProjected, projectedRef.current)) {
    projectedRef.current = rawProjected
  }
  const projected = projectedRef.current

  // While the pane is closed (or open but the diff is not yet fresh), the
  // summary is the source once it has loaded; until then the projection paints.
  const fallback = summaryLoaded ? summaryFiles : projected

  // Trust the full diff only when the pane is open AND a fetch has completed for
  // THIS open session (`stale` from useReviewDiff). diffCache survives a
  // close, so on reopen after working-tree changes the cached list is
  // outdated — the summary/projection wins until the fresh fetch lands, then the
  // list upgrades. Freshness (not `loading`) is the gate, so the debounced
  // refetches while open never flicker the source; and an empty-but-fresh diff
  // (fully merged branch) still mirrors the pane's "no changes" state.
  const useDiff = reviewOpen && !stale
  const files = useDiff ? diffFiles : fallback

  const uncommittedCount = useDiff
    ? countUncommitted(diffFiles)
    : summaryLoaded
      ? countUncommitted(summaryFiles)
      : projected.length

  return { files: files ?? EMPTY_FILES, uncommittedCount }
}

function countUncommitted(files: GitDiff[]): number {
  return files.reduce((n, f) => n + (f.uncommitted ? 1 : 0), 0)
}
