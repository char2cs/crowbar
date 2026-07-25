// Lucide X (stroke-based), sized by the button's own size variant — same as
// TabAddButton's Plus. Phosphor's X plus an explicit `size={12}` rendered
// heavier than the "+" at the other end of the same tab bar (and the explicit
// size was dead anyway: the variant's `[&_svg]:size-*` class beats the
// attribute).
import { X } from 'lucide-react'
import React from 'react'
import { Button } from '@/components/ui/button'

interface CloseSplitButtonProps {
  isBottomPane: boolean
  disablePaneActions: boolean
  isInSplit: boolean
  onClosePane: () => void
}

/**
 * Closes this split pane. A PANE action, not a tab action — unlike
 * TabAddButton it stays pinned at the right edge of the tab bar, outside the
 * scrolling tab container, so it never moves as tabs open/close. Its chrome
 * (variant, size, classes, icon family) is otherwise IDENTICAL to
 * TabAddButton's: both sit in the same bar and must read as the same control.
 */
const CloseSplitButton = React.memo(function CloseSplitButton({
  isBottomPane,
  disablePaneActions,
  isInSplit,
  onClosePane,
}: CloseSplitButtonProps) {
  if (isBottomPane) return null
  if (disablePaneActions || !isInSplit) return null

  return (
    <Button
      onClick={onClosePane}
      variant="ghost"
      size="icon-sm"
      className="shrink-0 rounded-sm text-muted-foreground hover:bg-sidebar-element-hover"
      tooltip="Close Split"
      tooltipSide="bottom"
      aria-label="Close split pane"
    >
      <X />
    </Button>
  )
})

export default CloseSplitButton
