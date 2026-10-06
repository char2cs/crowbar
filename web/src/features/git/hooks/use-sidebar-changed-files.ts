import { useMemo, useRef } from 'react'
import deepEqual from 'fast-deep-equal'
import { useWorkspaceStoreById } from '@/features/workspace/stores/hooks/use-workspace-store-by-id'
import { gitStatusToChangedFiles } from '@/features/git/utils/git-status-to-changed-files'
import type { GitDiff } from '@/features/git/types/git-types'

export interface UseSidebarChangedFilesResult {
  files: GitDiff[]
  uncommittedCount: number
}

const EMPTY_FILES: GitDiff[] = []

/**
 * Data source for the always-mounted sidebar changed-files tree (P2b, Task 27).
 *
 * The sidebar shows the FULL branch picture (committed-vs-parent files AND
 * working-tree changes, with +N/-N counts) from three layers, cheapest first:
 *
 *   - The status projection (`gitStatusToChangedFiles`) is the instant
 *     first-paint fallback: it is already in the git store, so it renders with
 *     zero network latency, but it has no committed-only files and no counts.
 *   - The files-only summary (the workspace's `reviewFiles`) is the always-on
 *     source once it loads: it is O(file count) — no line content — so it is
 *     refetched on every git-status tick, and it restores the committed files and
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
  // Files-only summary — kept current by the workspace's git feed, the
  // full-picture source. Null until it has loaded.
  const summaryFiles = useWorkspaceStoreById(wsId ?? '', (s) => s.reviewFiles)

  // Cheap working-tree status — the instant first-paint fallback.
  const statusFiles = useWorkspaceStoreById(wsId ?? '', (s) => s.gitStatus?.files)

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

  // The summary is the single source once it has loaded; until then the
  // status projection paints. The full line-level diff used to take over here
  // while the review pane was open — it no longer exists to take over with,
  // and the summary already carries the same file set with the same ± counts.
  const files = summaryFiles ?? projected
  const uncommittedCount = summaryFiles ? countUncommitted(summaryFiles) : projected.length

  return { files: files ?? EMPTY_FILES, uncommittedCount }
}

function countUncommitted(files: GitDiff[]): number {
  return files.reduce((n, f) => n + (f.uncommitted ? 1 : 0), 0)
}
