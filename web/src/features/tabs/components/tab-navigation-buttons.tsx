// web/src/features/tabs/components/tab-navigation-buttons.tsx
import { SidebarToggleIcon } from '@/components/ui/sidebar-toggle-icon'
import React from 'react'
import { Button } from '@/components/ui/button'
import { cn } from '@/utils/cn'

interface TabNavigationButtonsProps {
  isBottomPane: boolean
  sidebarOpen: boolean
  sidebarPosition: 'left' | 'right'
  onToggleSidebar: () => void
}

const TabNavigationButtons = React.memo(function TabNavigationButtons({
  isBottomPane,
  sidebarOpen,
  sidebarPosition,
  onToggleSidebar,
}: TabNavigationButtonsProps) {
  if (isBottomPane) return null
  return (
    <Button
      onClick={onToggleSidebar}
      variant="ghost"
      size="icon-xs"
      className={cn(
        'shrink-0 text-muted-foreground',
        sidebarPosition === 'right' && 'scale-x-[-1]',
      )}
      tooltip={sidebarOpen ? 'Hide Sidebar' : 'Show Sidebar'}
      tooltipSide="bottom"
      aria-label={sidebarOpen ? 'Hide sidebar' : 'Show sidebar'}
    >
      <SidebarToggleIcon size={14} />
    </Button>
  )
})

export default TabNavigationButtons
