import { deleteProject, deleteRepo } from '@/lib/api'
import { deleteFolder } from '@/lib/api/sidebar-placement'
import { deleteChat } from '@/features/agent/api/agent-api'
import { getOwningChatId } from '@/lib/workspace-scope'
import { owningChatIdOfWorkspace } from '@/components/sidebar/lib/branch-row-id'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
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

/**
 * Whether any of `ids` is still a row the sidebar knows about.
 *
 * Projects are matched through the repos that carry their id rather than through
 * the project list: this module watches the sidebar store, and a deleted project
 * takes its repos with it, so the last repo leaving IS the project being gone.
 * A project that held no repos has nothing here to wait on and releases at once,
 * which is correct — there was never a row to flash back.
 */
function stillPresent(repos: Repo[], ids: readonly string[]): boolean {
  return ids.some(
    (id) =>
      repos.some((r) => r.id === id || r.projectId === id) ||
      repos.some((r) => r.workspaces.some((w) => w.id === id)) ||
      // Chats: same reseed channel workspaces/repos ride, checked so a
      // drag-to-trashed chat's row stays hidden across the round trip
      // instead of flashing back the instant the DELETE resolves.
      repos.some((r) => r.chats?.some((c) => c.id === id)),
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

function sendRemoval(entry: RemovalEntry, init?: RequestInit): Promise<void> {
  // Spread rather than pass `init` straight through: the ordinary commit has no
  // options at all, and handing every delete an explicit `undefined` would put
  // an argument on the wire-facing signature that only the unload flush uses.
  const opts: [RequestInit] | [] = init ? [init] : []
  switch (entry.kind) {
    case 'workspace': {
      // A worktree is taken by deleting the CHAT that holds it: DELETE
      // .../chats/:id now cascades the worktree teardown, so this is the same
      // destruction the workspace route did, addressed by the only id a route
      // may name. No fallback to a workspace route — that group is gone.
      // Rejected rather than thrown, so `flushDrainingRemovals`'s `.catch` on
      // an unloading page still catches it.
      //
      // Resolved from the SIDEBAR TREE first (`owningChatIdOfWorkspace` — the
      // same union `rows-from-repo.ts` renders the row from), and only then
      // from `workspace-scope.ts`'s side registry. That order is the fix: the
      // registry is a second copy of this fact, written on navigation and on
      // seed, and a workspace the user has only ever SEEN as a row — never
      // opened — could legitimately be absent from it. Asking it first turned
      // that absence into a rejected delete on a perfectly valid row, and since
      // the row had already been optimistically hidden, the removal looked like
      // it worked right up until the next reseed brought it back.
      const owningChatId =
        owningChatIdOfWorkspace(useSidebarStore.getState().repos, entry.id) ??
        getOwningChatId(entry.id)
      if (!owningChatId) {
        return Promise.reject(new Error(`no owning chat recorded for workspace ${entry.id}`))
      }
      return deleteChat(entry.id, owningChatId, ...opts).then(() => {
        bumpRepoTree(entry.repoId)
      })
    }
    case 'folder':
      // Folders carry no dedicated push channel any more (Task 34). `stillPresent`
      // above never checks `r.folders` at all, so `releaseWhenGone` below always
      // finds a folder id already absent and releases the tray row IMMEDIATELY —
      // it never subscribes for a folder kind, so there is no hang to worry about
      // here. The real problem this guards against: the row is still sitting in
      // `useSidebarStore` (nothing else removes it any more), so an immediate
      // release without applying the tombstone would let it flash right back on
      // screen the instant the tray hides it. Apply the tombstone (and the
      // promotion shift the delete triggers) straight off the DELETE's own
      // response, the same way row-actions.ts's writes do — and `bump` the repo's
      // folder signal too, so the `crowbar_folders` cache every tree rebuild reads
      // from agrees, or the deleted folder comes BACK on the next unrelated
      // rebuild (see row-actions.ts's performRenameFolder for the full story).
      return deleteFolder(entry.projectId, entry.repoId, entry.id, ...opts).then((shifted) => {
        const apply = useSidebarStore.getState().applyFolderDTO
        apply({
          id: entry.id,
          repoId: entry.repoId,
          projectId: entry.projectId,
          name: '',
          order: 0,
          status: 'deleted',
        })
        shifted.forEach(apply)
        useFolderSignalStore.getState().bump(entry.repoId)
      })
    case 'repo':
      return deleteRepo(entry.projectId, entry.repoId, ...opts)
    case 'project':
      return deleteProject(entry.projectId, ...opts)
    case 'chat':
      // No local tombstone (unlike folder's own special case): a chat DOES
      // arrive on a real push/reseed channel. But that channel has a condition
      // — see `bumpRepoTree` — and this is one of the surfaces that does not
      // meet it, so the acting client rings its own bell.
      return deleteChat(entry.wsId, entry.id, ...opts).then(() => {
        bumpRepoTree(entry.repoId)
      })
  }
}

/**
 * Tell this repo's sidebar tree to re-read its rows.
 *
 * `app-sync-provider.tsx`'s `openRepoTreeSubscription` reseeds `crowbar_chats`
 * on exactly one trigger: this repo's generation in `useFolderSignalStore`
 * moving. The only thing that normally moves it is
 * `use-workspace-agent-chats-stream.ts`, on a structural chat frame — and that
 * hook runs only for a MOUNTED workspace. App-sync's own comment records the
 * assumption that made that acceptable: "a chat can only be created, renamed or
 * moved from a surface that has that workspace mounted."
 *
 * That assumption is no longer true, and this is the correction. The sidebar is
 * on screen on every route (including project home, where no repo workspace is
 * mounted at all), it deletes and creates chats, and it does so for rows whose
 * workspace the user has never opened. Without this bump the daemon really did
 * take the chat and the row simply stayed on screen — which reads as "I deleted
 * these chats and they came back", because the optimistic hide releases as soon
 * as the WORKSPACE half tombstones and the surviving chat row paints again.
 *
 * The same reasoning `row-actions.ts` already applies to every folder and chat
 * RENAME it fires (`performRenameChat`, `performRenameFolder`,
 * `performCreateFolder`, `performSetWorkspaceLock` all bump); create and delete
 * were the two verbs left out.
 */
function bumpRepoTree(repoId: string): void {
  if (repoId) useFolderSignalStore.getState().bump(repoId)
}

/**
 * Send every removal that was only waiting on its clock, because the page is
 * going away.
 *
 * The tray holds a row for eight seconds before the delete is sent, and the tray
 * is memory. Anything that ends the page inside that window — a reload, an HMR
 * update, quitting the app — used to drop the intent silently: the row had
 * already been hidden, so the removal LOOKED done, and the next boot read it
 * straight back off the daemon. That is not an undo, it is a lost write, and the
 * user has no way to tell the two apart.
 *
 * Only the draining entries go. A repo or a project sits in the tray with no
 * clock, waiting on an explicit answer, and an unload is not that answer — those
 * are dropped, which is the safe direction for the two removals that cascade.
 *
 * `keepalive` is what makes this work at all: a request issued from a pagehide
 * handler is normally cancelled with the document. It caps the body at 64KB,
 * which a DELETE with no body is comfortably inside.
 */
export function flushDrainingRemovals(): void {
  for (const entry of useRemovalTrayStore.getState().entries) {
    if (entry.deadlineAt === null) continue
    // Settled BEFORE the send, which makes this idempotent: the tray is drawn in
    // two places now (the workspace tree and the Chats panel), both are mounted
    // at once inside the sidebar carousel, and both register the pagehide
    // handler that calls this — so without it one unload would post every
    // pending delete twice.
    useRemovalTrayStore.getState().settle(entry.entryId)
    // No await and no catch: the document is unloading, there is nobody left to
    // tell and nothing left to roll back to.
    void sendRemoval(entry, { keepalive: true }).catch(() => {})
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
