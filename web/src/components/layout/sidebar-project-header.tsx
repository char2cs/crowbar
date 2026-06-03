import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { useProjectStore } from '@/lib/store/projects'
import { useSettingsStore } from '@/features/settings/store'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { IS_MAC } from '@/utils/platform'
import { cn } from '@/utils/cn'
import { ChevronDown } from 'lucide-react'
import { GearSix } from '@phosphor-icons/react'

interface SidebarProjectHeaderProps {
  onProjectsClick?: () => void
  onProjectSelect?: (projectId: string) => void
}

export function SidebarProjectHeader({ onProjectsClick, onProjectSelect }: SidebarProjectHeaderProps) {
  const projects = useProjectStore(s => s.projects)
  const activeProjectId = useProjectStore(s => s.activeProjectId)
  const sidebarPosition = useSettingsStore(s => s.settings.sidebarPosition)

  const activeProject = projects.find(p => p.id === activeProjectId)
  const isRight = sidebarPosition === 'right'

  const handleSelect = (id: string) => {
    useProjectStore.getState().setActiveProject(id)
    onProjectSelect?.(id)
  }

  return (
    <div
      className={cn(
        'flex w-full flex-shrink-0 items-center px-3',
        IS_MAC ? 'h-[44px]' : 'h-[34px]',
      )}
      data-tauri-drag-region
    >
      {IS_MAC && !isRight && <div className="w-[52px] shrink-0" />}

      <DropdownMenu>
        <DropdownMenuTrigger
          className={cn(
            'inline-flex items-center gap-1 rounded-sm px-1.5 py-0.5 text-[13px] font-semibold text-foreground outline-none hover:bg-accent/50',
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
        className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
      >
        <GearSix size={16} />
      </button>
    </div>
  )
}
