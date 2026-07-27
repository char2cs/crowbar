import { useEffect, useState } from 'react'
import { getReviewOutline, type FileOutline } from '@/features/git/api/review-window-api'

export interface UseReviewOutlineResult {
  outline: FileOutline[]
  /** True once an outline fetch has completed for the current workspace. */
  loaded: boolean
}

// Stable-empty rule: one module-level empty array kept referentially stable
// across renders and workspaces, so a selector returning it never looks like a
// new value (see use-review-files-summary.ts).
const EMPTY_OUTLINE: FileOutline[] = []

// The daemon can fire `git-status-changed` at ~2-3Hz while a terminal churns
// the tree. Coalesce a burst into one refetch, matching the summary hook.
const GIT_STATUS_DEBOUNCE_MS = 250

/**
 * Fetches the branch-review outline: per-file hunk GEOMETRY, no line content.
 *
 * This is what lets the review surface reserve correct scroll space for every
 * changed file before fetching a single patch. It is O(hunks) — 2.28MB raw but
 * 27KB gzipped on a 1M-line branch — where the old composite it replaces was
 * 158MB of per-line JSON.
 *
 * Deliberately NOT part of first paint: the file list renders from the summary,
 * and heights sharpen when this lands. Passing a null wsId disables fetching.
 */
export function useReviewOutline(wsId: string | null): UseReviewOutlineResult {
  const [outline, setOutline] = useState<FileOutline[]>(EMPTY_OUTLINE)
  const [loaded, setLoaded] = useState(false)

  // Reset synchronously in the render where the workspace changes, so a stale
  // outline cannot describe the wrong branch for even one frame.
  const [prevWs, setPrevWs] = useState<string | null>(wsId)
  if (prevWs !== wsId) {
    setPrevWs(wsId)
    setOutline(EMPTY_OUTLINE)
    setLoaded(false)
  }

  useEffect(() => {
    if (!wsId) return

    let cancelled = false
    let debounceTimer: ReturnType<typeof setTimeout> | null = null

    const fetchOutline = async () => {
      try {
        const next = await getReviewOutline(wsId)
        if (cancelled) return
        setOutline(next)
        setLoaded(true)
      } catch {
        // The pane must not blank on an outline failure: the summary alone
        // still renders every file, just with estimated rather than exact
        // heights. A later tick can succeed.
      }
    }

    void fetchOutline()

    const handler = () => {
      if (debounceTimer) clearTimeout(debounceTimer)
      debounceTimer = setTimeout(() => {
        debounceTimer = null
        void fetchOutline()
      }, GIT_STATUS_DEBOUNCE_MS)
    }
    window.addEventListener('git-status-changed', handler)

    return () => {
      cancelled = true
      if (debounceTimer) clearTimeout(debounceTimer)
      window.removeEventListener('git-status-changed', handler)
    }
  }, [wsId])

  return { outline, loaded }
}
