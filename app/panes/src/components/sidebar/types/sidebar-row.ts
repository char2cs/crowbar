import type { WorkspaceStatus } from '@/core/entities/repo-types'
import type { OperationState } from '@/core/entities/operations'
import type { Problem } from '@/lib/errors/problem'

type SidebarRowKind = 'chat' | 'branch' | 'folder' | 'workflow'

export interface SidebarRow {
  id: string
  kind: SidebarRowKind
  parentId: string | null
  order: number
  createdAt?: string
  label: string
  labelProvisional?: boolean
  ownsWorktree: boolean
  workspaceId: string | null
  canBranchOut?: boolean
  working: boolean
  hasView: boolean
  branchName?: string
  added?: number
  deleted?: number
  locked?: boolean
  status?: WorkspaceStatus
  isPlaceholder?: boolean
  needsProvisioning?: boolean
  heldByPath?: string
  placeholderReason?: string
  repoIcon?: {
    repoId: string
    projectId: string
    name: string
    avatarLabel: string
    avatarColor: string
    avatarURL?: string
  }
  operation?: {
    opId: string
    state: OperationState
    problem: Problem | null
  }
  draft?: true
  removal?: {
    entryId: string
    deadlineAt: number | null
    extra: number
  }
}
