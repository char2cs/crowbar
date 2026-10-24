import { Tabs, TabsList, TabsTab } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useFocusedWorkspaceContextStore } from '@/features/window/stores/focused-workspace-context-store'
import {
  SIDEBAR_PANEL_TABS,
  availableSidebarPanelTabsKey,
  useShownSidebarTab,
} from './sidebar-panel-tabs'

/**
 * The sidebar's mode switch: Chats / Files / Git as a segmented control with a sliding pill.
 * The active segment shows a filled icon and its label; the rest show their label once wide enough.
 */
export function SidebarModeBar({ className }: { className?: string }) {
  const shown = useShownSidebarTab()
  const setActiveTab = useSidebarStore((s) => s.setActiveTab)
  const availableKey = useFocusedWorkspaceContextStore(availableSidebarPanelTabsKey)
  const available = new Set(availableKey.split(','))

  return (
    // @container so child labels can respond to the sidebar's actual width
    <div className={cn('@container flex shrink-0 items-center px-2 py-1.5', className)}>
      <Tabs value={shown} onValueChange={(v) => setActiveTab(v as typeof shown)} className="w-full">
        <TabsList
          variant="default"
          aria-label="Sidebar mode"
          className="w-full bg-sidebar-element-idle text-foreground/70"
        >
          {SIDEBAR_PANEL_TABS.map(({ tab, label, Icon }) => {
            const isActive = shown === tab
            return (
              <TabsTab
                key={tab}
                value={tab}
                aria-label={label}
                disabled={!available.has(tab)}
                className="flex flex-1 items-center justify-center gap-1"
              >
                <Icon size={14} weight={isActive ? 'fill' : 'regular'} />
                <span
                  className={cn(
                    // < 280px: no labels for anyone
                    'hidden',
                    // 280–420px: label only for the active tab
                    isActive && '@[280px]:inline',
                    // ≥ 420px: labels for all tabs
                    '@[420px]:inline',
                  )}
                >
                  {label}
                </span>
              </TabsTab>
            )
          })}
        </TabsList>
      </Tabs>
    </div>
  )
}
