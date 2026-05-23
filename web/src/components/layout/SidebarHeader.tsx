import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'

interface SidebarHeaderProps {
  projectName: string
  userInitials: string
  onProjectsClick?: () => void
}

export function SidebarHeader({ projectName, userInitials, onProjectsClick }: SidebarHeaderProps) {
  return (
    <div className="flex h-12 flex-shrink-0 items-center gap-1.5 border-b border-border px-3">
      <div className="h-[22px] w-[22px] flex-shrink-0 rounded-[6px] bg-primary" />
      <Button
        variant="ghost"
        size="sm"
        className="h-auto px-1.5 py-0.5 text-[13px] text-muted-foreground"
        onClick={onProjectsClick}
      >
        Projects
      </Button>
      <span className="text-[13px] text-muted-foreground/40">/</span>
      <span className="text-[13px] font-semibold text-foreground">{projectName}</span>
      <Avatar className="ml-auto h-6 w-6">
        <AvatarFallback className="text-[10px] font-bold">{userInitials}</AvatarFallback>
      </Avatar>
    </div>
  )
}
