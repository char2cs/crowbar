import { createFileRoute, redirect } from '@tanstack/react-router'
import { fetchLandingWorkspaceId, fetchProjects } from '@/lib/api'

export const Route = createFileRoute('/_shell/')({
  component: () => null,
  beforeLoad: async () => {
    let projects: { id: string }[] = []
    try {
      projects = await fetchProjects()
    } catch {
      projects = []
    }

    if (projects.length === 0) {
      throw redirect({ to: '/oobe' })
    }

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
