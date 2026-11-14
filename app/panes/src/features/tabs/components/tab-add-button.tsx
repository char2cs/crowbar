import { PlusIcon } from '@phosphor-icons/react'
import React from 'react'
import { Button } from '@/components/ui/button'

interface TabAddButtonProps {
  isBottomPane: boolean
  onNewTab: () => void
}

/** The strip's "+", opening a New Tab page. The last child of the scrolling tab container, so it
 *  moves with the tabs like a browser's; never sortable, so never draggable. */
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
