import { deleteRepo, deleteWorkspace } from '@/lib/api'
import { deleteFolder } from '@/lib/api/sidebar-placement'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useRemovalTrayStore, type RemovalEntry } from '@/lib/store/sidebar-removal'
import { toast } from '@/features/window/stores/toast-store'

/**
 * Committing a hold — the one step of the removal path that destroys anything.
 *
 * Everything before this is reversible by construction: the rows were hidden,
 * not deleted, and Cancel puts the ids back. Here the daemon is finally told,
 * and from this point the tray has nothing left to undo.
 */

/** Where a committed removal sends you when it took the workspace you were in. */
export interface RemovalNavigate {
  (target: { projectId: string; repoId: string; wsId: string } | null): void
}

/** What the caller knows that this module cannot ask for itself. */
export interface RemovalContext {
  /** The workspace the editor is showing, or '' on a route that has none. */
  activeWorkspaceId: string
  navigate: RemovalNavigate
}

/** Whether any of `ids` is still a row the sidebar knows about. */
function stillPresent(repos: Repo[], ids: readonly string[]): boolean {
  return ids.some(
    (id) =>
      repos.some((r) => r.id === id) || repos.some((r) => r.workspaces.some((w) => w.id === id)),
  )
}

/**
 * Stop hiding the entry's rows once the daemon has actually taken them.
 *
 * The tray row goes the instant the request is sent, but the rows themselves
 * stay hidden across the round trip — releasing them with the request in flight
 * would flash every one of them back on screen for as long as it takes the
 * tombstones to arrive.
 */
function releaseWhenGone(ids: readonly string[]): void {
  const release = () => useRemovalTrayStore.getState().release(ids)
  if (!stillPresent(useSidebarStore.getState().repos, ids)) {
    release()
    return
  }
  const unsubscribe = useSidebarStore.subscribe((state) => {
    if (stillPresent(state.repos, ids)) return
    unsubscribe()
    release()
  })
}

function sendRemoval(entry: RemovalEntry): Promise<void> {
  switch (entry.kind) {
    case 'workspace':
      return deleteWorkspace(entry.projectId, entry.repoId, entry.id)
    case 'folder':
      return deleteFolder(entry.projectId, entry.repoId, entry.id)
    case 'repo':
      return deleteRepo(entry.projectId, entry.repoId)
  }
}

/**
 * Fire the removal `entry` has been holding.
 *
 * The tray row leaves first: from the moment the request is out there is nothing
 * left to cancel, and a row that still offers Cancel would be lying. A refusal
 * puts the rows back and says why — this is the only path that can surface one,
 * because the user has already walked away from the gesture that started it.
 */
export async function commitRemoval(entry: RemovalEntry, context: RemovalContext): Promise<void> {
  useRemovalTrayStore.getState().settle(entry.entryId)

  try {
    await sendRemoval(entry)
  } catch (err) {
    useRemovalTrayStore.getState().release(entry.hiddenIds)
    toast.error(
      `Couldn't remove ${entry.label}: ${err instanceof Error ? err.message : 'request failed'}`,
    )
    return
  }

  releaseWhenGone(entry.hiddenIds)
  leaveIfRemoved(entry, context)
}

/**
 * Leave a route whose workspace has just been removed.
 *
 * The fallback was resolved when the row was held, against a tree that still
 * had it — by now the answer is gone from the tree, which is the whole reason
 * it is carried on the entry.
 */
function leaveIfRemoved(
  entry: RemovalEntry,
  { activeWorkspaceId, navigate }: RemovalContext,
): void {
  if (!activeWorkspaceId || !entry.hiddenIds.includes(activeWorkspaceId)) return

  const repos = useSidebarStore.getState().repos
  const fallbackRepo = entry.fallbackWsId
    ? repos.find((r) => r.workspaces.some((w) => w.id === entry.fallbackWsId))
    : undefined
  if (!fallbackRepo?.projectId || !entry.fallbackWsId) {
    navigate(null)
    return
  }
  navigate({
    projectId: fallbackRepo.projectId,
    repoId: fallbackRepo.id,
    wsId: entry.fallbackWsId,
  })
}
