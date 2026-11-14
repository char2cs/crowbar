import { useEffect, useState, useSyncExternalStore } from 'react'
import { getReviewOutline, type FileOutline } from '@/features/git/api/review-window-api'
import { useHostResolved } from '@/lib/host-resolve'
import { getOwningChatId, subscribeToWorkspaceScope } from '@/lib/workspace-scope'
import { onGitStatusChanged } from '@/features/git/stores/git-refresh'

export interface UseReviewOutlineResult {
  outline: FileOutline[]
  /** True once an outline fetch has completed for the current workspace. */
  loaded: boolean
}

// One module-level empty array, so an empty outline never looks like a new value.
const EMPTY_OUTLINE: FileOutline[] = []

/** The outline (hunk geometry, O(hunks)) that sizes every file before a patch is fetched. Not
 *  first paint: the summary renders and heights sharpen when it lands. A `commit` outline is of
 *  immutable trees, so it skips status refetches; a null wsId fetches nothing. */
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

  // getReviewOutline throws until the owning chat id is recorded, which can trail activation;
  // subscribing re-runs the effect when it lands instead of failing silently until a git tick.
  const owningChatId = useSyncExternalStore(
    (onChange) => (wsId ? subscribeToWorkspaceScope(wsId, onChange) : () => {}),
    () => (wsId ? getOwningChatId(wsId) : null),
  )

  // The host is named by the project list, which the sidebar's cache can outrun; a
  // fetch before it is swallowed below and never retried. '' reads as unresolved.
  const hostResolved = useHostResolved({ wsId: wsId ?? '' })

  useEffect(() => {
    if (!wsId) return
    if (owningChatId === null || !hostResolved) return

    let cancelled = false

    const fetchOutline = async () => {
      try {
        const next = await getReviewOutline({ wsId, commit })
        if (cancelled) return
        setOutline(next)
        setLoaded(true)
      } catch {
        return // polled: the summary still renders every file at estimated heights
      }
    }

    void fetchOutline()

    if (commit)
      return () => {
        cancelled = true
      }

    // Status changes arrive already coalesced by the workspace's git effect.
    const stop = onGitStatusChanged(wsId, () => void fetchOutline())

    return () => {
      cancelled = true
      stop()
    }
  }, [wsId, commit, owningChatId, hostResolved])

  return { outline, loaded }
}
