import { useWorkspaceStoreContext } from '../workspace-context'
import type { WorkflowActions } from '../slices/workflow-slice'
import type { FlowDefinition, FlowStep } from '@/features/workflow/types/workflow'

export const useFlowDefinition = (): FlowDefinition | null =>
  useWorkspaceStoreContext(s => s.flowDefinition)

export const useCurrentStepId = (): string | null =>
  useWorkspaceStoreContext(s => s.currentStepId)

export const useCurrentStep = (): FlowStep | null =>
  useWorkspaceStoreContext(s => {
    if (!s.flowDefinition || !s.currentStepId) return null
    return s.flowDefinition.steps.find(step => step.id === s.currentStepId) ?? null
  })

export const useWorkflowActions = (): WorkflowActions =>
  useWorkspaceStoreContext(s => s.workflowActions)
