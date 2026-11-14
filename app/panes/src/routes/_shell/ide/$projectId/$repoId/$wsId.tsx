import { useEffect } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { selectProjectsSynced, selectRepos, useSelect } from '@/core/entities/select'
import { useOperations } from '@/core/entities/operations'
import { mergeRunsOn, shouldRedirectUnknownWorkspace } from '@/lib/store/workspace-route-guard'

export const Route = createFileRoute('/_shell/ide/$projectId/$repoId/$wsId')({
  component: WorkspaceRouteGuard,
})

function WorkspaceRouteGuard() {
  const { projectId, repoId, wsId } = Route.useParams()
  const navigate = useNavigate()
  const repos = useSelect(selectRepos)
  const synced = useSelect(selectProjectsSynced)
  const merging = useOperations((state) => mergeRunsOn(state.ops, wsId))

  // The IDE shell records this workspace's scope from the path before WorkspaceView renders
  // (recordWorkspaceScopeFromPath); this guard only redirects an unknown workspace.
  const redirect = shouldRedirectUnknownWorkspace(
    { projectId, repoId, wsId },
    { repos, synced, merging },
  )

  useEffect(() => {
    if (redirect) void navigate({ to: '/', replace: true })
  }, [redirect, navigate])

  return null
}
