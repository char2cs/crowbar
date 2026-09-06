import { useEffect, useState, useSyncExternalStore } from 'react'
import { getReviewOutline, type FileOutline } from '@/features/git/api/review-window-api'
import { getOwningChatId, subscribeToWorkspaceScope } from '@/lib/workspace-scope'

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
 *
 * `commit` scopes the outline to one commit instead of the branch. A
 * commit-scoped outline describes two immutable trees, so it also drops the
 * `git-status-changed` refetch: nothing the working tree does can change it,
 * and refetching on every keystroke in a terminal would be pure waste.
 */
export function useReviewOutline(wsId: string | null, commit?: string): UseReviewOutlineResult {
  const [outline, setOutline] = useState<FileOutline[]>(EMPTY_OUTLINE)
  const [loaded, setLoaded] = useState(false)

  // Reset synchronously in the render where the SCOPE changes, so a stale
  // outline cannot describe the wrong diff for even one frame. Keyed on the
  // commit too: switching between two commit tabs never changes wsId.
  const scopeKey = `${wsId ?? ''}\u0000${commit ?? ''}`
  const [prevScope, setPrevScope] = useState(scopeKey)
  if (prevScope !== scopeKey) {
    setPrevScope(scopeKey)
    setOutline(EMPTY_OUTLINE)
    setLoaded(false)
  }

  // reviewBaseForWorkspace(wsId) — which getReviewOutline resolves through —
  // throws without a recorded owning chat id. The sidebar's chat-list fetch
  // that records one races WorkspaceView's own (often faster) hydration, so on
  // a workspace that just activated this can still be null; firing anyway used
  // to hit the throw, land in the catch below, and leave the outline empty
  // until an unrelated git-status-changed tick happened to retry it.
  // Subscribing makes the id a piece of React state so the effect re-runs the
  // moment the sidebar records one — same fix as useWorkspaceEffects'
  // useOwningChatId.
  const owningChatId = useSyncExternalStore(
    (onChange) => (wsId ? subscribeToWorkspaceScope(wsId, onChange) : () => {}),
    () => (wsId ? getOwningChatId(wsId) : null),
  )

  useEffect(() => {
    if (!wsId) return
    if (owningChatId === null) return

    let cancelled = false
    let debounceTimer: ReturnType<typeof setTimeout> | null = null

    const fetchOutline = async () => {
      try {
        const next = await getReviewOutline({ wsId, commit })
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

    if (commit)
      return () => {
        cancelled = true
      }

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
  }, [wsId, commit, owningChatId])

  return { outline, loaded }
}
