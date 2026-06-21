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

// ---------------------------------------------------------------------------
// Canonical DTOs (§5) — camelCase mirrors of the Go `api/v0/dto` contracts.
// These are the single source of truth the IndexedDB entity cache stores and
// the hierarchical read API returns. Additive: the legacy flat types above stay
// in place until the W17/W18 cutover.
// ---------------------------------------------------------------------------

export type WorkspaceStatusDTO =
  | 'new'
  | 'locked'
  | 'pr-conflicts'
  | 'deleted'
  | 'pr-merged'
  | 'pr-open'
  | 'pr-closed'

export interface WorkspaceDTO {
  id: string
  repoId: string
  projectId: string
  branch: string
  parentId: string
  forkPointSha: string
  status: WorkspaceStatusDTO
  working: boolean
  lastError: string
  isDefault?: boolean
  added: number
  deleted: number
  mergeStrategy: string
  canMergeLocally: boolean
  parentBranch: string
  prUrl: string
  prTitle: string
  prTargetBranch: string
}

export interface RepoDTO {
  id: string
  projectId: string
  name: string
  path: string
  defaultBranch: string
  avatarLabel: string
  avatarColor: string
  avatarUrl: string // proxied /v0/projects/:p/repos/:r/icon endpoint
  avatarEmoji: string
}

export interface ProjectDTO {
  id: string
  name: string
  path: string
  status: string // "" | "deleted"
  lastActivity: string
}

export interface ThreadReplyDTO {
  id: string
  threadId: string
  body: string
  author: string
  isAgent: boolean
  createdAt: string
}

export interface ThreadDTO {
  id: string
  projectId: string
  repoId: string
  workspaceId: string
  filePath: string
  line: number
  startLine: number
  endLine: number
  side: 'old' | 'new'
  body: string
  author: string
  isAgent: boolean
  resolved: boolean
  createdAt: string
  replies: ThreadReplyDTO[]
}

export interface TerminalSessionDTO {
  id: string
  projectId: string
  repoId: string
  workspaceId: string
  profileId: string
  status: 'active' | 'ended'
  createdAt: string
  endedAt: string | null
}
