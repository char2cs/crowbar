import { PlusIcon } from '@phosphor-icons/react'
import React from 'react'
import { Button } from '@/components/ui/button'

interface TabAddButtonProps {
  isBottomPane: boolean
  onNewTab: () => void
}

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
