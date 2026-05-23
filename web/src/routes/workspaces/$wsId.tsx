import { createFileRoute, Outlet, useNavigate, useRouterState } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { workspaceQueryOptions } from '@/lib/queries'
import { WorkspaceStepTabs } from '@/components/layout/WorkspaceStepTabs'

export const Route = createFileRoute('/workspaces/$wsId')({
  component: WorkspaceLayout,
})

function WorkspaceLayout() {
  const { wsId } = Route.useParams()
  const navigate = useNavigate()
  const pathname = useRouterState({ select: s => s.location.pathname })
  const { data: workspace } = useQuery(workspaceQueryOptions(wsId))

  if (!workspace) {
    return (
      <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
        Loading…
      </div>
    )
  }

  const currentStep = pathname.split('/').pop() ?? workspace.currentState

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      <WorkspaceStepTabs
        states={workspace.flow.states}
        currentStep={currentStep}
        onStepChange={(step) =>
          navigate({ to: '/workspaces/$wsId/$step', params: { wsId, step } })
        }
      />
      <Outlet />
    </div>
  )
}
