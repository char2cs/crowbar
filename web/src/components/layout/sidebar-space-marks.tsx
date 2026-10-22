import { memo } from 'react'
import { PlusIcon } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { cn } from '@/utils/cn'
import { ProjectIconMark } from './project-icon-mark'
import type { Project } from '@/lib/types'

interface SidebarSpaceMarksProps {
  /** Every open project. With none and no `onAddProject`, nothing renders. */
  projects?: Project[]
  /** The id the tree's own `activeProjectId` tracks — mark and panel are two views of one number. */
  activeProjectId?: string
  onSelectProject?: (id: string) => void
  /** The tree's only entry point for a SECOND project. */
  onAddProject?: () => void
}

/**
 * One icon-only mark per project plus a trailing `+`, sized to sit inline in
 * `SidebarProjectHeader`'s window-chrome row. Memoized: every prop is a
 * primitive or a stable callback, and IDEShell re-renders far more often.
 */
export const SidebarSpaceMarks = memo(function SidebarSpaceMarks({
  projects = [],
  activeProjectId,
  onSelectProject,
  onAddProject,
}: SidebarSpaceMarksProps = {}) {
  if (projects.length === 0 && !onAddProject) return null

  return (
    <div
      data-testid="sidebar-space-marks"
      className="relative z-10 flex min-w-0 items-center gap-1"
    >
      {projects.map((project) => {
        const isActive = project.id === activeProjectId
        return (
          <Button
            key={project.id}
            onClick={() => onSelectProject?.(project.id)}
            variant="ghost"
            size="icon-sm"
            data-testid="space-mark"
            aria-current={isActive || undefined}
            className={cn(
              'shrink-0 rounded-sm text-muted-foreground hover:bg-sidebar-element-hover',
              // Full color while hovered previews "go here"; only idle marks stay dimmed.
              !isActive && 'opacity-60 hover:opacity-100',
            )}
            tooltip={project.name}
            tooltipSide="bottom"
            aria-label={project.name}
          >
            <ProjectIconMark project={project} size="lg" />
          </Button>
        )
      })}
      {onAddProject && (
        <Button
          onClick={() => onAddProject()}
          variant="ghost"
          size="icon-sm"
          data-testid="add-project-mark"
          className="shrink-0 rounded-sm text-muted-foreground hover:bg-sidebar-element-hover"
          tooltip="Add space"
          tooltipSide="bottom"
          aria-label="Add space"
        >
          <PlusIcon size={16} />
        </Button>
      )}
    </div>
  )
})
