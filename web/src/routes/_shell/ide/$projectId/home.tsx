import { createFileRoute, useParams } from '@tanstack/react-router'
import { useHomeWorkspaceState } from '@/features/workspace/lib/home-workspace-resolver'
import { useHostUnavailable } from '@/lib/host-unavailability'

// Only the loading/error/host-down state: WorkspaceHost retains the home workspace like any other
// and paints it beside the Outlet, so returning home is a warm reveal, not a cold mount.
export function HomeRoute() {
  const { projectId } = useParams({ from: '/_shell/ide/$projectId/home' })
  const { error } = useHomeWorkspaceState(projectId)
  const unavailable = useHostUnavailable({ projectId })

  if (error || unavailable) {
    return (
      <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
        Project Home unavailable
      </div>
    )
  }
  return null
}

export const Route = createFileRoute('/_shell/ide/$projectId/home')({
  component: HomeRoute,
})
