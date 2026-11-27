import type { Repo } from '@/core/entities/repo-types'

export function owningChatIdOfWorkspace(repos: readonly Repo[], wsId: string): string | null {
  for (const repo of repos) {
    const ws = repo.workspaces.find((w) => w.id === wsId)
    if (ws?.owningChatId) return ws.owningChatId
    const owner = repo.chats?.find((c) => c.ownsWorktree && c.workspaceId === wsId)
    if (owner) return owner.id
    if (repo.defaultWorkspaceId === wsId && repo.defaultOwningChatId) {
      return repo.defaultOwningChatId
    }
  }
  return null
}

export function workspaceIdOfBranchRow(repos: readonly Repo[], id: string): string | null {
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
