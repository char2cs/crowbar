import { createFileRoute, useParams } from '@tanstack/react-router'
import { useHomeWorkspaceState } from '@/features/workspace/lib/home-workspace-resolver'
import { useHostUnavailable } from '@/lib/host-unavailability'

// Only the loading/error/host-down state: WorkspaceHost retains the home workspace like any other
// and paints it beside the Outlet, so returning home is a warm reveal. A host that is down is
// named only when no home workspace is known to show.
export function HomeRoute() {
  const { projectId } = useParams({ from: '/_shell/ide/$projectId/home' })
  const { wsId, error } = useHomeWorkspaceState(projectId)
  const unavailable = useHostUnavailable({ projectId })

  if (wsId === null && (error || unavailable)) {
    return (
      <div className="flex flex-1 flex-col items-center justify-center gap-1 text-sm text-muted-foreground">
        <span>Project Home unavailable</span>
        <span className="text-xs">Connect to this project's host to load it.</span>
      </div>
    )
  }
  return null
}

export const Route = createFileRoute('/_shell/ide/$projectId/home')({
  component: HomeRoute,
})
