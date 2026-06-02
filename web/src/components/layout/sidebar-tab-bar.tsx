import { SquaresFour, ChatsCircle, FolderOpen, GitBranch } from '@phosphor-icons/react'
import { Tabs, TabsList, TabsTab } from '@/components/ui/tabs'
import { useSidebarStore, type SidebarTab } from '@/lib/store/sidebar'

const TABS: { tab: SidebarTab; label: string; Icon: React.ComponentType<{ size: number; weight: 'fill' | 'regular' }> }[] = [
  { tab: 'workspaces', label: 'Workspaces', Icon: SquaresFour },
  { tab: 'chats',      label: 'Chats',      Icon: ChatsCircle  },
  { tab: 'files',      label: 'Files',      Icon: FolderOpen   },
  { tab: 'git',        label: 'Git',        Icon: GitBranch    },
]

export function SidebarTabBar() {
  const activeTab = useSidebarStore(s => s.activeTab)
  const setActiveTab = useSidebarStore(s => s.setActiveTab)

  return (
    <div className="flex shrink-0 items-center px-2 py-1.5">
      <Tabs
        value={activeTab}
        onValueChange={(v) => setActiveTab(v as SidebarTab)}
        className="w-full"
      >
        <TabsList variant="default" className="w-full">
          {TABS.map(({ tab, label, Icon }) => (
            <TabsTab
              key={tab}
              value={tab}
              className="flex flex-1 items-center justify-center gap-1"
            >
              <Icon size={14} weight={activeTab === tab ? 'fill' : 'regular'} />
              <span>{label}</span>
            </TabsTab>
          ))}
        </TabsList>
      </Tabs>
    </div>
  )
}
