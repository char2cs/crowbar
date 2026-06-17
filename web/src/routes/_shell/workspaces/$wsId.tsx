import { useEffect } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { shouldRedirectUnknownWorkspace } from '@/lib/store/workspace-route-guard'

export const Route = createFileRoute('/_shell/workspaces/$wsId')({
  // IDEShell handles rendering via WorkspaceView; this component only guards
  // against unknown workspace ids (BUG-010).
  component: WorkspaceRouteGuard,
})

/**
 * BUG-010: an unknown workspace id in the URL (deleted workspace, stale link)
 * used to leave the app on an empty canvas firing endless 404/500 requests for
 * the dead id. Once the workspace list has loaded, redirect to the projects
 * page if the id is not present.
 */
function WorkspaceRouteGuard() {
  const { wsId } = Route.useParams()
  const navigate = useNavigate()
  const repos = useSidebarStore((state) => state.repos)
  const listStatus = useWorkspaceListStore((state) => state.data.status)

  const redirect = shouldRedirectUnknownWorkspace(listStatus, repos, wsId)

  useEffect(() => {
    if (redirect) void navigate({ to: '/projects', replace: true })
  }, [redirect, navigate])

  return null
}
