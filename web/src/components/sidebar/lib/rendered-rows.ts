import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { resolveRowRepo } from '@/components/sidebar/lib/sidebar-drop-policy'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useProjectDataStore, EMPTY_PROJECTS } from '@/lib/store/projects'
import { dataOf } from '@/lib/loadable'
import { useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { usePendingCreatesStore } from '@/lib/store/pending-creates'
import { hideRowsForInFlightCreates } from '@/components/sidebar/lib/rows-from-pending'
import {
  applyPendingRemovals,
  descendantHiddenIds,
  renderedHiddenIds,
} from '@/components/layout/removal-plan'
import { rowRepoScope } from '@/components/sidebar/lib/rows-from-repo'
import { rowsFromHome } from '@/components/sidebar/lib/rows-from-home'
import { compareSidebarRows } from '@/components/sidebar/lib/row-order'
import { rowsForProject } from '@/components/sidebar/lib/rows-for-project'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import {
  getHomeOwningChatId,
  getHomeWorkspaceId,
} from '@/features/workspace/lib/home-workspace-resolver'

/** Where a row lands among `rest`, relative to `targetId`. A stale/foreign
 *  `targetId` clamps to the end rather than refusing the drop. */
export function insertIndex(rest: string[], targetId: string, mode: 'before' | 'after'): number {
  const at = rest.indexOf(targetId)
  if (at < 0) return rest.length
  return mode === 'after' ? at + 1 : at
}

/**
 * The repos as the tree draws them — removal-tray-filtered, like
 * `SidebarTreeSurface` feeds `rowsFromRepo`.
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
  const tray = useRemovalTrayStore.getState()
  return applyPendingRemovals(
    useSidebarStore.getState().repos,
    renderedHiddenIds(tray.hiddenIds, tray.entries),
  )
}

/**
 * The exact rows one project's panel draws — `SpacePanel`'s own
 * `[...homeRows, ...repoRows]`. Every sibling index below is computed over
 * these, never over a per-workspace store (populated only while that
 * workspace's pane is mounted, so empty for the common repo-pane-open,
 * home-not-mounted drag — every reorder then wrote `order: 0`) or a hand-
 * merged chat/folder/repo list (which id'd a repo header by its workspace
 * while the row carries its owning chat's id, so the target was never found
 * and the index clamped to the end).
 */
export function renderedProjectRows(repos: readonly Repo[], projectId: string): SidebarRow[] {
  const homeWorkspaceId = getHomeWorkspaceId(projectId)
  const homeTree = homeWorkspaceId ? useHomeTreeStore.getState().trees[projectId] : undefined
  const hidden = descendantHiddenIds(useRemovalTrayStore.getState().entries)
  const rows = [
    ...(homeWorkspaceId && homeTree
      ? rowsFromHome(
          homeWorkspaceId,
          homeTree.chats.filter((c) => !hidden.has(c.id)),
          homeTree.folders.filter((f) => !hidden.has(f.id)),
          getHomeOwningChatId(projectId) ?? undefined,
        )
      : []),
    ...rowsForProject(repos, projectId),
  ]
  return [
    ...hideRowsForInFlightCreates(
      rows,
      usePendingCreatesStore.getState().entries,
      projectId,
      rowRepoScope(repos),
    ),
  ]
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
