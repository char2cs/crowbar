import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { useProjectStore } from '@/lib/store/projects'
import { useSettingsStore } from '@/features/settings/store'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { IS_MAC } from '@/utils/platform'
import { cn } from '@/utils/cn'
import { ChevronDown } from 'lucide-react'
import { GearSix } from '@phosphor-icons/react'

export function projectNameToHue(name: string): number {
  let hash = 0
  for (let i = 0; i < name.length; i++) {
    hash = (hash * 31 + name.charCodeAt(i)) >>> 0
  }
  return hash % 360
}

interface SidebarProjectHeaderProps {
  onProjectsClick?: () => void
  onProjectSelect?: (projectId: string) => void
}

export function SidebarProjectHeader({ onProjectsClick, onProjectSelect }: SidebarProjectHeaderProps) {
  const projects = useProjectStore(s => s.projects)
  const activeProjectId = useProjectStore(s => s.activeProjectId)
  const sidebarPosition = useSettingsStore(s => s.settings.sidebarPosition)

  const activeProject = projects.find(p => p.id === activeProjectId)
  const hue = projectNameToHue(activeProject?.name ?? '')
  const isRight = sidebarPosition === 'right'

  const bloomStyle = {
    background: `linear-gradient(${isRight ? '270deg' : '90deg'}, hsla(${hue}, 40%, 60%, 0.35) 0%, hsla(${hue}, 40%, 60%, 0.08) 60%, transparent 100%)`,
  }

  const handleSelect = (id: string) => {
    useProjectStore.getState().setActiveProject(id)
    onProjectSelect?.(id)
  }

  return (
    <div
      className={cn(
        'relative flex w-full flex-shrink-0 items-center overflow-hidden px-3',
        IS_MAC ? 'h-[44px]' : 'h-[34px]',
      )}
      data-tauri-drag-region
    >
      {/* Bloom gradient — header height only, no bottom border */}
      <div className="pointer-events-none absolute inset-0 z-0" style={bloomStyle} />

      {/* On Mac with left sidebar, leave space for OS traffic lights */}
      {IS_MAC && !isRight && <div className="relative z-10 w-[52px] shrink-0" />}

      <DropdownMenu>
        <DropdownMenuTrigger
          className={cn(
            'relative z-10 inline-flex items-center gap-1 rounded-sm px-1.5 py-0.5 text-[13px] font-semibold text-foreground outline-none hover:bg-accent/50',
            isRight ? 'mr-auto' : 'ml-auto',
          )}
        >
          {activeProject?.name ?? 'Select project'}
          <ChevronDown className="h-3 w-3 text-muted-foreground" />
        </DropdownMenuTrigger>
        <DropdownMenuContent align={isRight ? 'start' : 'end'} style={{ minWidth: '160px' }}>
          {projects.map(p => (
            <DropdownMenuItem
              key={p.id}
              onClick={() => handleSelect(p.id)}
              className={p.id === activeProjectId ? 'font-medium text-primary' : ''}
            >
              {p.name}
            </DropdownMenuItem>
          ))}
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={onProjectsClick} className="text-muted-foreground">
            Manage projects…
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <button
        onClick={() => useUIState.getState().openSettingsDialog()}
        aria-label="Settings"
        className="relative z-10 flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
      >
        <GearSix size={16} />
      </button>

      {/* On Mac with right sidebar, traffic lights are in content area — no spacer needed */}
    </div>
  )
}
