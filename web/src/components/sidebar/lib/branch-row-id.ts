import type { Repo } from '@/lib/store/sidebar'

/**
 * The two id spaces a branch row straddles.
 *
 * `rows-from-repo.ts` gives a locked branch and a repo home the id of the
 * `branch` CHAT that owns their workspace, because that is the id the daemon
 * resolves a placement against — a row id'd from the `Workspace` names
 * something it has never heard of. Every verb that acts on such a row has to
 * come back the other way, and they do not share a dispatcher: the rename
 * gesture picks its endpoint by which id space the id falls in, while open,
 * trash and create go through `resolveRow`. Hence its own module rather than a
 * helper inside either — importing one from the other closes a cycle.
 */
export function workspaceIdOfBranchRow(repos: readonly Repo[], id: string): string | null {
  for (const repo of repos) {
    const row = repo.chats?.find((c) => c.id === id && c.type === 'branch')
    if (row?.workspaceId) return row.workspaceId
  }
  return null
}
