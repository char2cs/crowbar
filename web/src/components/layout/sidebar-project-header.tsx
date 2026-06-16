import { ArrowLeft, ArrowRight, GearSix, SidebarSimple } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { useSidebar } from '@/components/ui/sidebar'
import { useSettingsStore } from '@/features/settings/store'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { useJumpNavigation } from '@/features/tabs/hooks/use-jump-navigation'
import { useActiveWebViewerNavigation } from '@/features/tabs/hooks/use-active-webviewer-navigation'
import { IS_MAC } from '@/utils/platform'
import { cn } from '@/utils/cn'

/**
 * Zen-style sidebar top bar: a sidebar-toggle on the leading edge and a
 * back / forward / settings cluster on the trailing edge. Mirrors when the
 * sidebar sits on the right. Back/forward reuse the editor jump navigation.
 */
export function SidebarProjectHeader() {
  const sidebarPosition = useSettingsStore((s) => s.settings.sidebarPosition)
  const isRight = sidebarPosition === 'right'
  const { open: sidebarOpen, toggleSidebar } = useSidebar()
  const { canGoBack, canGoForward, handleJumpBack, handleJumpForward } = useJumpNavigation(
    useActiveWebViewerNavigation(),
  )

  const toggle = (
    <Button
      onClick={toggleSidebar}
      variant="ghost"
      size="icon-sm"
      className={cn('shrink-0 text-muted-foreground', isRight && 'scale-x-[-1]')}
      tooltip={sidebarOpen ? 'Hide Sidebar' : 'Show Sidebar'}
      tooltipSide="bottom"
      aria-label={sidebarOpen ? 'Hide sidebar' : 'Show sidebar'}
    >
      <SidebarSimple size={16} />
    </Button>
  )

  const cluster = (
    <div className="flex shrink-0 items-center gap-0.5">
      <Button
        onClick={() => void handleJumpBack()}
        disabled={!canGoBack}
        variant="ghost"
        size="icon-sm"
        className="shrink-0 text-muted-foreground"
        tooltip="Go Back"
        tooltipSide="bottom"
        aria-label="Go back to previous location"
      >
        <ArrowLeft size={16} />
      </Button>
      <Button
        onClick={() => void handleJumpForward()}
        disabled={!canGoForward}
        variant="ghost"
        size="icon-sm"
        className="shrink-0 text-muted-foreground"
        tooltip="Go Forward"
        tooltipSide="bottom"
        aria-label="Go forward to next location"
      >
        <ArrowRight size={16} />
      </Button>
      <Button
        onClick={() => useUIState.getState().openSettingsDialog()}
        variant="ghost"
        size="icon-sm"
        className="shrink-0 text-muted-foreground"
        tooltip="Settings"
        tooltipSide="bottom"
        aria-label="Settings"
      >
        <GearSix size={16} />
      </Button>
    </div>
  )

  return (
    <div
      className={cn(
        'flex w-full flex-shrink-0 items-center gap-1 px-3',
        IS_MAC ? 'h-[44px]' : 'h-[34px]',
        isRight && 'flex-row-reverse',
      )}
      data-tauri-drag-region
    >
      {/* Reserve space for the macOS traffic lights on whichever side is
          top-left (only when the sidebar is on the left). */}
      {IS_MAC && !isRight && <div className="w-[52px] shrink-0" />}
      {toggle}
      <div className="flex-1" />
      {cluster}
    </div>
  )
}
