import { useState, useEffect } from 'react'
import { motion, useMotionValue, animate } from 'motion/react'
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

type NavItemProps = {
  tab: SidebarTab
  label: string
  Icon: React.ComponentType<{ size: number; weight: 'fill' | 'regular'; className?: string }>
  isActive: boolean
}

function NavItem({ tab, label, Icon, isActive }: NavItemProps) {
  const setActiveTab = useSidebarStore(s => s.setActiveTab)
  const [hovered, setHovered] = useState(false)
  const maxWidth = useMotionValue(isActive ? 80 : 0)

  useEffect(() => {
    const controls = animate(maxWidth, isActive || hovered ? 80 : 0, {
      duration: 0.22,
      ease: [0.25, 0.1, 0.25, 1],
    })
    return controls.stop
  }, [isActive, hovered, maxWidth])

  return (
    <ToolbarButton
      onClick={() => setActiveTab(tab)}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      aria-label={label}
      aria-pressed={isActive}
      className={cn(
        'flex h-7 items-center gap-1 rounded-md px-1.5 transition-[background-color,color] duration-150',
        isActive ? 'bg-accent text-foreground' : 'text-muted-foreground hover:text-foreground',
      )}
    >
      <Icon size={14} weight={isActive ? 'fill' : 'regular'} className="shrink-0" />
      <motion.span
        style={{ maxWidth }}
        className="overflow-hidden whitespace-nowrap text-[12px] font-medium"
      >
        {label}
      </motion.span>
    </ToolbarButton>
  )
}

export function SidebarNavIcons() {
  const activeTab       = useSidebarStore(s => s.activeTab)
  const sidebarPosition = useSettingsStore(s => s.settings.sidebarPosition)

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
          {NAV_ITEMS.map(({ tab, label, Icon }) => (
            <NavItem
              key={tab}
              tab={tab}
              label={label}
              Icon={Icon}
              isActive={activeTab === tab}
            />
          ))}
        </ToolbarGroup>
      </Toolbar>
    </div>
  )
}
