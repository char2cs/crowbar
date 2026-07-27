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
      // Same recipe as TabAddButton / CloseSplitButton beside it and as the
      // toggle in SidebarProjectHeader it swaps places with: icon-sm (28px,
      // 6px radius) and the sidebar hover token. This was icon-xs with neither
      // override, so hiding the sidebar swapped a 28px/6px button for a
      // 24px/8px one in the same spot — and left the toggle the odd control out
      // in its own toolbar row. Keep all four in sync.
      size="icon-sm"
      className={cn(
        'shrink-0 rounded-sm text-muted-foreground hover:bg-sidebar-element-hover',
        sidebarPosition === 'right' && 'scale-x-[-1]',
      )}
      tooltip={sidebarOpen ? 'Hide Sidebar' : 'Show Sidebar'}
      tooltipSide="bottom"
      aria-label={sidebarOpen ? 'Hide sidebar' : 'Show sidebar'}
    >
      <SidebarToggleIcon />
    </Button>
  )
})

export default TabNavigationButtons
