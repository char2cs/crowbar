import { describe, it, expect, beforeEach } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import { createWorkflowSlice, type WorkflowSlice } from '@/features/workspace/stores/slices/workflow-slice'
import type { FlowDefinition } from '@/features/workflow/types/workflow'

const mockFlow: FlowDefinition = {
  flowId: 'flow-1',
  flowType: 'crowbar-default',
  steps: [
    { id: 'brainstorm', label: 'Brainstorm', contentType: 'chat', isCompleted: false, isActive: true },
    { id: 'spec', label: 'Spec', contentType: 'diff', isCompleted: false, isActive: false },
    { id: 'build', label: 'Build', contentType: 'split', isCompleted: false, isActive: false },
  ],
}

function makeStore() {
  return createStore<WorkflowSlice>()(immer((set, get) => createWorkflowSlice(set as any, get as any, {} as any)))
}

describe('workflow-slice', () => {
  let store: ReturnType<typeof makeStore>
  beforeEach(() => { store = makeStore() })

  it('starts with no flow and no step', () => {
    expect(store.getState().flowDefinition).toBeNull()
    expect(store.getState().currentStepId).toBeNull()
  })

  it('setFlowDefinition stores the definition', () => {
    store.getState().workflowActions.setFlowDefinition(mockFlow)
    expect(store.getState().flowDefinition?.flowId).toBe('flow-1')
    expect(store.getState().flowDefinition?.steps).toHaveLength(3)
  })

  it('setCurrentStep updates currentStepId', () => {
    store.getState().workflowActions.setFlowDefinition(mockFlow)
    store.getState().workflowActions.setCurrentStep('spec')
    expect(store.getState().currentStepId).toBe('spec')
  })

  it('markStepCompleted sets isCompleted on the step', () => {
    store.getState().workflowActions.setFlowDefinition(mockFlow)
    store.getState().workflowActions.markStepCompleted('brainstorm')
    const step = store.getState().flowDefinition?.steps.find(s => s.id === 'brainstorm')
    expect(step?.isCompleted).toBe(true)
  })
})
