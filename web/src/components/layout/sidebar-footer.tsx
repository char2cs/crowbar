import { Plus, Settings } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useSettingsStore } from '@/features/settings/store'
import { useUIState } from '@/features/window/stores/ui-state-store'
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
   *  `+` mark pinned to the content-facing edge, opposite Settings. Omitted
   *  entirely, the mark just doesn't render — mirrors every other optional
   *  control here. */
  onAddProject?: () => void
}

/**
 * The space marks: one icon-only mark per project, plus a trailing `+` to
 * add another. Mounted in `ide-shell.tsx` as its own row — the sidebar's
 * true last element, below the floating file-explorer card — per the
 * product owner's explicit call: the marks read as a squeezed afterthought
 * wedged into `SidebarProjectHeader`'s window-chrome row (the dead middle
 * between the toggle and the back/forward/settings cluster), which is where
 * an earlier pass had reconciled them back to.
 *
 * `justify-center` and `size="icon-sm"` +
 * `ProjectIconMark size="lg"` (the tree row's own 20px mark, see
 * project-icon-mark.tsx) replace the old `icon-xs` + horizontal-scroll
 * treatment — that was a direct adaptation to a shared, fixed-height
 * window-chrome row; a full-width row of its own has no such scarcity.
 * `flex-wrap` lets an unusual number of open projects wrap to a second line
 * instead of scrolling or overflowing.
 *
 * Same testids as before (`space-mark`/`add-project-mark`, icon-only,
 * current-vs-muted opacity) — only where this mounts, and its own sizing,
 * changed.
 *
 * Settings also lives here now, out of `SidebarProjectHeader`'s trailing
 * cluster. Unlike the marks it does not join the centered group — it is
 * absolutely pinned to the OUTER edge, flush against the window frame
 * (left when the sidebar is docked left, right when docked right), so it
 * reads as "the sidebar's own settings, at the edge of the window" rather
 * than one more centered mark. `+` mirrors it on the opposite (content-
 * facing) edge for the same reason.
 */
export function SidebarFooter({
  projects = [],
  activeProjectId,
  onSelectProject,
  onAddProject,
}: SidebarFooterProps = {}) {
  const sidebarPosition = useSettingsStore((s) => s.settings.sidebarPosition)
  const isRight = sidebarPosition === 'right'

  if (projects.length === 0 && !onAddProject) return null

  return (
    <div
      data-testid="sidebar-footer"
      className="relative flex shrink-0 flex-wrap items-center justify-center gap-1 px-2 py-1.5"
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
              // Full color while hovered — a preview of "go here" — even
              // though the space isn't current yet; only truly idle marks
              // stay dimmed.
              !isActive && 'opacity-60 hover:opacity-100',
            )}
            tooltip={project.name}
            tooltipSide="top"
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
          className={cn(
            'absolute top-1.5 shrink-0 rounded-sm text-muted-foreground hover:bg-sidebar-element-hover',
            isRight ? 'left-2' : 'right-2',
          )}
          tooltip="Add space"
          tooltipSide="top"
          aria-label="Add space"
        >
          <Plus size={16} />
        </Button>
      )}
      <Button
        onClick={() => useUIState.getState().openSettingsDialog()}
        variant="ghost"
        size="icon-sm"
        data-testid="settings-button"
        className={cn(
          'absolute top-1.5 shrink-0 rounded-sm text-muted-foreground hover:bg-sidebar-element-hover',
          isRight ? 'right-2' : 'left-2',
        )}
        tooltip="Settings"
        tooltipSide="top"
        aria-label="Settings"
      >
        <Settings size={16} />
      </Button>
    </div>
  )
}
