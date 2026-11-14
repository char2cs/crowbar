import { GitPullRequest } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'

interface BranchReviewShortcutButtonProps {
  isBottomPane: boolean
  onOpen: () => void
}

/** A shortcut on the IDE sector's tab row to GitPanel's own "Review this branch" action
 *  (git-panel.tsx), which stays the feature's home. */
export function BranchReviewShortcutButton({
  isBottomPane,
  onOpen,
}: BranchReviewShortcutButtonProps) {
  if (isBottomPane) return null

  return (
    <Button
      variant="ghost"
      size="icon-sm"
      data-testid="branch-review-shortcut"
      aria-label="Review this branch"
      onClick={onOpen}
      tooltip="Review this branch"
      tooltipSide="bottom"
      className="shrink-0 rounded-sm text-muted-foreground hover:bg-sidebar-element-hover"
    >
      <GitPullRequest />
    </Button>
  )
}
