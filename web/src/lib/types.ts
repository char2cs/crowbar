// web/src/lib/types.ts
export interface WorkspacePayload {
  id: string
  projectId: string
  repoId: string
  branch: string
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

export interface Prerequisites {
  git: { installed: boolean; version?: string }
  gh: { installed: boolean; authed: boolean }
  glab: { installed: boolean; authed: boolean }
}
