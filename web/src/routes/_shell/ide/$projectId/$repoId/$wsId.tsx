import { useEffect } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import { useOperations } from '@/lib/store/operations'
import { mergeRunsOn, shouldRedirectUnknownWorkspace } from '@/lib/store/workspace-route-guard'

export const Route = createFileRoute('/_shell/ide/$projectId/$repoId/$wsId')({
  component: WorkspaceRouteGuard,
})

function WorkspaceRouteGuard() {
  const { projectId, repoId, wsId } = Route.useParams()
  const navigate = useNavigate()
  const repos = useSidebarStore((state) => state.repos)
  const workspacesRead = useFolderSignalStore((state) => state.seededWorkspaceRepoIds)
  const reposRead = useFolderSignalStore((state) => state.seededRepoListProjectIds)
  const merging = useOperations((state) => mergeRunsOn(state.ops, wsId))

  // The hierarchical scope (project+repo) for this workspace is recorded
  // synchronously by the IDE shell from the route path BEFORE WorkspaceView
  // renders (see recordWorkspaceScopeFromPath); this guard only handles the
  // unknown-workspace redirect.
  const redirect = shouldRedirectUnknownWorkspace(
    { projectId, repoId, wsId },
    { repos, workspacesRead, reposRead, merging },
  )

  useEffect(() => {
    if (redirect) void navigate({ to: '/', replace: true })
  }, [redirect, navigate])

  return null
}
