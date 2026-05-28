import { useState } from 'react'
import { motion, AnimatePresence } from 'motion/react'
import { SquaresFour, FolderOpen, GitBranch } from '@phosphor-icons/react'
import { useSidebarStore, type SidebarTab } from '@/lib/store/sidebar'
import { useSettingsStore } from '@/features/settings/store'
import { IS_MAC, IS_WINDOWS } from '@/utils/platform'
import { Toolbar, ToolbarButton, ToolbarGroup } from '@/components/ui/toolbar'
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
  const [hoveredTab, setHoveredTab] = useState<SidebarTab | null>(null)

  // Icons sit on the edge opposite the sidebar (near the content area).
  // macOS traffic lights (~80 px) are always on the left.
  // Windows native chrome (~138 px) is always on the right.
  const wrapperClass = cn(
    'flex items-center',
    sidebarPosition === 'right'
      ? (IS_MAC     ? 'mr-auto ml-[80px]' : 'mr-auto ml-2')
      : (IS_WINDOWS ? 'ml-auto mr-[138px]' : 'ml-auto mr-2'),
  )

  return (
    <div className={wrapperClass}>
      <Toolbar className="gap-0 p-0.5 rounded-lg">
        <ToolbarGroup>
          {NAV_ITEMS.map(({ tab, label, Icon }) => {
            const isActive = activeTab === tab
            return (
              <ToolbarButton
                key={tab}
                onClick={() => setActiveTab(tab)}
                onMouseEnter={() => setHoveredTab(tab)}
                onMouseLeave={() => setHoveredTab(null)}
                aria-label={label}
                aria-pressed={isActive}
                className={cn(
                  'flex h-6 items-center gap-1 rounded-md px-1.5 transition-[background-color,color] duration-150',
                  isActive
                    ? 'bg-accent text-foreground'
                    : 'text-muted-foreground hover:text-foreground',
                )}
              >
                <Icon size={13} weight={isActive ? 'fill' : 'regular'} className="shrink-0" />
                <motion.span
                  initial={false}
                  animate={{ maxWidth: isActive || hoveredTab === tab ? '5rem' : '0rem' }}
                  transition={{ duration: 0.18, ease: [0, 0, 0.2, 1] }}
                  className="overflow-hidden whitespace-nowrap text-[11px] font-medium"
                >
                  {label}
                </motion.span>
              </ToolbarButton>
            )
          })}
        </ToolbarGroup>
      </Toolbar>
    </div>
  )
}
