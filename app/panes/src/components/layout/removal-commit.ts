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

export interface RemovalNavigate {
  (target: { projectId: string; repoId: string; wsId: string } | null): void
}

export interface RemovalContext {
  activeWorkspaceId: string
  navigate: RemovalNavigate
}

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

function releaseWhenSettled(entry: RemovalEntry, opId: string): void {
  const release = () => treeUiStore.getState().release(entry.hiddenIds)
  whenSettled(opId).then(release, (err: unknown) => {
    release()
    report(err, { action: `Remove ${entry.label}` })
  })
}

async function sendOperation(send: (opId: string) => Promise<OperationReply>): Promise<string> {
  const opId = crypto.randomUUID()
  const reply = await send(opId)
  if ('operation' in reply) putLocalOperation(reply.operation)
  return opId
}

function sendRemoval(entry: RemovalEntry, init?: RequestInit): Promise<string | void> {
  const sendInit = entry.atRisk ? { ...init, ...DISCARD_WORK_INIT } : init
  const opts: [RequestInit] | [] = sendInit ? [sendInit] : []
  switch (entry.kind) {
    case 'workspace': {
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
      return (
        entry.repoId
          ? deleteFolder(entry.projectId, entry.repoId, entry.id, ...opts)
          : deleteHomeFolder(entry.projectId, entry.id, ...opts)
      ).then((shifted) => {
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
      windowPaneStore.getState().paneActions.closeViewsForProject(entry.projectId)
      return sendOperation((opId) => deleteProject(entry.projectId, opId, ...opts))
    case 'chat':
      return deleteChat(entry.wsId, entry.id, ...opts).then(announceTaken(entry))
  }
}

function chatTitles(): (id: string) => string {
  const chats = entityStore.getState().byKind.chat
  return (id) => chats[id]?.data.title || UNTITLED_CHAT_LABEL
}

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

export function flushDrainingRemovals(): void {
  for (const entry of treeUiStore.getState().removals) {
    if (entry.deadlineAt === null) continue
    treeUiStore.getState().settle(entry.entryId)
    sendRemoval(entry, { keepalive: true }).catch((err: unknown) =>
      report(err, { action: 'Remove' }),
    )
  }
}

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

function allSucceeded(op: Operation): boolean {
  return op.items.every(
    (item) =>
      !item.error &&
      item.state !== 'failed' &&
      (item.state === 'succeeded' || op.state === 'succeeded'),
  )
}

function dismissCreatesOf(ids: readonly string[]): void {
  const removed = new Set(ids)
  for (const op of Object.values(operationsState().ops)) {
    if (op.dismissed || !allSucceeded(op)) continue
    if (op.items.some((item) => removed.has(item.ref.id)))
      void dismissOperation(op.id).catch((err: unknown) => report(err, { action: 'Dismiss' }))
  }
}

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
