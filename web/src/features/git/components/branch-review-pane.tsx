import { useCallback, useEffect } from 'react'
import { useShallow } from 'zustand/react/shallow'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import { useSidebarStore } from '@/lib/store/sidebar'
import { getReview } from '../api/review-api'
import { ReviewDiffTab } from './review-diff-tab'

interface BranchReviewPaneProps {
  wsId: string
  isActivePane?: boolean
}

export function BranchReviewPane({ wsId, isActivePane }: BranchReviewPaneProps) {
  const store = useWorkspaceStore()

  // Branch + base for the shared diff header: title = branch name, meta = → base.
  // Sourced from the sidebar workspace record (same data the merge section uses).
  const branchHeader = useSidebarStore(
    useShallow((s): { title: string; baseBranch?: string } => {
      for (const repo of s.repos) {
        const ws = repo.workspaces.find((w) => w.id === wsId)
        if (ws) return { title: ws.branch || 'Branch Review', baseBranch: ws.parentBranch }
      }
      return { title: 'Branch Review' }
    }),
  )

  // Load the composite review read model on mount: description, merge strategy
  // and conversations. The DIFF no longer comes from here — the surface reads
  // /review/files + /review/outline and fetches patches per file.
  // Threads are intentionally NOT sourced here — they are seeded and kept live
  // by useWorkspaceThreadsStream (mounted in useWorkspaceEffects) so optimistic
  // writes and WS pushes are not clobbered on every pane remount.
  const load = useCallback(async () => {
    const actions = store.getState()
    actions.setBranchReviewDiffStatus('loading')
    try {
      const review = await getReview(wsId)
      const a = store.getState()
      a.setBranchReviewDescription(review.description)
      a.setBranchReviewMergeStrategy(review.mergeStrategy)
      a.setBranchReviewConversations(review.conversations)
      // Deliberately NOT storing review.diff. The pane renders from the
      // files summary + outline now; keeping the composite's line-level diff
      // would re-import the very payload this phase removed.
    } catch {
      store.getState().setBranchReviewDiffStatus('error')
    }
  }, [store, wsId])

  useEffect(() => {
    void load()
  }, [load])

  return (
    <div className="flex h-full flex-col overflow-hidden bg-background">
      <div className="flex flex-1 flex-col overflow-hidden">
        <ReviewDiffTab
          onRetry={() => void load()}
          branchHeader={branchHeader}
          isActivePane={isActivePane}
        />
      </div>
    </div>
  )
}
