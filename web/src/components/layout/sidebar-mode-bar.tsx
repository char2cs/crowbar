import { Tab } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useFocusedWorkspaceContextStore } from '@/features/window/stores/focused-workspace-context-store'
import {
  SIDEBAR_PANEL_TABS,
  availableSidebarPanelTabsKey,
  useShownSidebarTab,
} from './sidebar-panel-tabs'

/**
 * The sidebar's mode switch: Chats / Files / Git as one segmented control.
 * The active segment shows a filled icon and its label; the rest show their
 * icon, and their label too once the sidebar is wide enough.
 */
export function SidebarModeBar({ className }: { className?: string }) {
  const shown = useShownSidebarTab()
  const setActiveTab = useSidebarStore((s) => s.setActiveTab)
  const availableKey = useFocusedWorkspaceContextStore(availableSidebarPanelTabsKey)
  const available = new Set(availableKey.split(','))

  return (
    // @container so the labels follow the sidebar's own width, not the window's.
    <div className={cn('@container min-w-0 flex-1', className)}>
      <div
        role="tablist"
        aria-label="Sidebar mode"
        className="flex w-full items-center gap-0.5 rounded-lg bg-muted p-0.5"
      >
        {SIDEBAR_PANEL_TABS.map(({ tab, label, Icon }) => {
          const isActive = shown === tab
          return (
            <Tab
              key={tab}
              role="tab"
              aria-selected={isActive}
              aria-label={label}
              isActive={isActive}
              disabled={!available.has(tab)}
              onClick={() => setActiveTab(tab)}
              className="h-7 min-w-0 flex-1 justify-center gap-1 rounded-md px-2 text-xs"
            >
              <Icon size={14} weight={isActive ? 'fill' : 'regular'} />
              <span className={cn('truncate', !isActive && 'hidden @[420px]:inline')}>{label}</span>
            </Tab>
          )
        })}
      </div>
    </div>
  )
}
