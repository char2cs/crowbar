import { useEffect } from 'react'
import { useMatch } from '@tanstack/react-router'
import { SquaresFour, FolderOpen, GitBranch } from '@phosphor-icons/react'
import { Tabs, TabsList, TabsTab } from '@/components/ui/tabs'
import { cn } from '@/utils/cn'
import { useSidebarStore, type SidebarTab } from '@/lib/store/sidebar'

const TABS: {
  tab: SidebarTab
  label: string
  Icon: React.ComponentType<{ size: number; weight: 'fill' | 'regular' }>
}[] = [
  { tab: 'workspaces', label: 'Workspaces', Icon: SquaresFour },
  { tab: 'files', label: 'Files', Icon: FolderOpen },
  { tab: 'git', label: 'Git', Icon: GitBranch },
]

export function SidebarTabBar() {
  const activeTab = useSidebarStore((s) => s.activeTab)
  const setActiveTab = useSidebarStore((s) => s.setActiveTab)
  const isHomeRoute = useMatch({ from: '/_shell/ide/$projectId/home', shouldThrow: false })

  useEffect(() => {
    if (isHomeRoute && activeTab === 'git') {
      setActiveTab('workspaces')
    }
  }, [isHomeRoute, activeTab, setActiveTab])

  const visibleTabs = isHomeRoute ? TABS.filter((t) => t.tab !== 'git') : TABS

  return (
    // @container so child labels can respond to the sidebar's actual width
    <div className="@container flex shrink-0 items-center px-2 py-1.5">
      <Tabs
        value={activeTab}
        onValueChange={(v) => setActiveTab(v as SidebarTab)}
        className="w-full"
      >
        <TabsList variant="default" className="w-full bg-sidebar-element-idle text-foreground/70">
          {visibleTabs.map(({ tab, label, Icon }) => {
            const isActive = activeTab === tab
            return (
              <TabsTab
                key={tab}
                value={tab}
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
