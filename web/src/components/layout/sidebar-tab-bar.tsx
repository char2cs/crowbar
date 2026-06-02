import { SquaresFour, ChatsCircle, FolderOpen, GitBranch } from '@phosphor-icons/react'
import { Tab } from '@/components/ui/tabs'
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
      <div className="relative z-0 flex w-full items-center justify-center gap-x-0.5 rounded-lg bg-muted p-0.5 text-muted-foreground/72">
        {TABS.map(({ tab, label, Icon }) => {
          const isActive = activeTab === tab
          return (
            <Tab
              key={tab}
              isActive={isActive}
              aria-label={label}
              aria-pressed={isActive}
              data-active={isActive || undefined}
              className="flex flex-1 h-7 items-center justify-center gap-1 px-2"
              onClick={() => setActiveTab(tab)}
            >
              <Icon size={14} weight={isActive ? 'fill' : 'regular'} />
              <span className="text-[13px] font-medium">{label}</span>
            </Tab>
          )
        })}
      </div>
    </div>
  )
}
