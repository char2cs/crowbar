import type { Repo, Workspace } from '@/core/entities/repo-types'
import { EMPTY_CHATS } from '@/core/entities/views-from-entities'
import { placeholderKind, placeholderReason } from '@/lib/workspace/placeholder'
import type { Problem } from '@/lib/errors/problem'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { resolveOwnerChats, resolveOwnerOfChat } from '@/components/sidebar/lib/workspace-owners'

/** The SidebarRow fields RowGlyph reads, so view-row.tsx can spread them onto its own row. */
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

// A branch row's Workspace-derived icon fields, shared by deriveRows and chatIconIndex.
// `ownDefaultBranch` is the one fact a Workspace can't tell about itself: it separates the repo's
// own checkout from a branch Crowbar couldn't set up (see placeholderKind).
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

// Icon fields per workspace-owning chat across `repos`, from the same ownership fold the tree
// uses, so Recents draws the real branch/lock/PR glyph rather than a generic bubble. A chat
// absent from the map owns no workspace and keeps the caller's default chat glyph.
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
