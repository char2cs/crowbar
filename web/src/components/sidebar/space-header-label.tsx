import {
  HostStatusDot,
  hostDotLabel,
  hostIsUnreachable,
} from '@/features/hosts/components/host-status-dot'
import { useHostView } from '@/features/hosts/hooks/use-host-status'
import type { Project } from '@/lib/types'

/**
 * The project row's two lines: its name, and the daemon it lives on with that daemon's
 * status. Only the row's own `hostId` names the host; one no longer configured is said to be
 * not available, never Local.
 */
export function SpaceHeaderLabel({ project }: { project: Project }) {
  const { name: hostName, state } = useHostView(project.hostId)

  return (
    <span
      data-testid="space-header-label"
      data-unreachable={hostIsUnreachable(state) || undefined}
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
