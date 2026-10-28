import type { Repo } from '@/lib/store/sidebar'

/**
 * Names for workspaces shown side by side: each one's branch, qualified with
 * its repo where two share a branch, and its id when it has no branch.
 */
export function workspaceNames(
  repos: readonly Repo[],
  workspaceIds: readonly string[],
): Map<string, string> {
  const wanted = new Set(workspaceIds)
  const found = new Map<string, { repo: string; branch: string }>()
  for (const repo of repos) {
    for (const ws of repo.workspaces) {
      if (ws.branch && wanted.has(ws.id)) found.set(ws.id, { repo: repo.name, branch: ws.branch })
    }
  }
  const branchCount = new Map<string, number>()
  for (const { branch } of found.values())
    branchCount.set(branch, (branchCount.get(branch) ?? 0) + 1)

  return new Map(
    workspaceIds.map((id) => {
      const ws = found.get(id)
      if (!ws) return [id, id]
      return [id, (branchCount.get(ws.branch) ?? 0) > 1 ? `${ws.repo}/${ws.branch}` : ws.branch]
    }),
  )
}
