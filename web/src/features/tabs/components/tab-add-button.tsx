// Lucide Plus (stroke-based) to match the sibling toolbar icons — back/forward/
// settings are Lucide; Phosphor's bold Plus rendered heavier and larger than them.
import { Plus } from 'lucide-react'
import React from 'react'
import { Button } from '@/components/ui/button'

interface TabAddButtonProps {
  isBottomPane: boolean
  onNewTab: () => void
}

/**
 * The tab strip's "+" — opens a New Tab in this pane, nothing else. It is
 * rendered as the LAST child inside the scrolling tab container (see
 * tab-bar.tsx), immediately after the last tab, so it flows and shifts
 * horizontally as tabs open and close — the same placement a browser's
 * new-tab button uses. It is a plain button, never wrapped in
 * `SortableEditorTab`/`useSortable`, so it never enters `sortedBufferIds`
 * and is never draggable.
 */
const TabAddButton = React.memo(function TabAddButton({
  isBottomPane,
  onNewTab,
}: TabAddButtonProps) {
  if (isBottomPane) return null

  return (
    <Button
      onClick={onNewTab}
      variant="ghost"
      size="icon-sm"
      className="shrink-0 rounded-sm text-muted-foreground hover:bg-sidebar-element-hover"
      tooltip="New Tab"
      tooltipSide="bottom"
      aria-label="New tab"
    >
      <Plus />
    </Button>
  )
})

export default TabAddButton
