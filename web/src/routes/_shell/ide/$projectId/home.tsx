import { createFileRoute, useParams } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { WorkspaceView } from '@/features/workspace/components/workspace-view'
import { fetchHomeWorkspace } from '@/lib/api'

export function HomeRoute() {
  const { projectId } = useParams({ from: '/_shell/ide/$projectId/home' })
  const { data, isPending, isError } = useQuery({
    queryKey: ['home-workspace', projectId],
    queryFn: () => fetchHomeWorkspace(projectId),
  })

  if (isPending) return null
  if (isError || !data) {
    return (
      <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
        Project Home unavailable
      </div>
    )
  }

  return <WorkspaceView wsId={data.id} />
}

export const Route = createFileRoute('/_shell/ide/$projectId/home')({
  component: HomeRoute,
})
