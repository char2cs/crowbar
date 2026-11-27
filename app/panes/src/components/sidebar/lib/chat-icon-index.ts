import type { Repo, Workspace } from '@/core/entities/repo-types'
import { EMPTY_CHATS } from '@/core/entities/views-from-entities'
import { placeholderKind, placeholderReason } from '@/lib/workspace/placeholder'
import type { Problem } from '@/lib/errors/problem'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { resolveOwnerChats, resolveOwnerOfChat } from '@/components/sidebar/lib/workspace-owners'

export type ChatIconFields = Pick<
  SidebarRow,
  | 'kind'
  | 'ownsWorktree'
  | 'branchName'
  | 'added'
  | 'deleted'
  | 'locked'
  | 'status'
  | 'isPlaceholder'
  | 'needsProvisioning'
  | 'heldByPath'
  | 'placeholderReason'
>

export function workspaceIconFields(
  workspace: Workspace,
  ownDefaultBranch: string | undefined,
  problem: Problem | null,
): Omit<ChatIconFields, 'kind' | 'ownsWorktree'> {
  const kind = placeholderKind(workspace, ownDefaultBranch)
  return {
    branchName: workspace.branch,
    added: workspace.added,
    deleted: workspace.deleted,
    locked: workspace.status === 'locked',
    status: workspace.status,
    isPlaceholder: kind !== 'none',
    needsProvisioning: kind === 'unprovisioned',
    heldByPath: kind === 'none' ? undefined : workspace.heldByPath || undefined,
    placeholderReason: placeholderReason(workspace, kind, problem) || undefined,
  }
}

export function chatIconIndex(repos: readonly Repo[]): Map<string, ChatIconFields> {
  const index = new Map<string, ChatIconFields>()
  for (const repo of repos) {
    const chats = (repo.chats ?? EMPTY_CHATS).filter((c) => c.repoId === repo.id)
    const workspaces = repo.workspaces.filter((w) => w.status !== 'deleted')
    const ownerChats = resolveOwnerChats(workspaces)
    const ownerOfChat = resolveOwnerOfChat(ownerChats, chats)
    const workspaceById = new Map(workspaces.map((w) => [w.id, w]))
    for (const [chatId, wsId] of ownerOfChat) {
      const workspace = workspaceById.get(wsId)
      index.set(chatId, {
        kind: 'branch',
        ownsWorktree: true,
        ...(workspace ? workspaceIconFields(workspace, repo.defaultBranch, null) : {}),
      })
    }
  }
  return index
}
