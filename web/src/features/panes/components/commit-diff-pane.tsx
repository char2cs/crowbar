import { useCallback, useState } from 'react'
import { ReviewDiffTab } from '@/features/git/components/review-diff-tab'

interface CommitDiffPaneProps {
  sha: string
  isActivePane?: boolean
  /** This commit's workspace (CommitDiffContent.wsId), never the ambient WorkspaceStoreContext. */
  wsId: string
}

// One commit's diff: the branch-review surface itself with a different pair of trees behind it,
// so reading history costs what reading a review does.
export function CommitDiffPane({ sha, isActivePane, wsId }: CommitDiffPaneProps) {
  // A commit's windowed reads are immutable, so nothing refetches; Refresh only retries a
  // failed first load, by remounting the surface.
  const [attempt, setAttempt] = useState(0)
  const retry = useCallback(() => setAttempt((n) => n + 1), [])

  return (
    // ReviewDiffTab's flex-1 diff area needs an ancestor of definite height; without these two
    // boxes it resolves to zero height under a huge scroll reservation (BranchReviewPane too).
    <div className="flex h-full flex-col overflow-hidden bg-background">
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
        <ReviewDiffTab
          key={attempt}
          wsId={wsId}
          commit={sha}
          onRetry={retry}
          title={`Commit ${sha.substring(0, 7)}`}
          emptyMessage="This commit changed no files."
          isActivePane={isActivePane}
        />
      </div>
    </div>
  )
}
