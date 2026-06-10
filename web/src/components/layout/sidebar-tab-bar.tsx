import { SquaresFour, ChatsCircle, FolderOpen, GitBranch } from '@phosphor-icons/react'
import { Tabs, TabsList, TabsTab } from '@/components/ui/tabs'
import { cn } from '@/utils/cn'
import { useSidebarStore, type SidebarTab } from '@/lib/store/sidebar'

const TABS: {
  tab: SidebarTab
  label: string
  Icon: React.ComponentType<{ size: number; weight: 'fill' | 'regular' }>
}[] = [
  { tab: 'workspaces', label: 'Workspaces', Icon: SquaresFour },
  { tab: 'chats', label: 'Chats', Icon: ChatsCircle },
  { tab: 'files', label: 'Files', Icon: FolderOpen },
  { tab: 'git', label: 'Git', Icon: GitBranch },
]

export function SidebarTabBar() {
  const activeTab = useSidebarStore((s) => s.activeTab)
  const setActiveTab = useSidebarStore((s) => s.setActiveTab)

  return (
    // @container so child labels can respond to the sidebar's actual width
    <div className="@container flex shrink-0 items-center px-2 py-1.5">
      <Tabs
        value={activeTab}
        onValueChange={(v) => setActiveTab(v as SidebarTab)}
        className="w-full"
      >
        <TabsList variant="default" className="w-full">
          {TABS.map(({ tab, label, Icon }) => {
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
