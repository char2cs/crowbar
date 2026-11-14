import type { Repo, Workspace } from '@/core/entities/repo-types'

/** The ids deleting `wsId` removes: it and its descendants, skipping locked subtrees. */
function collectDeletedIds(allWorkspaces: Workspace[], wsId: string): Set<string> {
  // Index once so the BFS below is O(n) instead of an array.find()/filter()
  // per queued id (this list is every workspace across every repo).
  const byId = new Map(allWorkspaces.map((w) => [w.id, w]))
  const childrenByParentId = new Map<string, Workspace[]>()
  for (const w of allWorkspaces) {
    if (!w.parentId) continue
    const siblings = childrenByParentId.get(w.parentId)
    if (siblings) siblings.push(w)
    else childrenByParentId.set(w.parentId, [w])
  }

  const toDelete = new Set<string>()
  const queue = [wsId]
  while (queue.length > 0) {
    const id = queue.shift()!
    if (toDelete.has(id)) continue
    const ws = byId.get(id)
    if (ws?.status === 'locked') continue
    toDelete.add(id)
    for (const child of childrenByParentId.get(id) ?? []) {
      queue.push(child.id)
    }
  }
  return toDelete
}

// Locked worktrees refuse every daemon write (409), so mutation UI gates on this. Checks the tree
// rows and the default workspace, which is no row (`repo.defaultWorkspaceId`/`Status`) and is
// locked whenever an adopted protected branch is.
export function isWorkspaceLockedInSidebar(repos: Repo[], wsId: string | null): boolean {
  if (!wsId) return false
  for (const repo of repos) {
    if (repo.defaultWorkspaceId === wsId) return repo.defaultWorkspaceStatus === 'locked'
    const ws = repo.workspaces.find((w) => w.id === wsId)
    if (ws) return ws.status === 'locked'
  }
  return false
}

/** Whether `repo` holds `wsId`; its home (default) workspace is not in `workspaces`. */
export function holdsWorkspace(repo: Repo, wsId: string): boolean {
  return repo.defaultWorkspaceId === wsId || repo.workspaces.some((w) => w.id === wsId)
}

/** After deleting the active `wsId` (or an ancestor of it): its surviving parent, else the repo's
 *  locked base, else any survivor, else null (the caller goes to the projects page). */
export function getPostDeleteNavigationTarget(repos: Repo[], wsId: string): string | null {
  const repo = repos.find((r) => r.workspaces.some((w) => w.id === wsId))
  if (!repo) return null
  const ws = repo.workspaces.find((w) => w.id === wsId)!
  const deleted = collectDeletedIds(
    repos.flatMap((r) => r.workspaces),
    wsId,
  )
  if (ws.parentId && !deleted.has(ws.parentId)) return ws.parentId
  const survivors = repo.workspaces.filter((w) => !deleted.has(w.id))
  const base = survivors.find((w) => w.status === 'locked')
  return (base ?? survivors[0])?.id ?? null
}
