import { createFileRoute, redirect } from '@tanstack/react-router'
import { fetchLandingWorkspaceId } from '@/lib/api'

export const Route = createFileRoute('/')({
  component: () => null,
  // Resolve a real workspace to land on instead of a hardcoded id. With no
  // workspaces yet, send the user to the projects view to import one. If the
  // lookup fails (backend down), fall through to projects rather than a dead
  // workspace route.
  beforeLoad: async () => {
    let wsId: string | null = null
    try {
      wsId = await fetchLandingWorkspaceId()
    } catch {
      wsId = null
    }
    if (wsId) {
      throw redirect({ to: '/workspaces/$wsId', params: { wsId } })
    }
    throw redirect({ to: '/projects' })
  },
})
