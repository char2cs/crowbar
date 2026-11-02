import type { Repo, Workspace } from '@/core/entities/repo-types'
import { EMPTY_CHATS } from '@/core/entities/views-from-entities'
import { placeholderKind, placeholderReason } from '@/lib/workspace/placeholder'
import type { Problem } from '@/lib/errors/problem'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { resolveOwnerChats, resolveOwnerOfChat } from '@/components/sidebar/lib/workspace-owners'

/**
 * The icon-relevant subset of `SidebarRow` — what `RowGlyph` (sidebar-row.tsx)
 * actually reads to pick a glyph. `chatIconIndex` (below) is the one producer;
 * named here so `recents-band.tsx` can spread it onto its own hand-built row
 * without repeating the field list.
 */
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

/**
 * The `Workspace`-derived subset of a `branch` row's icon fields — exactly
 * the fields `deriveRows`'s workspace-node branch stamps below,
 * factored out so `chatIconIndex` (Recents' own icon fallback, below) can
 * produce the identical shape without re-deriving it.
 *
 * `ownDefaultBranch` is the owning repo's own `defaultBranch` — the one fact a
 * `Workspace` cannot tell about itself and the whole of what separates "the
 * repo's own main folder has this branch" from "Crowbar could not set this
 * branch up" (`placeholderKind`'s own doc).
 */
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

/**
 * The icon-relevant fields `RowGlyph` (sidebar-row.tsx) needs to draw a
 * workspace-owning chat's real branch/lock/PR-status glyph, keyed by chat id
 * across every repo passed in — the SAME ownership fold
 * (`resolveOwnerChats`/`resolveOwnerOfChat`) and `Workspace` fields this
 * file's own tree walk draws its glyph from, reused rather than re-derived.
 *
 * `recents-band.tsx` (via `recents-for-project.ts`) reads this instead of
 * hand-building a row that can only ever guess `kind: 'chat'` — Recents had
 * no ownership data of its own, so a chat that owns a workspace always fell
 * back to the generic `ChatsCircle` bubble glyph there, even though the
 * identical chat draws the real branch/lock/PR mark in the tree.
 *
 * A chat id absent from the returned map owns no workspace and renders as an
 * ordinary chat bubble — the caller's own existing default already gets that
 * case right, so this only ever holds entries worth overriding.
 */
export function chatIconIndex(repos: readonly Repo[]): Map<string, ChatIconFields> {
  const index = new Map<string, ChatIconFields>()
  for (const repo of repos) {
    const chats = (repo.chats ?? EMPTY_CHATS).filter((c) => c.repoId === repo.id)
    const workspaces = repo.workspaces.filter((w) => w.status !== 'deleted')
    const ownerChats = resolveOwnerChats(workspaces, chats)
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
