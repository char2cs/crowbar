import type { FlowDefinition } from '@/lib/types'

export const FEATURE_DEV_FLOW: FlowDefinition = {
  name: 'feature-development',
  description: 'Full feature development — brainstorm to reviewed implementation',
  states: [
    { name: 'brainstorming',  label: 'Brainstorm',    ui: 'chat' },
    { name: 'spec',           label: 'Spec',           ui: 'chat' },
    { name: 'implementation', label: 'Build',          ui: 'chat' },
    { name: 'ai_review',      label: 'AI Review',      ui: 'diff' },
    { name: 'human_review',   label: 'Human Review',   ui: 'diff' },
  ],
}

export const MOCK_FLOWS: FlowDefinition[] = [FEATURE_DEV_FLOW]
