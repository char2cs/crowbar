import { DISCARD_WORK_INIT, deleteProject, deleteRepo } from '@/lib/api'
import { workAtRiskOf } from '@/lib/errors/api-error'
import { deleteFolder, deleteHomeFolder } from '@/lib/api/sidebar-placement'
import { deleteChat } from '@/features/agent/api/agent-chat-commands'
import { getOwningChatId } from '@/lib/workspace-scope'
import { owningChatIdOfWorkspace } from '@/components/sidebar/lib/branch-row-id'
import {
  entityStore,
  forgetEntities,
  placeEntities,
  type EntityState,
} from '@/core/entities/entity-store'
import { readSelect, selectRepos } from '@/core/entities/select'
import { entityKey } from '@/core/stream/host-scope'
import { hostIdForProject } from '@/lib/host-resolve'
import { treeUiStore, type RemovalEntry } from '@/features/workspace/tree/tree-ui-store'
import { operationsState, putLocalOperation, type Operation } from '@/core/entities/operations'
import { dismissOperation, whenSettled, type OperationReply } from '@/core/entities/run-operation'
import { report } from '@/lib/errors/report'
import { toast } from '@/features/window/stores/toast-store'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import { holdsWorkspace } from '@/core/entities/repo-queries'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { UserFacingError } from '@/lib/errors/user-facing-error'

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

/** Whether any of `ids` is still a row the projects scope carries; a project counts while
 *  a repo of it does, since deleting it takes them. */
function stillPresent(s: EntityState, ids: readonly string[]): boolean {
  const { project, repo, workspace, chat } = s.byKind
  return ids.some(
    (id) =>
      id in project ||
      id in repo ||
      id in workspace ||
      id in chat ||
      Object.values(repo).some((r) => r.data.projectId === id),
  )
}

/**
 * Stop hiding the entry's rows once the daemon has actually taken them.
 *
 * The tray row goes the instant the request is sent, but the rows themselves
 * stay hidden across the round trip — releasing them with the request in flight
 * would flash every one of them back on screen until the tombstones arrive.
 */
function releaseWhenGone(entry: RemovalEntry): void {
  const ids = entry.hiddenIds
  const release = () => treeUiStore.getState().release(ids)
  if (!stillPresent(entityStore.getState(), ids)) {
    release()
    return
  }
  const stop = entityStore.subscribe((s) => {
    if (stillPresent(s, ids)) return
    stop()
    release()
  })
}

/** A repo or project delete is an operation: its rows come back when it fails,
 *  and the operations watcher has told why. */
function releaseWhenSettled(entry: RemovalEntry, opId: string): void {
  const release = () => treeUiStore.getState().release(entry.hiddenIds)
  whenSettled(opId).then(release, (err: unknown) => {
    release()
    report(err, { action: `Remove ${entry.label}` })
  })
}

/** Sends an operation's delete under a fresh key and holds its answer. */
async function sendOperation(send: (opId: string) => Promise<OperationReply>): Promise<string> {
  const opId = crypto.randomUUID()
  const reply = await send(opId)
  if ('operation' in reply) putLocalOperation(reply.operation)
  return opId
}

/** Sends the entry's delete; a repo or project delete resolves with its operation id. */
function sendRemoval(entry: RemovalEntry, init?: RequestInit): Promise<string | void> {
  // An entry carrying `atRisk` was confirmed with that list on screen: that
  // confirmation, and nothing else, is consent to destroy it.
  const sendInit = entry.atRisk ? { ...init, ...DISCARD_WORK_INIT } : init
  // Spread rather than pass `init` straight through: the ordinary commit has no
  // options at all, and handing every delete an explicit `undefined` would put
  // an argument on the wire-facing signature that only the unload flush uses.
  const opts: [RequestInit] | [] = sendInit ? [sendInit] : []
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
      // same union `deriveRows` renders the row from), and only then
      // from `workspace-scope.ts`'s side registry. That order is the fix: the
      // registry is a second copy of this fact, written on navigation and on
      // seed, and a workspace the user has only ever SEEN as a row — never
      // opened — could legitimately be absent from it. Asking it first turned
      // that absence into a rejected delete on a perfectly valid row, and since
      // the row had already been optimistically hidden, the removal looked like
      // it worked right up until the next reseed brought it back.
      const owningChatId =
        owningChatIdOfWorkspace(readSelect(selectRepos), entry.id) ?? getOwningChatId(entry.id)
      if (!owningChatId) {
        return Promise.reject(
          new UserFacingError('its chat is still loading — try again in a moment'),
        )
      }
      return deleteChat(entry.id, owningChatId, ...opts).then(announceTaken(entry))
    }
    case 'folder':
      // A project-home folder rides no repo and has its own route, scoped by project.
      return (
        entry.repoId
          ? deleteFolder(entry.projectId, entry.repoId, entry.id, ...opts)
          : deleteHomeFolder(entry.projectId, entry.id, ...opts)
      ).then((shifted) => {
        // The delete's answer applies at once, so the row does not flash back before its frame.
        const hostId = hostIdForProject(entry.projectId)
        forgetEntities('folder', [entityKey(hostId, entry.id)])
        placeEntities(
          'folder',
          Object.fromEntries(shifted.map((f) => [entityKey(hostId, f.id), f])),
        )
      })
    case 'repo':
      return sendOperation((opId) => deleteRepo(entry.projectId, entry.repoId, opId, ...opts))
    case 'project':
      // A space owns its content's lifetime (project-scoped panes §8, Zen's
      // `#deleteWorkspaceOwnedTabs`): its views go through the real
      // `closeView` teardown — every member's CLI stopped, every workspace
      // evicted — rather than being left parked where nothing can ever reach
      // or close them again. Before the DELETE, while the chats are still
      // resolvable.
      windowPaneStore.getState().paneActions.closeViewsForProject(entry.projectId)
      return sendOperation((opId) => deleteProject(entry.projectId, opId, ...opts))
    case 'chat':
      return deleteChat(entry.wsId, entry.id, ...opts).then(announceTaken(entry))
  }
}

/** The title of each chat as held before the delete, so a taken row is named. */
function chatTitles(): (id: string) => string {
  const chats = entityStore.getState().byKind.chat
  return (id) => chats[id]?.data.title || UNTITLED_CHAT_LABEL
}

/** Once a chat delete lands, names the chats its git cascade took beyond the selection,
 *  which the user never chose. */
function announceTaken(entry: RemovalEntry): (taken: string[]) => void {
  const titleOf = chatTitles()
  return (taken) => {
    const chosen = new Set(entry.hiddenIds)
    const extra = taken.filter((id) => !chosen.has(id))
    if (extra.length === 0) return
    const noun = extra.length === 1 ? 'chat' : 'chats'
    toast.info(`Also removed ${extra.length} ${noun}`, extra.map(titleOf).join(', '))
  }
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
  for (const entry of treeUiStore.getState().removals) {
    if (entry.deadlineAt === null) continue
    // Settled BEFORE the send, which makes this idempotent: the tray is drawn in
    // two places now (the workspace tree and the Chats panel), both are mounted
    // at once inside the sidebar carousel, and both register the pagehide
    // handler that calls this — so without it one unload would post every
    // pending delete twice.
    treeUiStore.getState().settle(entry.entryId)
    // No await: the document is unloading and there is nothing to roll back to;
    // a refusal still reaches the user if the page outlives it.
    sendRemoval(entry, { keepalive: true }).catch((err: unknown) =>
      report(err, { action: 'Remove' }),
    )
  }
}

/**
 * Fire the removal `entry` has been holding.
 *
 * The tray row leaves first: from the moment the request is out there is nothing
 * left to cancel, and a row that still offers Cancel would be lying. A refusal
 * puts the rows back and says why — this is the only path that can surface one,
 * because the user has already walked away from the gesture that started it.
 *
 * A refusal over work that exists nowhere else is a question, not a failure:
 * the entry goes back in the tray, still hidden, for the confirm dialog to list
 * that work and ask whether to delete it anyway.
 */
export async function commitRemoval(entry: RemovalEntry, context: RemovalContext): Promise<void> {
  treeUiStore.getState().settle(entry.entryId)

  let opId: string | void
  try {
    opId = await sendRemoval(entry)
  } catch (err) {
    const atRisk = workAtRiskOf(err)
    if (atRisk && !entry.atRisk) {
      treeUiStore.getState().askToDiscard(entry, atRisk)
      return
    }
    treeUiStore.getState().release(entry.hiddenIds)
    report(err, { action: `Couldn't remove ${entry.label}`, fallback: 'request failed' })
    return
  }

  if (opId) releaseWhenSettled(entry, opId)
  else releaseWhenGone(entry)
  dismissCreatesOf(entry.hiddenIds)
  leaveIfRemoved(entry, context)
}

// A partly failed create keeps its failed items' reasons until the user acts on them.
function allSucceeded(op: Operation): boolean {
  return op.items.every(
    (item) =>
      !item.error &&
      item.state !== 'failed' &&
      (item.state === 'succeeded' || op.state === 'succeeded'),
  )
}

/** A succeeded create's row stands in until its entity arrives; once the user
 *  removed that entity it never will, so the operation is settled history. */
function dismissCreatesOf(ids: readonly string[]): void {
  const removed = new Set(ids)
  for (const op of Object.values(operationsState().ops)) {
    if (op.dismissed || !allSucceeded(op)) continue
    if (op.items.some((item) => removed.has(item.ref.id)))
      void dismissOperation(op.id).catch((err: unknown) => report(err, { action: 'Dismiss' }))
  }
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

  const repos = readSelect(selectRepos)
  const fallbackRepo = entry.fallbackWsId
    ? repos.find((r) => holdsWorkspace(r, entry.fallbackWsId ?? ''))
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
