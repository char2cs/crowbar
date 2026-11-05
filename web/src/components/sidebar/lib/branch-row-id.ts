import type { Repo } from '@/core/entities/repo-types'

// A branch row is id'd by the chat that owns its workspace (the id the daemon resolves placements
// by). Owner: Workspace.owningChatId, else an ownsWorktree chat, else the repo home's
// defaultOwningChatId — read off the sidebar tree so delete and render share one source of truth.
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
  // '' is the root, never a row: an unowned workspace's owningChatId is '' too.
  if (!id) return null
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
