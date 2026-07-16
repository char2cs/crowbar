import { createFileRoute, useParams } from '@tanstack/react-router'
import { useHomeWorkspaceState } from '@/features/workspace/lib/home-workspace-resolver'

/**
 * Project-home route. Renders nothing itself: IDEShell resolves the home
 * workspace id (`ensureHomeWorkspaceResolved`) and keeps its WorkspaceView
 * mounted-but-hidden in WorkspaceHost — the SAME keep-alive retention real
 * workspaces get — so returning to project home is a warm slot reveal
 * instead of a cold fetch+hydrate+mount every visit (see
 * features/workspace/lib/home-workspace-resolver.ts and ide-shell.tsx).
 * This route only surfaces the loading/error state; WorkspaceHost paints the
 * actual content in the same content pane, as an Outlet sibling.
 */
export function HomeRoute() {
  const { projectId } = useParams({ from: '/_shell/ide/$projectId/home' })
  const { error } = useHomeWorkspaceState(projectId)

  if (error) {
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
