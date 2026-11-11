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

// Committing a hold: the one step of the removal path that destroys anything. Before this the
// rows were only hidden; here the daemon is told, and the tray has nothing left to undo.

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

/** Unhides the entry's rows only once the daemon has taken them; releasing with the request in
 *  flight would flash them back on screen until the tombstones arrive. */
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
      // A worktree goes by deleting the CHAT that holds it (DELETE .../chats/:id cascades the
      // teardown). Owner read off the sidebar tree first: the scope registry misses never-opened
      // rows. Rejected, not thrown, so `flushDrainingRemovals`'s `.catch` still catches it.
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
      // A space owns its views' lifetime (§8): close them through the real teardown, BEFORE the
      // DELETE while their chats still resolve, or they would be parked where nothing can reach.
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

// On unload, send every removal still draining on its clock: the hidden row LOOKS removed, so
// dropping it would be a lost write. Clockless repo/project holds await an answer and are dropped.
// `keepalive` keeps a pagehide request from being cancelled with the document (64KB body cap).
export function flushDrainingRemovals(): void {
  for (const entry of treeUiStore.getState().removals) {
    if (entry.deadlineAt === null) continue
    // Settled BEFORE the send so this is idempotent: every mounted tray registers a pagehide
    // handler, and one unload must not post a delete twice.
    treeUiStore.getState().settle(entry.entryId)
    // No await: the document is unloading and there is nothing to roll back to;
    // a refusal still reaches the user if the page outlives it.
    sendRemoval(entry, { keepalive: true }).catch((err: unknown) =>
      report(err, { action: 'Remove' }),
    )
  }
}

// Fires the held removal. The tray row leaves first (nothing is cancellable once sent); a refusal
// puts the rows back and reports it. A refusal over unsaved work instead re-holds the entry so the
// confirm dialog can ask whether to discard it.
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

/** Leaves a route whose workspace was just removed, using the fallback resolved at hold time,
 *  while the tree still had the row. */
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
