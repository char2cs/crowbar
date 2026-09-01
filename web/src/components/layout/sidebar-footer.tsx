import { Plus } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/utils/cn'
import { ProjectIconMark } from './project-icon-mark'
import type { Project } from '@/lib/types'

interface SidebarFooterProps {
  /** Every open project. Defaults to none — with no `onAddProject` either,
   *  the footer renders nothing at all rather than an empty reserved row. */
  projects?: Project[]
  /** Spec §4.1: "the mark and panel are two views of one number" — the SAME
   *  id SpaceScroller's own `activeProjectId` prop tracks. */
  activeProjectId?: string
  /** Mirrors SpaceScroller's `onActiveProjectChange` shape (id in, nothing
   *  out) so wiring this to that scroller later is a rename, not a rewrite. */
  onSelectProject?: (id: string) => void
  /** The tree's only entry point for a SECOND project (spec §3 ruling): a
   *  trailing `+` mark after the last project's own mark. Omitted entirely,
   *  the trailing mark just doesn't render — mirrors every other optional
   *  control here. */
  onAddProject?: () => void
}

/**
 * The sidebar's own footer: one icon-only mark per project, plus a trailing
 * `+` to add another. Deliberate placement override (task-10, this recovery
 * batch) — moved out of `SidebarProjectHeader`'s window-chrome row and off
 * the top of the sidebar entirely, to the TRUE LAST element of the sidebar's
 * vertical stack in `ide-shell.tsx`, sitting below (in normal flow, after)
 * the floating file-explorer card rather than inside it. Same rendering
 * logic as before (`data-testid=space-mark`/`add-project-mark`, icon-only,
 * current-vs-muted opacity) — only where it mounts changed.
 */
export function SidebarFooter({
  projects = [],
  activeProjectId,
  onSelectProject,
  onAddProject,
}: SidebarFooterProps = {}) {
  if (projects.length === 0 && !onAddProject) return null

  return (
    <div
      data-testid="sidebar-footer"
      className="flex w-full flex-shrink-0 items-center justify-center gap-0.5 px-2 py-1.5"
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
            className={cn('shrink-0 rounded-sm', !isActive && 'opacity-60')}
            tooltip={project.name}
            tooltipSide="top"
            aria-label={project.name}
          >
            <ProjectIconMark project={project} size="md" />
          </Button>
        )
      })}
      {onAddProject && (
        <Button
          onClick={() => onAddProject()}
          variant="ghost"
          size="icon-sm"
          data-testid="add-project-mark"
          className="shrink-0 rounded-sm"
          tooltip="Add project"
          tooltipSide="top"
          aria-label="Add project"
        >
          <Plus size={16} />
        </Button>
      )}
    </div>
  )
}
