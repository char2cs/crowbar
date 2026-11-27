export interface Project {
  id: string
  name: string
  path: string
  lastActivity: Date
  order?: number
  avatarUrl?: string
  avatarEmoji?: string
  hostId: string
}

type WorkspaceStatusDTO =
  'new' | 'locked' | 'pr-conflicts' | 'deleted' | 'pr-merged' | 'pr-open' | 'pr-closed'

export interface WorkspaceDTO {
  id: string
  repoId: string
  projectId: string
  branch: string
  parentId: string
  forkPointSha: string
  status: WorkspaceStatusDTO
  working: boolean
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
  localPath?: string
  heldByPath?: string
  provisioning: WorkspaceProvisioning
  kind?: 'git' | 'home'
  folderId?: string
  order?: number
  createdAt?: string
  owningChatId?: string
}

export type WorkspaceProvisioning = 'provisioned' | 'placeholder' | 'shared'

export interface RepoDTO {
  id: string
  projectId: string
  name: string
  path: string
  defaultBranch: string
  avatarLabel: string
  avatarColor: string
  avatarUrl: string
  avatarEmoji: string
  order?: number
  folderId?: string
  deleting?: boolean
}

export interface FolderDTO {
  id: string
  repoId: string
  projectId: string
  parentId?: string
  name: string
  order: number
  createdAt?: string
  status?: string
}

export type ChatType = 'chat' | 'workflow'

export interface ChatDTO {
  id: string
  repoId: string
  projectId: string
  type?: ChatType
  workspaceId: string
  ownsWorktree?: boolean
  parentId?: string
  title: string
  order: number
  createdAt?: string
}

export interface ProjectDTO {
  id: string
  name: string
  path: string
  status: string
  lastActivity: string
  order?: number
  avatarUrl?: string
  avatarEmoji?: string
}

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
  messageId: string
  body: string
  author: string
  isAgent: boolean
  resolved: boolean
  createdAt: string
  replies: ThreadReplyDTO[]
}
