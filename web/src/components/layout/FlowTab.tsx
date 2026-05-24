import { Outlet, useNavigate, useRouterState } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { workspaceQueryOptions } from '@/lib/queries'
import { WorkspaceStepTabs } from './WorkspaceStepTabs'

interface FlowTabProps {
  workspaceId: string
}

export function FlowTab({ workspaceId }: FlowTabProps) {
  const navigate = useNavigate()
  const routerState = useRouterState()
  const pathname = routerState.location.pathname
  const { data: workspace } = useQuery(workspaceQueryOptions(workspaceId))

  const currentStep = pathname.split('/').pop() ?? workspace?.currentState ?? ''

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      {/* Route content — ChatView or DiffView rendered by the router outlet */}
      <div className="flex flex-1 flex-col overflow-hidden">
        <Outlet />
      </div>

      {/* WorkspaceStepTabs pinned below ChatInput (ChatInput is inside Outlet/ChatView) */}
      {workspace && (
        <div className="border-t border-border">
          <WorkspaceStepTabs
            states={workspace.flow.states}
            currentStep={currentStep}
            onStepChange={(step: string) =>
              void navigate({ to: '/workspaces/$wsId/$step', params: { wsId: workspaceId, step } })
            }
          />
        </div>
      )}
    </div>
  )
}
