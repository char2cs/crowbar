import { GitBranchIcon } from 'lucide-react'
import { createFileRoute, redirect } from '@tanstack/react-router'
import { fetchLandingWorkspaceId, fetchProjects } from '@/lib/api'
import {
  Empty,
  EmptyMedia,
  EmptyHeader,
  EmptyTitle,
  EmptyDescription,
} from '@/components/ui/empty'

function NoReposScreen() {
  return (
    <Empty>
      <EmptyMedia variant="icon">
        <GitBranchIcon />
      </EmptyMedia>
      <EmptyHeader>
        <EmptyTitle>No repositories yet</EmptyTitle>
        <EmptyDescription>Add a git repository to open a workspace.</EmptyDescription>
      </EmptyHeader>
    </Empty>
  )
}

export const Route = createFileRoute('/_shell/')({
  component: NoReposScreen,
  beforeLoad: async () => {
    // Run both fetches in parallel so we don't serialize two API calls on cold start
    const [projectsResult, wsIdResult] = await Promise.allSettled([
      fetchProjects(),
      fetchLandingWorkspaceId(),
    ])

    // Only treat empty list as "no projects" — propagate actual errors
    if (projectsResult.status === 'rejected') {
      throw projectsResult.reason
    }
    const projects = projectsResult.value

    if (projects.length === 0) {
      throw redirect({ to: '/oobe' })
    }

    const wsId = wsIdResult.status === 'fulfilled' ? wsIdResult.value : null

    if (wsId) {
      throw redirect({ to: '/workspaces/$wsId', params: { wsId } })
    }
    // Has projects but no active workspace — render NoReposScreen
  },
})
