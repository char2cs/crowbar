// web/src/lib/types.ts
export type UIMode = 'chat' | 'diff'

export interface FlowStateDefinition {
  name: string    // machine name: 'brainstorming', 'ai_review'
  label: string   // display name: 'Brainstorm', 'AI Review'
  ui: UIMode
}

export interface FlowDefinition {
  name: string
  description: string
  states: FlowStateDefinition[]
}

export interface WorkspacePayload {
  id: string
  repoId: string
  branch: string
  flowName: string
  currentState: string
  flow: FlowDefinition
}

export interface ChatMessage {
  id: string
  role: 'user' | 'assistant'
  content: string
  authorName: string
  authorInitials: string
  modelName?: string
  timestamp: string
  toolCalls?: number
  durationSec?: number
}

export interface Project {
  id: string
  name: string
  path: string
  lastActivity: Date
}
