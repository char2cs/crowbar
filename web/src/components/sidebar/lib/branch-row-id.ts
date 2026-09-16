import type { Repo } from '@/lib/store/sidebar'

/**
 * The two id spaces a branch row straddles.
 *
 * `rows-from-repo.ts` gives EVERY workspace-owning row — a locked branch, a
 * repo/project home, and an ordinary fork or forked thread alike — the id of
 * the CHAT that owns its workspace, because that is the id the daemon
 * resolves a placement against — a row id'd from the `Workspace` names
 * something it has never heard of. Every verb that acts on such a row has to
 * come back the other way, and they do not share a dispatcher: the rename
 * gesture picks its endpoint by which id space the id falls in, while open,
 * trash and create go through `resolveRow`. Hence its own module rather than a
 * helper inside either — importing one from the other closes a cycle.
 *
 * `Workspace.owningChatId` is the authoritative direction for every ROW
 * workspace (`rows-from-repo.ts`'s own `resolveOwnerChats`) — a chat's own
 * `workspaceId` cannot serve here instead, since a thread carries its
 * parent's, not just the true owner's. The repo/project home is the one
 * exception: it is never a member of `repo.workspaces`, so there is no
 * `Workspace` record to read an `owningChatId` off of — `Repo.defaultOwningChatId`
 * (lifted from the SAME `WorkspaceDTO.owningChatId` field) stands in for it
 * there. Never `Chat.type`: Task 9 stopped minting/retyping a `'branch'`
 * chat to mark this row.
 */
/**
 * The other direction: the chat that owns `wsId`'s worktree, straight off the
 * sidebar store.
 *
 * `rows-from-repo.ts`'s `resolveOwnerChats`/`resolveHomeOwnerId` resolve, in
 * single-id form — `Workspace.owningChatId` (or, for the repo/project home,
 * `Repo.defaultOwningChatId`) first, then a `Chat` that claims the workspace
 * itself (`Chat.ownsWorktree`, which lands with the chat rather than with the
 * workspace and so can answer while the `Workspace` record is still in
 * flight).
 *
 * This exists so the DELETE path reads the same source of truth the RENDER path
 * does. It used to ask `workspace-scope.ts`'s `getOwningChatId` — a separate
 * module-level registry written by route parsing and by the sidebar store's own
 * seed. A second copy of "who owns this workspace" can only ever drift from the
 * first, and when it did, the delete rejected before it was sent: the row was
 * already optimistically hidden, so the removal LOOKED done, and the next
 * reseed put it straight back. Reading the tree the user is actually looking at
 * removes the drift instead of papering over it.
 */
export function owningChatIdOfWorkspace(repos: readonly Repo[], wsId: string): string | null {
  for (const repo of repos) {
    const ws = repo.workspaces.find((w) => w.id === wsId)
    if (ws?.owningChatId) return ws.owningChatId
    const owner = repo.chats?.find((c) => c.ownsWorktree && c.workspaceId === wsId)
    if (owner) return owner.id
    if (repo.defaultWorkspaceId === wsId) {
      if (repo.defaultOwningChatId) return repo.defaultOwningChatId
      const home = repo.chats?.find((c) => c.ownsWorktree && c.workspaceId === wsId)
      if (home) return home.id
    }
  }
  return null
}

export function workspaceIdOfBranchRow(repos: readonly Repo[], id: string): string | null {
  for (const repo of repos) {
    const ws = repo.workspaces.find((w) => w.owningChatId === id)
    if (ws) return ws.id
    if (repo.defaultWorkspaceId) {
      if (repo.defaultOwningChatId === id) return repo.defaultWorkspaceId
      const home = repo.chats?.find(
        (c) => c.id === id && c.ownsWorktree && c.workspaceId === repo.defaultWorkspaceId,
      )
      if (home) return repo.defaultWorkspaceId
    }
  }
  return null
}
