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
 * The space marks (spec §4.1): one icon-only mark per project, plus a
 * trailing `+` to add another. Reconciled back onto spec (this recovery
 * batch reverses task-10's placement override): mounted in
 * `SidebarProjectHeader`'s window-chrome row — the dead middle between the
 * sidebar toggle and the back/forward/settings cluster — rather than as the
 * sidebar's own true-last sibling below the floating file-explorer card.
 * Spec §2 rules that card "ALWAYS THE LAST ELEMENT... Nothing goes below
 * it", which the old footer placement violated regardless of what it held —
 * both the marks AND the trailing add-project mark move with it.
 *
 * `min-w-0` plus `overflow-x-auto` (not `w-full` and a fixed cross-axis
 * size any more): the window-chrome row is real estate shared with
 * fixed-width siblings (the toggle, the back/forward/settings cluster), and
 * at the sidebar's minimum width (250px, see use-sidebar-panel.ts) those
 * already consume most of a 34-44px-tall row — that scarcity is the genuine
 * constraint the original override was protecting against. Marks render at
 * `icon-xs`, the row's smallest button size, and scroll horizontally rather
 * than push the toggle/cluster off screen or wrap when there are more
 * projects than fit. The `flex-1` sizing itself lives on a wrapper in
 * `SidebarProjectHeader`, not here — it has to hold the dead middle's width
 * open even on the empty render below, which this component's own callers
 * (and its tests) still expect to render nothing at all.
 *
 * Same testids as before (`space-mark`/`add-project-mark`, icon-only,
 * current-vs-muted opacity) — only where this mounts, and its own sizing,
 * changed.
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
      className="scrollbar-hidden flex min-w-0 items-center justify-center gap-0.5 overflow-x-auto [overscroll-behavior-x:contain]"
    >
      {projects.map((project) => {
        const isActive = project.id === activeProjectId
        return (
          <Button
            key={project.id}
            onClick={() => onSelectProject?.(project.id)}
            variant="ghost"
            size="icon-xs"
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
          size="icon-xs"
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
