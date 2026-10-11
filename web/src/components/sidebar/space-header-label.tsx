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
export function SpaceHeaderLabel({ project }: { project: Project }) {
  const { name: hostName, state } = useHostView(project.hostId)
  const dimmed = project.hostId !== LOCAL_HOST.id && hostIsUnreachable(state)

  return (
    <span
      data-testid="space-header-label"
      data-unreachable={dimmed || undefined}
      className="flex min-w-0 flex-1 flex-col justify-center gap-px data-unreachable:opacity-55"
    >
      <span className="truncate font-heading text-[15px]/[1.15] font-normal">{project.name}</span>
      <span className="flex min-w-0 items-center gap-1.25 text-[11px]/[13px] font-normal text-muted-foreground">
        <HostStatusDot hostId={project.hostId} state={state} className="size-1.5" />
        <span className="truncate">{hostName ?? hostDotLabel(state)}</span>
      </span>
    </span>
  )
}
