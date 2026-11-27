import type { ChatType, WorkspaceProvisioning } from '@/lib/types'

export type WorkspaceStatus =
  'new' | 'locked' | 'pr-conflicts' | 'deleted' | 'pr-merged' | 'pr-open' | 'pr-closed'

export interface Folder {
  id: string
  repoId: string
  parentId?: string
  name: string
  order: number
  createdAt?: string
}

export interface Chat {
  id: string
  repoId: string
  type?: ChatType
  parentId?: string
  workspaceId?: string
  ownsWorktree?: boolean
  title: string
  order: number
  createdAt?: string
}

export interface Workspace {
  id: string
  branch: string
  parentId?: string
  folderId?: string
  order?: number
  createdAt?: string
  status?: WorkspaceStatus
  added?: number
  deleted?: number
  age: string
  working?: boolean
  canMergeLocally?: boolean
  mergeConflicts?: boolean
  parentBranch?: string
  prUrl?: string
  localPath?: string
  heldByPath?: string
  provisioning?: WorkspaceProvisioning
  owningChatId?: string
}

export interface Repo {
  id: string
  projectId?: string
  order?: number
  folderId?: string
  name: string
  avatarLabel: string
  avatarColor: string
  avatarURL?: string
  workspaces: Workspace[]
  folders?: Folder[]
  chats?: Chat[]
  defaultWorkspaceId?: string
  defaultBranch?: string
  defaultWorkspaceStatus?: WorkspaceStatus
  defaultOwningChatId?: string
  defaultWorking?: boolean
  localPath?: string
  deleting?: boolean
}
