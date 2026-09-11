import { GitPullRequest } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'

interface BranchReviewShortcutButtonProps {
  isBottomPane: boolean
  onOpen: () => void
}

/**
 * A shortcut into the SAME "Review this branch" action GitPanel's own button
 * triggers (git-panel.tsx, `openBranchReviewForActiveWorkspace`) — branch
 * review's real home stays the git file-explorer card; this is just a
 * faster way to reach it from the IDE sector's own tab row, pinned at the
 * right edge next to CloseViewButton. Same toolbar-button recipe as its row
 * neighbours (icon-sm, rounded-sm, the sidebar hover token).
 */
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
