import { useCurrentStep } from '@/features/workspace/stores/hooks/use-workflow'
import { ChatView } from './chat-view'
import { DiffView } from './diff-view'
import { SplitView } from './split-view'

interface FlowContentProps {
  workspaceId: string
}

export function FlowContent({ workspaceId }: FlowContentProps) {
  const currentStep = useCurrentStep()

  if (!currentStep) {
    return (
      <div className="flex h-full items-center justify-center text-muted-foreground">
        <p className="text-sm">No active step</p>
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col overflow-hidden">
      {currentStep.contentType === 'chat' && (
        <ChatView workspaceId={workspaceId} stepId={currentStep.id} />
      )}
      {currentStep.contentType === 'diff' && (
        <DiffView workspaceId={workspaceId} stepId={currentStep.id} />
      )}
      {currentStep.contentType === 'split' && (
        <SplitView workspaceId={workspaceId} stepId={currentStep.id} />
      )}
    </div>
  )
}
