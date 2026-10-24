import type { ReactNode } from 'react'
import { ErrorBoundary } from '@/components/error-boundary'
import { GitPanel } from '@/features/git/components/git-panel'
import type { SidebarTab } from '@/lib/store/sidebar-ui'
import { NavStack } from './nav-stack'
import { SidebarFilesPanel } from './sidebar-files-panel'
import { SidebarModeBar } from './sidebar-mode-bar'
import { useShownSidebarTab } from './sidebar-panel-tabs'

interface SidebarBodyProps {
  /** The Chats panel; the shell owns its props. */
  chatsPanel: ReactNode
  /** The project switcher line above the mode bar; the shell owns the projects. */
  projectSwitcher: ReactNode
}

/**
 * The project switcher and mode bar over exactly one visible panel. All three stay mounted and the
 * inactive ones are `hidden`, so scroll position and folded rows survive a switch.
 */
export function SidebarBody({ chatsPanel, projectSwitcher }: SidebarBodyProps) {
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
      {projectSwitcher}
      <div className="flex shrink-0 items-center px-2 py-1.5">
        <SidebarModeBar />
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
