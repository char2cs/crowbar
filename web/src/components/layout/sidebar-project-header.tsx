import { memo } from 'react'
import { GearIcon } from '@phosphor-icons/react'
import { SidebarToggleIcon } from '@/components/ui/sidebar-toggle-icon'
import { Button } from '@/components/ui/button'
import { useSidebar } from '@/components/ui/sidebar'
import { useSettingsStore } from '@/features/settings/store'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { IS_MAC } from '@/utils/platform'
import { cn } from '@/utils/cn'
import { CHROME_ROW_HEIGHT_PX } from '@/lib/chrome-metrics'

/**
 * Sidebar top bar: the sidebar toggle on the inner edge (the side facing the
 * panes) and Settings on the outer edge, beside the macOS traffic lights when
 * the sidebar is on the left. Mirrors when the sidebar sits on the right.
 *
 * MEMOIZED: IDEShell re-renders far more often than this bar changes.
 */
export const SidebarProjectHeader = memo(function SidebarProjectHeader() {
  const sidebarPosition = useSettingsStore((s) => s.settings.sidebarPosition)
  const isRight = sidebarPosition === 'right'
  const { open: sidebarOpen, toggleSidebar } = useSidebar()

  const toggle = (
    <Button
      onClick={toggleSidebar}
      variant="ghost"
      size="icon-sm"
      className={cn(
        'shrink-0 rounded-sm text-muted-foreground hover:bg-sidebar-element-hover',
        isRight && 'scale-x-[-1]',
      )}
      tooltip={sidebarOpen ? 'Hide Sidebar' : 'Show Sidebar'}
      tooltipSide="bottom"
      aria-label={sidebarOpen ? 'Hide sidebar' : 'Show sidebar'}
    >
      <SidebarToggleIcon />
    </Button>
  )

  const settings = (
    <Button
      onClick={() => useUIState.getState().openSettingsDialog()}
      variant="ghost"
      size="icon-sm"
      className="relative z-10 shrink-0 rounded-sm text-muted-foreground hover:bg-sidebar-element-hover"
      tooltip="Settings"
      tooltipSide="bottom"
      aria-label="Settings"
    >
      <GearIcon size={16} />
    </Button>
  )

  return (
    <div
      className={cn(
        'relative flex w-full flex-shrink-0 items-center gap-1 overflow-hidden',
        // The 12px breathing room hugs the outer (screen-edge) side the sidebar
        // is docked against; the inner side uses the same 8px inset as the
        // context pill and tab bar so the buttons line up with the column.
        isRight ? 'pr-3 pl-2' : 'pl-3 pr-2',
        isRight && 'flex-row-reverse',
      )}
      style={{ height: CHROME_ROW_HEIGHT_PX }}
      data-tauri-drag-region
    >
      {/* Reserve space for the macOS traffic lights on whichever side is
          top-left (only when the sidebar is on the left). */}
      {IS_MAC && !isRight && <div className="relative z-10 w-[72px] shrink-0" />}
      {settings}
      <div className="min-w-0 flex-1" />
      {toggle}
    </div>
  )
})
