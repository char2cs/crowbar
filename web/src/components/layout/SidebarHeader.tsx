import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useProjectStore } from '@/lib/store/projects'
import { ChevronDown } from 'lucide-react'

interface SidebarHeaderProps {
  userInitials: string
  onProjectsClick?: () => void
  onProjectSelect?: (projectId: string) => void
}

export function SidebarHeader({ userInitials, onProjectsClick, onProjectSelect }: SidebarHeaderProps) {
  const { projects, activeProjectId, setActiveProject } = useProjectStore()
  const activeProject = projects.find(p => p.id === activeProjectId)

  const handleSelect = (id: string) => {
    setActiveProject(id)
    onProjectSelect?.(id)
  }

  return (
    <div className="flex h-12 flex-shrink-0 items-center gap-1.5 border-b border-border px-3">
      <div className="h-[22px] w-[22px] flex-shrink-0 rounded-[6px] bg-primary" />

      <Button
        variant="ghost"
        size="sm"
        className="h-auto px-1.5 py-0.5 text-[13px] text-muted-foreground hover:text-foreground"
        onClick={onProjectsClick}
      >
        Projects
      </Button>

      <span className="text-[13px] text-muted-foreground/40">/</span>

      <DropdownMenu>
        <DropdownMenuTrigger
          className="inline-flex h-auto items-center gap-1 rounded-sm px-1.5 py-0.5 text-[13px] font-semibold text-foreground hover:bg-accent hover:text-accent-foreground"
        >
          {activeProject?.name ?? 'Select project'}
          <ChevronDown className="h-3 w-3 text-muted-foreground" />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="min-w-[160px]">
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

      <Avatar className="ml-auto h-6 w-6">
        <AvatarFallback className="text-[10px] font-bold">{userInitials}</AvatarFallback>
      </Avatar>
    </div>
  )
}
