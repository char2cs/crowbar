import { useEffect, useState } from 'react'
import { createStore, useStore, type StoreApi } from 'zustand'
import { useShallow } from 'zustand/react/shallow'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { getReview } from '@/features/git/api/review-api'
import type { GitDiff } from '@/features/git/types/git-types'
import type { WorkspaceState } from '@/features/workspace/stores/workspace-store.types'

export interface UseReviewDiffResult {
  files: GitDiff[]
  uncommittedCount: number
  loading: boolean
  /**
   * True until a `getReview` fetch has completed SUCCESSFULLY for the current
   * enabled session (i.e. since the gate last flipped on for this workspace).
   * `branchReview.diffCache` deliberately survives a disable — the review pane
   * owns that state and clearing it from here would be a side effect on shared
   * state — so on re-enable the previously cached files are exposed again
   * immediately while being re-validated. `stale` lets consumers (the sidebar)
   * refuse to render them until the fresh fetch lands. Once fresh, subsequent
   * debounced refetches do NOT flip it back (stale-while-revalidate within an
   * open session), so the sidebar never flickers on git ticks.
   */
  stale: boolean
}

// Stable-empty rule: a fresh `[]` literal returned from the selector on every
// render would be a new reference each time, defeating useShallow/useStore's
// identity check and forcing a re-render every render pass (the infinite-loop
// footgun). Reusing one module-level empty array keeps it referentially
// stable across renders and across workspaces.
const EMPTY_FILES: GitDiff[] = []

// The daemon can fire `git-status-changed` at ~2-3Hz while a terminal is busy
// touching the working tree. Debouncing (trailing) coalesces a whole burst
// into a single refetch instead of re-fetching the entire branch diff — which
// can be tens of thousands of lines — on every tick.
const GIT_STATUS_DEBOUNCE_MS = 250

// A detached, never-shared store used only when there is no active workspace
// (wsId is null/empty), so the `useStore` call below can always be invoked
// unconditionally (rules of hooks) while still resolving to EMPTY_FILES. It
// never touches the shared workspace-store registry (no persistence, no
// registry-map entry) and is typed loosely — only `.branchReview.diffCache`
// is ever read off it.
const NULL_REVIEW_STORE = createStore<WorkspaceState>(
  () => ({ branchReview: { diffCache: null } }) as unknown as WorkspaceState,
) as StoreApi<WorkspaceState>

export interface UseReviewDiffOptions {
  /**
   * Gate the expensive `getReview` fetch. When `false` (or wsId is null) the
   * hook makes ZERO network calls, registers ZERO `git-status-changed`
   * listeners, and returns a stable empty result. The always-mounted sidebar
   * passes `enabled: <branch review pane is open>` so the full line-level diff
   * is only pulled while it can actually be seen (P2b). Defaults to `true` so
   * existing callers keep their behavior.
   */
  enabled?: boolean
}

/**
 * Fetches the branch-review diff for the active workspace and caches it in
 * `branchReview.diffCache`. Re-fetches whenever `git-status-changed` fires,
 * debounced 250ms trailing so a burst of events coalesces into one refetch.
 *
 * `files` is derived reactively straight from the store's `diffCache` (not
 * duplicated into local `useState`), so an identical payload — which the
 * store's equality gate (see `setBranchReviewDiff`) skips writing — never
 * forces a re-render here, and downstream memoized consumers see a stable
 * reference.
 *
 * Guards against no active ws / a disabled gate: returns empty state and does
 * no fetching when wsId is empty/null or `enabled` is false.
 */
export function useReviewDiff(
  wsId: string | null,
  { enabled = true }: UseReviewDiffOptions = {},
): UseReviewDiffResult {
  const [loading, setLoading] = useState(false)

  // When disabled (or no ws) subscribe to the inert store so `files` resolves
  // to the stable EMPTY_FILES and no live diffCache is leaked from a prior
  // open — the hook returns a clean empty state.
  const active = enabled && !!wsId
  const activeKey = active ? wsId : null

  // Freshness tracking: a new enabled session (the gate flipped on, or the
  // workspace changed while enabled) must not trust whatever diffCache holds
  // until a fetch completes for THIS session. The reset happens via React's
  // render-phase "derived state" pattern — synchronously, during the very
  // render in which the session key changes — so not even one frame of the
  // stale pre-close diff can paint on reopen.
  const [fresh, setFresh] = useState(false)
  const [prevKey, setPrevKey] = useState<string | null>(activeKey)
  if (prevKey !== activeKey) {
    setPrevKey(activeKey)
    setFresh(false)
  }

  const activeStore: StoreApi<WorkspaceState> =
    active && wsId ? getOrCreateWorkspaceStore(wsId) : NULL_REVIEW_STORE
  const files = useStore(
    activeStore,
    useShallow((s) => s.branchReview.diffCache?.files ?? EMPTY_FILES),
  )

  useEffect(() => {
    if (!enabled || !wsId) return

    let cancelled = false
    let debounceTimer: ReturnType<typeof setTimeout> | null = null

    const fetchDiff = async () => {
      setLoading(true)
      try {
        const review = await getReview(wsId)
        if (cancelled) return
        getOrCreateWorkspaceStore(wsId).getState().setBranchReviewDiff(review.diff)
        // Only a SUCCESSFUL fetch validates this session's data; on error the
        // session stays stale so consumers keep their fallback rather than
        // trusting a cache the failed refetch could not confirm.
        setFresh(true)
      } catch {
        // silently ignore: the sidebar should not crash on review fetch failure
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void fetchDiff()

    const handler = () => {
      if (debounceTimer) clearTimeout(debounceTimer)
      debounceTimer = setTimeout(() => {
        debounceTimer = null
        void fetchDiff()
      }, GIT_STATUS_DEBOUNCE_MS)
    }
    window.addEventListener('git-status-changed', handler)

    return () => {
      cancelled = true
      if (debounceTimer) clearTimeout(debounceTimer)
      window.removeEventListener('git-status-changed', handler)
    }
  }, [wsId, enabled])

  const uncommittedCount = files.filter((f) => f.uncommitted).length

  return { files, uncommittedCount, loading, stale: !fresh }
}
