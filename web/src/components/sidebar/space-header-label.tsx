import { useProjectHost } from '@/features/hosts/hooks/use-host-status'
import { hostDotLabel, hostToneText } from '@/features/hosts/lib/host-tone'
import type { Project } from '@/lib/types'

// 18px heading (20.7px line) over the 12px host line is 32.7px of the 34px inside an h-9 row;
// the fold button's own overflow clips anything taller.
const TITLE = 'truncate font-heading text-[18px]/[1.15] font-normal'
const HOST_LINE = 'truncate text-[10px]/[12px] font-normal text-muted-foreground'

/** The project row's name over its own host. A remote project's name and host line take the
 *  host's status colour; one whose host is no longer configured is not available, never Local. */
export function SpaceHeaderLabel({
  project,
  hostLineId,
}: {
  project: Project
  /** The host line's id, for the row's control to be described by. */
  hostLineId: string
}) {
  const { name: hostName, state, remote } = useProjectHost(project.hostId)
  const tint = (className: string) => (remote ? hostToneText(state, className) : { className })

  return (
    <span data-testid="space-header-label" className="flex min-w-0 flex-1 flex-col justify-center">
      <span {...tint(TITLE)}>{project.name}</span>
      <span id={hostLineId} {...tint(HOST_LINE)}>
        {hostName ?? hostDotLabel(state)}
        {/* Its colour is not read in the control's description: its state is, in words. */}
        {hostName !== undefined && state !== 'up' && (
          <span className="sr-only">, {hostDotLabel(state)}</span>
        )}
      </span>
    </span>
  )
}
