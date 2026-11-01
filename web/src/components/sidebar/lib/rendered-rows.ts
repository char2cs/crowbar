import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { resolveRowRepo } from '@/components/sidebar/lib/sidebar-drop-policy'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useProjectDataStore, EMPTY_PROJECTS } from '@/lib/store/projects'
import { dataOf } from '@/lib/loadable'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import { applyPendingRemovals, renderedHiddenIds } from '@/components/layout/removal-plan'
import { compareSidebarRows } from '@/components/sidebar/lib/row-order'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import { getHomeWorkspaceId } from '@/features/workspace/lib/home-workspace-resolver'
import { deriveRows } from '@/features/workspace/tree/rows'
import { treeInputFor } from '@/features/workspace/tree/input'

/** Where a row lands among `rest`, relative to `targetId`. A stale/foreign
 *  `targetId` clamps to the end rather than refusing the drop. */
export function insertIndex(rest: string[], targetId: string, mode: 'before' | 'after'): number {
  const at = rest.indexOf(targetId)
  if (at < 0) return rest.length
  return mode === 'after' ? at + 1 : at
}

/**
 * The repos as the tree draws them — removal-tray-filtered, like the tree input.
 *
 * Through `renderedHiddenIds`, not the tray store's raw `hiddenIds`: the two
 * differ by every LIVE entry's own primary row, which stays on screen
 * transformed in place (`RemovingSidebarRow`). Reading the raw set modelled a
 * tree the user is not looking at — a held FOLDER was gone from it with its
 * children re-homed to the folder's parent (`applyPendingRemovals`), while the
 * rendered tree still drew the folder with those children nested under it, so
 * every sibling index a drop computed during an eight-second hold was counted
 * over a different list than the one it was aimed at. The raw set is still the
 * base, so rows committed but not yet tombstoned stay hidden here exactly as
 * they are on screen.
 */
export function visibleRepos(): Repo[] {
  const tray = treeUiStore.getState()
  return applyPendingRemovals(
    useSidebarStore.getState().repos,
    renderedHiddenIds(tray.hiddenIds, tray.removals),
  )
}

/** The rows one project's panel draws from the daemon's data: a drop index counts these,
 *  never an operation's or a draft's stand-in row, which the daemon has not heard of. */
export function renderedProjectRows(projectId: string, repos?: readonly Repo[]): SidebarRow[] {
  return deriveRows(treeInputFor(projectId, repos)).filter((r) => !r.operation && !r.draft)
}

/** `containerId`'s members in the order `SidebarTree` draws them, minus the lifted rows. */
export function renderedSiblings(
  rows: readonly SidebarRow[],
  containerId: string,
  lifted: ReadonlySet<string>,
): string[] {
  return rows
    .filter((r) => (r.parentId ?? '') === containerId && !lifted.has(r.id))
    .sort(compareSidebarRows)
    .map((r) => r.id)
}

/** The project whose panel draws `wsId`'s rows — a repo workspace, or a project's home.
 *  Home is asked of the resolver (the source a Recents row's workspace is
 *  stamped from), across every project known, not only those with a tree loaded. */
export function projectOfWorkspace(repos: readonly Repo[], wsId: string): string | null {
  const scope = resolveRowRepo(repos, wsId)
  if (scope?.projectId) return scope.projectId
  const projectIds = new Set<string>([
    ...(dataOf(useProjectDataStore.getState().data) ?? EMPTY_PROJECTS).map((p) => p.id),
    ...Object.keys(useHomeTreeStore.getState().trees),
    ...repos.flatMap((r) => (r.projectId ? [r.projectId] : [])),
  ])
  for (const projectId of projectIds) {
    if (getHomeWorkspaceId(projectId) === wsId) return projectId
  }
  return null
}
