import { useEffect } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { shouldRedirectUnknownWorkspace } from '@/lib/store/workspace-route-guard'
import { setWorkspaceScope } from '@/features/workspace/stores/workspace-store-registry'

export const Route = createFileRoute('/_shell/ide/$projectId/$repoId/$wsId')({
  component: WorkspaceRouteGuard,
})

function WorkspaceRouteGuard() {
  const { projectId, repoId, wsId } = Route.useParams()
  const navigate = useNavigate()
  const repos = useSidebarStore((state) => state.repos)
  const listStatus = useWorkspaceListStore((state) => state.data.status)

  // §3/§7: record this workspace's hierarchical scope so the wsId-keyed
  // files/git/terminal/editor callers resolve the full project/repo path.
  useEffect(() => {
    setWorkspaceScope({ projectId, repoId, wsId })
  }, [projectId, repoId, wsId])

  const redirect = shouldRedirectUnknownWorkspace(listStatus, repos, wsId)

  useEffect(() => {
    if (redirect) void navigate({ to: '/', replace: true })
  }, [redirect, navigate])

  return null
}
