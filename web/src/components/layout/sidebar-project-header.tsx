import { memo } from 'react'
import { SidebarToggleIcon } from '@/components/ui/sidebar-toggle-icon'
import { Button } from '@/components/ui/button'
import { useSidebar } from '@/components/ui/sidebar'
import { useSettingsStore } from '@/features/settings/store'
import { IS_MAC } from '@/utils/platform'
import { cn } from '@/utils/cn'
import { BuildReadout } from './build-readout'
import { SidebarSpaceMarks } from './sidebar-space-marks'
import type { Project } from '@/lib/types'
import { CHROME_ROW_HEIGHT_PX } from '@/lib/chrome-metrics'

interface SidebarProjectHeaderProps {
  projects: Project[]
  activeProjectId?: string
  onSelectProject: (id: string) => void
  onAddProject: () => void
}

/**
 * Sidebar top bar: the space marks beside the traffic lights, the build
 * readout, then the sidebar toggle on the inner edge (the side facing the
 * panes). Mirrors when the sidebar sits on the right.
 * Settings lives in the file/git card's head (`SidebarCarousel`).
 *
 * MEMOIZED: IDEShell re-renders far more often than these props change.
 */
export const SidebarProjectHeader = memo(function SidebarProjectHeader({
  projects,
  activeProjectId,
  onSelectProject,
  onAddProject,
}: SidebarProjectHeaderProps) {
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
      <div
        className={cn(
          'relative z-10 flex min-w-0 flex-1 items-center gap-1',
          isRight && 'flex-row-reverse',
        )}
      >
        <SidebarSpaceMarks
          projects={projects}
          activeProjectId={activeProjectId}
          onSelectProject={onSelectProject}
          onAddProject={onAddProject}
        />
      </div>
      {toggle}
      {/* Beside the toggle; an overlay, so it moves nothing. On a left or hidden sidebar it sits at the end of the tab strip instead. */}
      {isRight && <BuildReadout className="left-11" />}
    </div>
  )
})
