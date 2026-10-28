import type { Repo } from '@/lib/store/sidebar'

/** A workspace as the user knows it: its branch, else its id. */
export function workspaceBranch(repos: readonly Repo[], workspaceId: string): string {
  for (const repo of repos) {
    const ws = repo.workspaces.find((w) => w.id === workspaceId)
    if (ws) return ws.branch
  }
  return workspaceId
}
