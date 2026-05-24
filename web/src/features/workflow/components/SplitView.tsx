import { ChatView } from './ChatView'
import { DiffView } from './DiffView'

interface SplitViewProps {
  workspaceId: string
  stepId: string
}

export function SplitView({ workspaceId, stepId }: SplitViewProps) {
  return (
    <div className="flex h-full overflow-hidden">
      <div className="flex-1 min-w-0 border-r border-border overflow-hidden">
        <ChatView workspaceId={workspaceId} stepId={stepId} />
      </div>
      <div className="flex-1 min-w-0 overflow-hidden">
        <DiffView workspaceId={workspaceId} stepId={stepId} />
      </div>
    </div>
  )
}
