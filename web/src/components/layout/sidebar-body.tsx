import type { ReactNode } from 'react'
import { GearIcon } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { ErrorBoundary } from '@/components/error-boundary'
import { GitPanel } from '@/features/git/components/git-panel'
import { useUIState } from '@/features/window/stores/ui-state-store'
import type { SidebarTab } from '@/lib/store/sidebar-ui'
import { NavStack } from './nav-stack'
import { SidebarFilesPanel } from './sidebar-files-panel'
import { SidebarModeBar } from './sidebar-mode-bar'
import { useShownSidebarTab } from './sidebar-panel-tabs'

interface SidebarBodyProps {
  /** The Chats panel; the shell owns its props. */
  chatsPanel: ReactNode
}

/**
 * The mode bar over exactly one visible panel. All three stay mounted and the
 * inactive ones are `hidden`, so scroll position and folded rows survive a switch.
 */
export function SidebarBody({ chatsPanel }: SidebarBodyProps) {
  const shown = useShownSidebarTab()
  const panel = (tab: SidebarTab, children: ReactNode) => (
    <div
      data-sidebar-panel={tab}
      hidden={shown !== tab}
      className="flex min-h-0 flex-1 flex-col overflow-hidden"
    >
      {children}
    </div>
  )

  return (
    <NavStack>
      <div className="flex shrink-0 items-center gap-1 px-2 py-1.5">
        <SidebarModeBar />
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label="Settings"
          tooltip="Settings"
          tooltipSide="bottom"
          onClick={() => useUIState.getState().openSettingsDialog()}
          className="shrink-0 rounded-sm text-muted-foreground hover:bg-sidebar-element-hover"
        >
          <GearIcon size={16} />
        </Button>
      </div>
      {panel('chats', chatsPanel)}
      {panel('files', <SidebarFilesPanel />)}
      {panel(
        'git',
        <ErrorBoundary>
          <GitPanel />
        </ErrorBoundary>,
      )}
    </NavStack>
  )
}
