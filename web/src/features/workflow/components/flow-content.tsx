// web/src/features/workflow/components/flow-content.tsx
import { useCurrentStep } from '@/features/workspace/stores/hooks/use-workflow'
import { MarkdownChatView } from '@/features/markdown-chat/components/markdown-chat-view'
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
        <MarkdownChatView workspaceId={workspaceId} stepId={currentStep.id} />
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
