import { useParams } from '@tanstack/react-router'
import { useHomeWorkspaceState } from '@/features/workspace/lib/home-workspace-resolver'
import { useHostUnavailable } from '@/lib/host-unavailability'

/** The home route's loading/error/host-down state; WorkspaceHost retains and paints the home workspace beside the Outlet, so a host that is down is named only when no home workspace is known to show. */
export function ProjectHomeNotice() {
  const { projectId } = useParams({ from: '/_shell/ide/$projectId/home' })
  const { wsId, error } = useHomeWorkspaceState(projectId)
  const unavailable = useHostUnavailable({ projectId })
  const noHomeToShow = wsId === null
  const cannotLoadHome = error || unavailable

  if (noHomeToShow && cannotLoadHome) {
    return (
      <div className="flex flex-1 flex-col items-center justify-center gap-1 text-sm text-muted-foreground">
        <span>Project Home unavailable</span>
        <span className="text-xs">Connect to this project's host to load it.</span>
      </div>
    )
  }
  return null
}
