// web/src/features/workflow/types/workflow.ts

export type StepContentType = 'chat' | 'diff' | 'split'

export interface FlowStep {
  id: string
  label: string
  icon?: string
  contentType: StepContentType
  isCompleted: boolean
  isActive: boolean
}

export interface FlowDefinition {
  flowId: string
  /** Identifies the flow template, e.g. 'crowbar-default'. Extensible to any flow type. */
  flowType: string
  steps: FlowStep[]
}
