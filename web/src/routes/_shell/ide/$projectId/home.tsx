import { useEffect, useState } from 'react'
import { createFileRoute, useParams } from '@tanstack/react-router'
import { WorkspaceView } from '@/features/workspace/components/workspace-view'
import { fetchHomeWorkspace } from '@/lib/api'
import { setWorkspaceScope } from '@/lib/workspace-scope'

export function HomeRoute() {
  const { projectId } = useParams({ from: '/_shell/ide/$projectId/home' })
  const [wsId, setWsId] = useState<string | null>(null)
  const [error, setError] = useState(false)

  useEffect(() => {
    let cancelled = false
    fetchHomeWorkspace(projectId)
      .then((ws) => {
        if (!cancelled) setWsId(ws.id)
      })
      .catch(() => {
        if (!cancelled) setError(true)
      })
    return () => {
      cancelled = true
    }
  }, [projectId])

  if (error) {
    return (
      <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
        Project Home unavailable
      </div>
    )
  }
  if (!wsId) return null

  // Record the workspace scope before WorkspaceView renders — home workspaces
  // have no repoId, so workspaceBase() uses the /home API path instead.
  setWorkspaceScope({ projectId, repoId: '', wsId })
  return <WorkspaceView wsId={wsId} />
}

export const Route = createFileRoute('/_shell/ide/$projectId/home')({
  component: HomeRoute,
})
