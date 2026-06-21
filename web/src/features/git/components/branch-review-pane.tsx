import { useCallback, useEffect } from 'react'
import { GitPullRequest } from '@phosphor-icons/react'
import {
  useWorkspaceStore,
} from '@/features/workspace/stores/workspace-context'
import { getReview } from '../api/review-api'
import { ReviewDiffTab } from './review-diff-tab'

interface BranchReviewPaneProps {
  wsId: string
}

export function BranchReviewPane({ wsId }: BranchReviewPaneProps) {
  const store = useWorkspaceStore()

  // Load the composite review read model + branch diff on mount. The backend
  // folds description, merge strategy, conversations, and diff into /review.
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
      a.setBranchReviewDiff(review.diff)
    } catch {
      store.getState().setBranchReviewDiffStatus('error')
    }
  }, [store, wsId])

  useEffect(() => {
    void load()
  }, [load])

  return (
    <div className="flex h-full flex-col overflow-hidden bg-background">
      <div className="flex shrink-0 items-center gap-2 border-border border-b px-3 py-2">
        <GitPullRequest className="size-4 text-muted-foreground" />
        <span className="ui-text-sm font-medium text-foreground">Branch Review</span>
      </div>

      <div className="flex flex-1 flex-col overflow-hidden">
        <ReviewDiffTab onRetry={() => void load()} />
      </div>
    </div>
  )
}
