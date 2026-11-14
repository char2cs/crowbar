import { useEffect, useState, useSyncExternalStore } from 'react'
import { getReviewFiles } from '@/features/git/api/review-api'
import { reviewFilesSummaryToChangedFiles } from '@/features/git/utils/review-file-summary-to-git-diff'
import { useHostResolved } from '@/lib/host-resolve'
import { report } from '@/lib/errors/report'
import { getOwningChatId, subscribeToWorkspaceScope } from '@/lib/workspace-scope'
import type { GitDiff } from '@/features/git/types/git-types'
import { useWorkspaceStoreById } from '@/features/workspace/stores/hooks/use-workspace-store-by-id'

export interface UseReviewFilesSummaryResult {
  files: GitDiff[]
  /** True once a summary fetch has completed for the current workspace. Until
   *  then consumers keep their instant-first-paint fallback (the status
   *  projection). Reset to false when the workspace changes. */
  loaded: boolean
}

// One module-level empty array, referentially stable across renders and workspaces.
const EMPTY_FILES: GitDiff[] = []

/** The files summary. The branch scope reads the store's `reviewFiles` (one fetch per git tick,
 *  shared with the sidebar); a commit scope is immutable, so fetched once. A null wsId fetches
 *  nothing and returns empty. */
export function useReviewFilesSummary(
  wsId: string | null,
  commit?: string,
): UseReviewFilesSummaryResult {
  const storeFiles = useWorkspaceStoreById(wsId ?? '', (s) => s.reviewFiles)
  const commitSummary = useCommitFilesSummary(commit ? wsId : null, commit)
  if (commit) return commitSummary
  return { files: storeFiles ?? EMPTY_FILES, loaded: storeFiles !== null }
}

function useCommitFilesSummary(
  wsId: string | null,
  commit: string | undefined,
): UseReviewFilesSummaryResult {
  const [files, setFiles] = useState<GitDiff[]>(EMPTY_FILES)
  const [loaded, setLoaded] = useState(false)

  // Reset in the render where the scope changes so a stale summary never paints
  // for the new one; keyed on the commit because two commit tabs share a wsId.
  const scopeKey = `${wsId ?? ''}\u0000${commit ?? ''}`
  const [prevScope, setPrevScope] = useState(scopeKey)
  if (prevScope !== scopeKey) {
    setPrevScope(scopeKey)
    setLoaded(false)
    setFiles(EMPTY_FILES)
  }

  // getReviewFiles resolves through a chat-scoped route that throws without a
  // recorded owning chat id, which the sidebar records after hydration; subscribe
  // so the effect re-runs the moment it arrives.
  const owningChatId = useSyncExternalStore(
    (onChange) => (wsId ? subscribeToWorkspaceScope(wsId, onChange) : () => {}),
    () => (wsId ? getOwningChatId(wsId) : null),
  )

  // The host is named by the project list, which the sidebar's cache can outrun; a
  // fetch before it would fail and never be retried. '' reads as unresolved.
  const hostResolved = useHostResolved({ wsId: wsId ?? '' })

  useEffect(() => {
    if (!wsId || owningChatId === null || !hostResolved) return
    let cancelled = false
    getReviewFiles({ wsId, commit })
      .then((summary) => {
        if (cancelled) return
        setFiles(reviewFilesSummaryToChangedFiles(summary))
        setLoaded(true)
      })
      .catch((err: unknown) => report(err, { action: 'Load the commit’s files' }))
    return () => {
      cancelled = true
    }
  }, [wsId, commit, owningChatId, hostResolved])

  return { files, loaded }
}
