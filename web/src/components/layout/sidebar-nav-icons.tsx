import { SquaresFour, FolderOpen, GitBranch } from '@phosphor-icons/react'
import { useSidebarStore, type SidebarTab } from '@/lib/store/sidebar'
import { useSettingsStore } from '@/features/settings/store'
import { IS_MAC, IS_WINDOWS } from '@/utils/platform'
import Tooltip from '@/components/ui/tooltip'
import { cn } from '@/utils/cn'

const NAV_ITEMS = [
  { tab: 'workspaces' as SidebarTab, label: 'Workspaces', Icon: SquaresFour },
  { tab: 'files'      as SidebarTab, label: 'Files',      Icon: FolderOpen  },
  { tab: 'git'        as SidebarTab, label: 'Git',        Icon: GitBranch   },
] as const

export function SidebarNavIcons() {
  const activeTab       = useSidebarStore(s => s.activeTab)
  const setActiveTab    = useSidebarStore(s => s.setActiveTab)
  const sidebarPosition = useSettingsStore(s => s.settings.sidebarPosition)

  // Icons always anchor to the sidebar edge.
  // macOS traffic lights (~80 px) occupy the left side of the window — only
  // relevant when the sidebar is on the left.
  // Windows native chrome (~138 px) occupies the right side — only relevant
  // when the sidebar is on the right.
  const groupClass = cn(
    'flex items-center gap-1',
    sidebarPosition === 'right'
      ? (IS_WINDOWS ? 'mr-[138px]' : 'mr-2')
      : (IS_MAC     ? 'ml-[80px]'  : 'ml-2'),
  )

  return (
    <div className={groupClass}>
      {NAV_ITEMS.map(({ tab, label, Icon }) => (
        <Tooltip key={tab} content={label} side="bottom">
          <button
            onClick={() => setActiveTab(tab)}
            aria-label={label}
            aria-pressed={activeTab === tab}
            className={cn(
              'flex h-7 w-7 items-center justify-center rounded-md transition-colors',
              activeTab === tab
                ? 'bg-accent text-foreground'
                : 'text-muted-foreground hover:text-foreground',
            )}
          >
            <Icon size={15} weight={activeTab === tab ? 'fill' : 'regular'} />
          </button>
        </Tooltip>
      ))}
    </div>
  )
}
