import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'
import type { FlowDefinition } from '@/features/workflow/types/workflow'

export interface WorkflowActions {
  setFlowDefinition(def: FlowDefinition): void
  setCurrentStep(stepId: string): void
  markStepCompleted(stepId: string): void
}

export interface WorkflowSlice {
  flowDefinition: FlowDefinition | null
  currentStepId: string | null
  workflowActions: WorkflowActions
}

export const createWorkflowSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  WorkflowSlice
> = (set) => ({
  flowDefinition: null,
  currentStepId: null,

  workflowActions: {
    setFlowDefinition(def) {
      set(state => {
        state.flowDefinition = def
        if (!state.currentStepId) {
          state.currentStepId = def.steps[0]?.id ?? null
        }
      })
    },
    setCurrentStep(stepId) {
      set(state => { state.currentStepId = stepId })
    },
    markStepCompleted(stepId) {
      set(state => {
        const step = state.flowDefinition?.steps.find(s => s.id === stepId)
        if (step) step.isCompleted = true
      })
    },
  },
})
