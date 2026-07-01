import { useEffect } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { shouldRedirectUnknownWorkspace } from '@/lib/store/workspace-route-guard'

export const Route = createFileRoute('/_shell/ide/$projectId/$repoId/$wsId')({
  component: WorkspaceRouteGuard,
})

function WorkspaceRouteGuard() {
  const { wsId } = Route.useParams()
  const navigate = useNavigate()
  const repos = useSidebarStore((state) => state.repos)
  const listStatus = useWorkspaceListStore((state) => state.data.status)

  // The hierarchical scope (project+repo) for this workspace is recorded
  // synchronously by the IDE shell from the route path BEFORE WorkspaceView
  // renders (see recordWorkspaceScopeFromPath); this guard only handles the
  // unknown-workspace redirect.
  const redirect = shouldRedirectUnknownWorkspace(listStatus, repos, wsId)

  useEffect(() => {
    if (redirect) void navigate({ to: '/', replace: true })
  }, [redirect, navigate])

  return null
}
