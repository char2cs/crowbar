import { useCallback, useState } from 'react'
import { ReviewDiffTab } from '@/features/git/components/review-diff-tab'

interface CommitDiffPaneProps {
  sha: string
  isActivePane?: boolean
}

/**
 * One commit's diff, on the same surface as the branch review.
 *
 * There is no separate renderer here — and that is the point. This pane used to
 * be a Monaco stack of its own: a second diff implementation with its own
 * search bar, its own header, its own hunk headers and its own comment layer,
 * fed by a payload carrying every line of the commit. Reading history now costs
 * exactly what reading a review costs, because it IS the review surface with a
 * different pair of trees behind it.
 */
export function CommitDiffPane({ sha, isActivePane }: CommitDiffPaneProps) {
  // The windowed reads are immutable for a commit, so there is nothing to
  // refetch on a timer; Refresh exists only for a failed first load, and
  // remounting the surface is the whole retry.
  const [attempt, setAttempt] = useState(0)
  const retry = useCallback(() => setAttempt((n) => n + 1), [])

  return (
    // The host box is load-bearing, not decoration. ReviewDiffTab is
    // `h-full min-h-0 flex-col` and its diff area is `min-h-0 flex-1`, so it
    // needs an ancestor with a DEFINITE height to divide up. Rendered bare into
    // the pane it got a content-driven one instead: the header and the binary
    // rows still measured, the flex-1 diff area resolved to zero, and the
    // surface reserved ten million pixels of scroll for content that was never
    // given a single one to draw in. BranchReviewPane supplies the same two
    // boxes for the same reason.
    <div className="flex h-full flex-col overflow-hidden bg-background">
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
        <ReviewDiffTab
          key={attempt}
          commit={sha}
          onRetry={retry}
          branchHeader={{ title: `Commit ${sha.substring(0, 7)}` }}
          emptyMessage="This commit changed no files."
          isActivePane={isActivePane}
        />
      </div>
    </div>
  )
}
