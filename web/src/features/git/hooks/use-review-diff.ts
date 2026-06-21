import { useEffect, useState } from 'react'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { getReview } from '@/features/git/api/review-api'
import type { GitDiff } from '@/features/git/types/git-types'

export interface UseReviewDiffResult {
  files: GitDiff[]
  uncommittedCount: number
  loading: boolean
}

/**
 * Fetches the branch-review diff for the active workspace and caches it in
 * `branchReview.diffCache`. Re-fetches whenever `git-status-changed` fires.
 * Returns `{ files, uncommittedCount, loading }`.
 *
 * Guards against no active ws: returns empty state when wsId is empty/null.
 */
export function useReviewDiff(wsId: string | null): UseReviewDiffResult {
  const [files, setFiles] = useState<GitDiff[]>([])
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (!wsId) return

    let cancelled = false

    const fetch = async () => {
      setLoading(true)
      try {
        const review = await getReview(wsId)
        if (cancelled) return
        const store = getOrCreateWorkspaceStore(wsId)
        store.getState().setBranchReviewDiff(review.diff)
        setFiles(review.diff.files ?? [])
      } catch {
        // silently ignore: the sidebar should not crash on review fetch failure
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void fetch()

    const handler = () => {
      void fetch()
    }
    window.addEventListener('git-status-changed', handler)

    return () => {
      cancelled = true
      window.removeEventListener('git-status-changed', handler)
    }
  }, [wsId])

  const uncommittedCount = files.filter((f) => f.uncommitted).length

  return { files, uncommittedCount, loading }
}
