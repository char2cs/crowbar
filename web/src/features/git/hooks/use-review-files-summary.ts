import { useEffect, useRef, useState } from 'react'
import deepEqual from 'fast-deep-equal'
import { getReviewFiles } from '@/features/git/api/review-api'
import { reviewFilesSummaryToChangedFiles } from '@/features/git/utils/review-file-summary-to-git-diff'
import type { GitDiff } from '@/features/git/types/git-types'

export interface UseReviewFilesSummaryResult {
  files: GitDiff[]
  /** True once a summary fetch has completed for the current workspace. Until
   *  then consumers keep their instant-first-paint fallback (the status
   *  projection). Reset to false when the workspace changes. */
  loaded: boolean
}

// Stable-empty rule: one module-level empty array kept referentially stable
// across renders and workspaces (see use-review-diff.ts).
const EMPTY_FILES: GitDiff[] = []

// The daemon can fire `git-status-changed` at ~2-3Hz while a terminal churns the
// working tree. Debounce (trailing) coalesces a burst into a single refetch —
// the same coalescing Task 6 established for the full diff, applied here to the
// cheap summary so it stays cheap even under a busy tree.
const GIT_STATUS_DEBOUNCE_MS = 250

/**
 * Fetches the files-only branch-review summary for a workspace on mount and on
 * every (debounced) `git-status-changed` tick. The payload is O(file count) and
 * carries no line content, so — unlike the full review diff — it is safe to pull
 * continuously; it is the always-on source for the sidebar's full changed-files
 * list (committed + working-tree, with +N/-N counts).
 *
 * A fast-deep-equal gate skips the state write when a refetch returns an
 * identical list, so an unchanged reload never churns the memoized tree. A
 * failed fetch is swallowed (the sidebar keeps its fallback rather than
 * crashing). Passing a null wsId disables all fetching and returns empty.
 */
export function useReviewFilesSummary(
  wsId: string | null,
  commit?: string,
): UseReviewFilesSummaryResult {
  const [files, setFiles] = useState<GitDiff[]>(EMPTY_FILES)
  const [loaded, setLoaded] = useState(false)
  const filesRef = useRef<GitDiff[]>(EMPTY_FILES)

  // Reset synchronously in the render where the SCOPE changes, so a stale
  // summary cannot paint for the new one before the effect re-runs (the same
  // derived-state reset use-review-diff uses). Keyed on the commit too:
  // switching between two commit tabs never changes wsId.
  const scopeKey = `${wsId ?? ''}\u0000${commit ?? ''}`
  const [prevScope, setPrevScope] = useState(scopeKey)
  if (prevScope !== scopeKey) {
    setPrevScope(scopeKey)
    setLoaded(false)
    filesRef.current = EMPTY_FILES
    setFiles(EMPTY_FILES)
  }

  useEffect(() => {
    if (!wsId) return

    let cancelled = false
    let debounceTimer: ReturnType<typeof setTimeout> | null = null

    const fetchSummary = async () => {
      try {
        const mapped = reviewFilesSummaryToChangedFiles(await getReviewFiles({ wsId, commit }))
        if (cancelled) return
        if (!deepEqual(mapped, filesRef.current)) {
          filesRef.current = mapped
          setFiles(mapped)
        }
        setLoaded(true)
      } catch {
        // The sidebar must not crash on a summary fetch failure; the caller
        // keeps its status-projection fallback until a later tick succeeds.
      }
    }

    void fetchSummary()

    // A commit-scoped summary describes two immutable trees; the working tree
    // cannot change it, so it takes no status ticks.
    if (commit)
      return () => {
        cancelled = true
      }

    const handler = () => {
      if (debounceTimer) clearTimeout(debounceTimer)
      debounceTimer = setTimeout(() => {
        debounceTimer = null
        void fetchSummary()
      }, GIT_STATUS_DEBOUNCE_MS)
    }
    window.addEventListener('git-status-changed', handler)

    return () => {
      cancelled = true
      if (debounceTimer) clearTimeout(debounceTimer)
      window.removeEventListener('git-status-changed', handler)
    }
  }, [wsId, commit])

  return { files, loaded }
}
