import {
  HostStatusDot,
  hostDotLabel,
  hostIsUnreachable,
} from '@/features/hosts/components/host-status-dot'
import { useHostView } from '@/features/hosts/hooks/use-host-status'
import { LOCAL_HOST } from '@/features/hosts/types'
import type { Project } from '@/lib/types'

/** The project row's name over its own host and that host's status: one no longer configured
 *  is not available, never Local. Only a remote host's row dims. */
export function SpaceHeaderLabel({
  project,
  hostLineId,
}: {
  project: Project
  /** The host line's id, for the row's control to be described by. */
  hostLineId: string
}) {
  const { name: hostName, state } = useHostView(project.hostId)
  const dimmed = project.hostId !== LOCAL_HOST.id && hostIsUnreachable(state)

  return (
    <span
      data-testid="space-header-label"
      data-unreachable={dimmed || undefined}
      className="group/label flex min-w-0 flex-1 flex-col justify-center gap-px"
    >
      {/* Only the name dims, less in light than in dark: every line keeps WCAG AA contrast. */}
      <span className="truncate font-heading text-[15px]/[1.15] font-normal group-data-unreachable/label:opacity-70 dark:group-data-unreachable/label:opacity-55">
        {project.name}
      </span>
      <span
        id={hostLineId}
        className="flex min-w-0 items-center gap-1.25 text-[11px]/[13px] font-normal text-muted-foreground"
      >
        <HostStatusDot hostId={project.hostId} state={state} className="size-1.5" />
        <span className="truncate">{hostName ?? hostDotLabel(state)}</span>
      </span>
    </span>
  )
}
