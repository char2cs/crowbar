// Lucide X (stroke-based), sized by the button's own size variant — same as
// TabAddButton's Plus. Phosphor's X plus an explicit `size={12}` rendered
// heavier than the "+" at the other end of the same tab bar (and the explicit
// size was dead anyway: the variant's `[&_svg]:size-*` class beats the
// attribute).
import { X } from 'lucide-react'
import React from 'react'
import { Button } from '@/components/ui/button'

interface CloseViewButtonProps {
  isBottomPane: boolean
  disablePaneActions: boolean
  /** §5.4: every view has a close control except a working one — the same rule
   *  Recents' own × applies (`recents-band.tsx`'s `canClose`), read here for
   *  whichever chat(s) this pane's VIEW holds rather than duplicated. */
  canClose: boolean
  onCloseView: () => void
}

/**
 * Ends the whole VIEW this pane belongs to — every pane in it, through
 * `closeView` (spec §5.4, "what Recents' × means for a view of any size").
 *
 * Used to close only THIS pane, surviving a multi-pane view with the rest
 * still up: closing one half of a split left the other's own control gone
 * the moment it became the split's sole survivor (`isInSplit` gated on there
 * being 2+ panes), stranding the user with no way to finish closing it from
 * the pane chrome at all — Recents' × was the only surface left, a different
 * control in a different place. A view is the close UNIT everywhere else
 * (Recents, the model spec); this is the one place that still meant "pane".
 *
 * Stays pinned at the right edge of the tab bar, outside the scrolling tab
 * container, so it never moves as tabs open/close. Its chrome (variant, size,
 * classes, icon family) is otherwise IDENTICAL to TabAddButton's: both sit in
 * the same bar and must read as the same control.
 */
const CloseViewButton = React.memo(function CloseViewButton({
  isBottomPane,
  disablePaneActions,
  canClose,
  onCloseView,
}: CloseViewButtonProps) {
  if (isBottomPane) return null
  if (disablePaneActions || !canClose) return null

  return (
    <Button
      onClick={onCloseView}
      variant="ghost"
      size="icon-sm"
      className="shrink-0 rounded-sm text-muted-foreground hover:bg-sidebar-element-hover"
      tooltip="Close"
      tooltipSide="bottom"
      aria-label="Close view"
    >
      <X />
    </Button>
  )
})

export default CloseViewButton
