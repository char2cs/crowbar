import { useEffect, useState, useSyncExternalStore } from 'react'
import { getReviewFiles } from '@/features/git/api/review-api'
import { reviewFilesSummaryToChangedFiles } from '@/features/git/utils/review-file-summary-to-git-diff'
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

// Stable-empty rule: one module-level empty array kept referentially stable
// across renders and workspaces (see use-review-diff.ts).
const EMPTY_FILES: GitDiff[] = []

/**
 * The files-only branch-review summary. The workspace's own scope reads the
 * workspace store's `reviewFiles`, which the git feed keeps current — the one
 * fetch per git tick, shared with the sidebar tree. A commit scope describes two
 * immutable trees, so it is fetched once on its own and takes no ticks.
 * A null wsId disables all fetching and returns empty.
 */
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

  useEffect(() => {
    if (!wsId || owningChatId === null) return
    let cancelled = false
    getReviewFiles({ wsId, commit })
      .then((summary) => {
        if (cancelled) return
        setFiles(reviewFilesSummaryToChangedFiles(summary))
        setLoaded(true)
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [wsId, commit, owningChatId])

  return { files, loaded }
}
