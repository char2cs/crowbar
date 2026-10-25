import { PlusIcon } from '@phosphor-icons/react'
import React from 'react'
import { Button } from '@/components/ui/button'

interface TabAddButtonProps {
  isBottomPane: boolean
  onNewTab: () => void
}

/**
 * The tab strip's "+": opens a New Tab page in this pane, where what lands in
 * it is chosen (new-tab-view.tsx). It is the LAST child inside the scrolling
 * tab container (see tab-bar.tsx), so it shifts as tabs open and close, the
 * way a browser's new-tab button does. Never a `SortableEditorTab`, so it is
 * never draggable.
 */
const TabAddButton = React.memo(function TabAddButton({
  isBottomPane,
  onNewTab,
}: TabAddButtonProps) {
  if (isBottomPane) return null

  return (
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label="New tab"
      tooltip="New tab"
      tooltipSide="bottom"
      onClick={onNewTab}
      className="shrink-0 rounded-sm text-muted-foreground hover:bg-sidebar-element-hover"
    >
      <PlusIcon />
    </Button>
  )
})

export default TabAddButton
