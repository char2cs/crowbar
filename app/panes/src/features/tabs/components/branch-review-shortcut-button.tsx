import { GitPullRequest } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'

interface BranchReviewShortcutButtonProps {
  isBottomPane: boolean
  onOpen: () => void
}

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
