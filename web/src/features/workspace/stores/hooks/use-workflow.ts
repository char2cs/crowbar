import { useWorkspaceStoreContext } from '../workspace-context'
import type { WorkflowActions } from '../slices/workflow-slice'
import type { FlowDefinition, FlowStep } from '@/features/workflow/types/workflow'
import type { ChatMessage } from '@/lib/types'

const EMPTY_MESSAGES: ChatMessage[] = []

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

/** Returns the persisted chat messages for a specific step. */
export const useChatMessages = (stepId: string): ChatMessage[] =>
  useWorkspaceStoreContext(s => s.chatMessages[stepId] ?? EMPTY_MESSAGES)

/** Returns whether the given step has an in-flight response. */
export const useIsSending = (stepId: string): boolean =>
  useWorkspaceStoreContext(s => s.sendingByStep[stepId] ?? false)
