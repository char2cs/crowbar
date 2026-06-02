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
    <div className="flex h-8 shrink-0 items-center justify-center px-2">
      <div className="flex items-center gap-0.5 rounded-lg border border-white/8 bg-background p-0.5 shadow-sm shadow-black/30 not-disabled:inset-shadow-[0_1px_--theme(--color-white/16%)]">
        {TABS.map(({ tab, label, Icon }) => {
          const isActive = activeTab === tab
          return (
            <Tab
              key={tab}
              isActive={isActive}
              aria-label={label}
              aria-pressed={isActive}
              data-active={isActive || undefined}
              className="flex h-7 items-center gap-1 px-2"
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
