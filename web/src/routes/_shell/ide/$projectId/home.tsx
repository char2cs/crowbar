import { createFileRoute, useParams } from '@tanstack/react-router'
import { useHomeWorkspaceState } from '@/features/workspace/lib/home-workspace-resolver'
import { useHostUnavailable } from '@/lib/host-unavailability'

/**
 * Project-home route. Renders nothing itself: IDEShell resolves the home
 * workspace id (`ensureHomeWorkspaceResolved`) and keeps its WorkspaceView
 * mounted-but-hidden in WorkspaceHost — the SAME keep-alive retention real
 * workspaces get — so returning to project home is a warm slot reveal
 * instead of a cold fetch+hydrate+mount every visit (see
 * features/workspace/lib/home-workspace-resolver.ts and ide-shell.tsx).
 * This route only surfaces the loading/error state; WorkspaceHost paints the
 * actual content in the same content pane, as an Outlet sibling. The route
 * stays while the host its project may be on is down, and says so only when there is no
 * last-known home workspace to show.
 */
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
