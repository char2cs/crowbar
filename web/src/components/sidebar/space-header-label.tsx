import { HostStatusDot, hostDotLabel } from '@/features/hosts/components/host-status-dot'
import { useHostDotState } from '@/features/hosts/hooks/use-host-status'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST, type HostDotState } from '@/features/hosts/types'
import type { Project } from '@/lib/types'

const UNREACHABLE: ReadonlySet<HostDotState> = new Set(['down', 'needs-repair', 'not-available'])

/**
 * The project row's two lines: its name, and the daemon it lives on with that daemon's
 * status. Only the row's own `hostId` names the host; one no longer configured is said to be
 * not available, never Local.
 */
export function SpaceHeaderLabel({ project }: { project: Project }) {
  const hostName = useHostConfigStore((s) =>
    project.hostId === LOCAL_HOST.id
      ? LOCAL_HOST.name
      : s.hosts.find((h) => h.id === project.hostId)?.name,
  )
  const status = useHostDotState(project.hostId)
  const state = hostName === undefined ? 'not-available' : status

  return (
    <span
      data-testid="space-header-label"
      data-unreachable={UNREACHABLE.has(state) || undefined}
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
