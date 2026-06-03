// web/src/components/projects/ProjectCard.tsx
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { formatRelativeDate } from '@/utils/date'
import type { Project } from '@/lib/types'

interface ProjectCardProps {
  project: Project
  active?: boolean
  repoCount?: number
  onClick: () => void
}

export function ProjectCard({ project, active, repoCount = 0, onClick }: ProjectCardProps) {
  return (
    <Card
      className={`cursor-pointer transition-colors hover:bg-accent/50 ${active ? 'ring-1 ring-primary' : ''}`}
      onClick={onClick}
    >
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-semibold">{project.name}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-1">
        <p className="truncate font-mono text-[11px] text-muted-foreground">{project.path}</p>
        <div className="flex items-center gap-2">
          <Badge variant="outline" className="text-[10px]">{repoCount} repos</Badge>
          <span className="text-[11px] text-muted-foreground/60">{formatRelativeDate(project.lastActivity)}</span>
        </div>
      </CardContent>
    </Card>
  )
}
