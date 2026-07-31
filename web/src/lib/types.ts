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
  mergeConflicts: boolean
  parentBranch: string
  prUrl: string
  prTitle: string
  prTargetBranch: string
  /** On-disk worktree directory for this workspace (e.g. /home/user/project). */
  localPath?: string
  /** Worktree dir holding this branch when the workspace is a placeholder
   *  (locked + no localPath). Absent on healthy workspaces. */
  heldByPath?: string
  /** "home" for the project home workspace; absent or "git" for normal git workspaces. */
  kind?: 'git' | 'home'
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

/** Which agent wrote a review message, and out of which chat.
 *
 *  Both are omitempty on the wire and ABSENT far more often than present: every
 *  human message has neither, and so does every agent message written before
 *  attribution existed. A client renders exactly as it did before this pair when
 *  they are missing, and resolves them against the providers and chats it already
 *  holds when they are not — never branching on a particular id's value. */
interface AgentAttribution {
  providerId?: string
  chatId?: string
}

export interface ThreadReplyDTO extends AgentAttribution {
  id: string
  threadId: string
  body: string
  author: string
  isAgent: boolean
  createdAt: string
}

/** The root message's attribution rides on the THREAD, alongside its body, author
 *  and agent flag — the root is flattened onto the thread rather than appearing in
 *  `replies`, so without it an agent-opened thread would render attributed on every
 *  reply and anonymous on the finding itself. */
export interface ThreadDTO extends AgentAttribution {
  id: string
  projectId: string
  repoId: string
  workspaceId: string
  filePath: string
  line: number
  startLine: number
  endLine: number
  side: 'old' | 'new'
  /** Real id of the root comment (Messages[0]); lets the client edit the root. */
  messageId: string
  body: string
  author: string
  isAgent: boolean
  resolved: boolean
  createdAt: string
  replies: ThreadReplyDTO[]
  /** Tombstone flag on a broadcast frame: the thread was deleted, drop it. */
  deleted?: boolean
}

export interface TerminalSessionDTO {
  id: string
  projectId: string
  repoId: string
  workspaceId: string
  profileId: string
  status: 'active' | 'detached' | 'suspended' | 'ended'
  /** Process exit code; only present on "ended" frames where the exit code is known (>=0). */
  exitCode?: number
  createdAt: string
  endedAt: string | null
}
