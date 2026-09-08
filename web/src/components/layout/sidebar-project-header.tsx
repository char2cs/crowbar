// Lucide (ISC) rather than Phosphor for this cluster: long-tail arrows and a
// panel glyph — the toolbar language this app is aiming at.
import { ArrowLeft, ArrowRight } from 'lucide-react'
import { SidebarToggleIcon } from '@/components/ui/sidebar-toggle-icon'
import { Button } from '@/components/ui/button'
import { useSidebar } from '@/components/ui/sidebar'
import { useSettingsStore } from '@/features/settings/store'
import { useJumpNavigation } from '@/features/tabs/hooks/use-jump-navigation'
import { IS_MAC } from '@/utils/platform'
import { cn } from '@/utils/cn'

/**
 * Sidebar top bar: a back / forward / sidebar-toggle cluster on the trailing
 * edge, with a `flex-1` spacer holding it off the traffic-light side.
 * Mirrors when the sidebar sits on the right. Back/forward reuse the editor
 * jump navigation.
 *
 * Settings lives in `SidebarFooter` now, pinned to the content-facing edge
 * of the project-marks row — see that file's doc comment.
 */
export function SidebarProjectHeader() {
  const sidebarPosition = useSettingsStore((s) => s.settings.sidebarPosition)
  const isRight = sidebarPosition === 'right'
  const { open: sidebarOpen, toggleSidebar } = useSidebar()
  const { canGoBack, canGoForward, handleJumpBack, handleJumpForward } = useJumpNavigation()

  const cluster = (
    <div className="flex shrink-0 items-center gap-0.5">
      <Button
        onClick={() => void handleJumpBack()}
        disabled={!canGoBack}
        variant="ghost"
        size="icon-sm"
        className="shrink-0 rounded-sm text-muted-foreground hover:bg-sidebar-element-hover"
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
        className="shrink-0 rounded-sm text-muted-foreground hover:bg-sidebar-element-hover"
        tooltip="Go Forward"
        tooltipSide="bottom"
        aria-label="Go forward to next location"
      >
        <ArrowRight size={16} />
      </Button>
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
    </div>
  )

  return (
    <div
      className={cn(
        'flex w-full flex-shrink-0 items-center gap-1',
        // The 12px breathing room hugs the outer (screen-edge) side the sidebar
        // is docked against; the inner side uses the same 8px inset as the
        // context pill and tab bar so the buttons line up with the column.
        isRight ? 'pr-3 pl-2' : 'pl-3 pr-2',
        IS_MAC ? 'h-[44px]' : 'h-[34px]',
        isRight && 'flex-row-reverse',
      )}
      data-tauri-drag-region
    >
      {/* Reserve space for the macOS traffic lights on whichever side is
          top-left (only when the sidebar is on the left). */}
      {IS_MAC && !isRight && <div className="w-[72px] shrink-0" />}
      <div className="min-w-0 flex-1" />
      {cluster}
    </div>
  )
}
