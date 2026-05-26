import { useFlowDefinition, useCurrentStepId, useWorkflowActions } from '../stores/hooks/use-workflow'
import { usePaneActions, useActivePaneId } from '../stores/hooks/use-pane-store'
import { useBuffers } from '../stores/hooks/use-buffer-store'
import type { CrowbarChatContent } from '@/features/panes/types/pane-content'

export function WorkspaceStepFooter() {
  const flowDefinition = useFlowDefinition()
  const currentStepId = useCurrentStepId()
  const { setCurrentStep } = useWorkflowActions()
  const activePaneId = useActivePaneId()
  const paneActions = usePaneActions()
  const buffers = useBuffers()

  if (!flowDefinition) return null

  function handleStepClick(stepId: string) {
    setCurrentStep(stepId)

    const chatBuffer = buffers.find(b => b.type === 'crowbarChat') as CrowbarChatContent | undefined
    if (chatBuffer) {
      const activePane = paneActions.getActivePane()
      if (activePane?.activeBufferId !== chatBuffer.id) {
        const chatPane = paneActions.getPaneByBufferId(chatBuffer.id)
        if (chatPane) {
          paneActions.activatePaneBuffer(chatPane.id, chatBuffer.id)
          paneActions.setActivePane(chatPane.id)
        } else {
          paneActions.addBufferToPane(activePaneId, chatBuffer.id, true)
        }
      }
    }
  }

  return (
    <div className="flex h-8 shrink-0 items-center gap-1 border-t border-border bg-card px-2">
      {flowDefinition.steps.map(step => (
        <button
          key={step.id}
          onClick={() => handleStepClick(step.id)}
          className={[
            'flex h-6 items-center gap-1.5 rounded px-2 text-xs font-medium transition-colors',
            step.id === currentStepId
              ? 'bg-primary text-primary-foreground'
              : 'text-muted-foreground hover:bg-accent hover:text-foreground',
            step.isCompleted && step.id !== currentStepId ? 'opacity-60' : '',
          ].join(' ')}
        >
          {step.isCompleted && step.id !== currentStepId && <span className="text-[10px]">✓</span>}
          {step.label}
        </button>
      ))}
    </div>
  )
}
