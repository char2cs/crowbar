import { IS_MOBILE } from '@/utils/platform'
import { readSelect, selectRepos } from '@/core/entities/select'
import { createChatWithOwnWorktree } from '@/features/agent/api/agent-chat-creates'
import { createSurfaceFor } from '@/features/agent/api/agent-providers-api'
import { presetChatLandingPresentation } from '@/features/agent/hooks/use-chat-presentation'
import type { LandingChatPresentation } from '@/features/settings/lib/chat-presentation'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import { runOperation, whenSettled } from '@/core/entities/run-operation'
import { report } from '@/lib/errors/report'
import type { OperationItem } from '@/core/entities/operations'
import { toast } from '@/features/window/stores/toast-store'
import { resolveHomeRowScope } from '@/core/entities/home-rows'
import { renderedProjectRows } from '@/components/sidebar/lib/rendered-rows'
import { resolveHomeOwnerId } from '@/components/sidebar/lib/workspace-owners'
import {
  navigateThenOpenChat,
  openChatInOwnView,
  resolveChatRow,
  resolveRow,
  type NavigateFn,
} from './open-actions'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { newDraftChatId } from '@/features/agent/lib/draft-chat'
import { startHomeRowThread } from './home-actions'
import {
  createInFlight,
  enabledProvider,
  readUnreadHostProviders,
  startingProvider,
  startThread,
} from './thread-create'

export async function handleCreate(
  parentId: string,
  kind: 'workspace' | 'thread',
  navigate: NavigateFn,
  presentation?: LandingChatPresentation,
  branchName?: string,
): Promise<void> {
  const homeRow = resolveHomeRowScope(parentId)
  if (homeRow) {
    if (kind !== 'thread') return
    return startHomeRowThread(homeRow.projectId, homeRow.homeWorkspaceId, parentId, presentation)
  }

  const currentRepos = readSelect(selectRepos)
  const chatRow = resolveChatRow(currentRepos, parentId)
  if (chatRow && !chatRow.chat.workspaceId) return
  const found = resolveRow(currentRepos, chatRow?.chat.workspaceId ?? parentId)
  if (!found) return
  const { repo, subject } = found
  const { projectId } = repo
  if (!projectId) return

  if (kind === 'workspace') {
    const owningChatId = repo.workspaces.find((w) => w.id === subject.id)?.owningChatId
    const placementParentId =
      owningChatId ||
      (subject.id === repo.defaultWorkspaceId
        ? resolveHomeOwnerId(subject.id, repo.defaultOwningChatId, repo.chats ?? [])
        : parentId)
    const inFlightKey = `workspace:${placementParentId}`
    if (createInFlight.has(inFlightKey)) return
    const read = readUnreadHostProviders(projectId)
    if (read) {
      createInFlight.add(inFlightKey)
      const answered = await read.finally(() => createInFlight.delete(inFlightKey))
      if (!answered) return
    }
    if (!enabledProvider(projectId)) return
    const order = renderedProjectRows(projectId).filter(
      (r) => r.parentId === placementParentId,
    ).length
    treeUiStore.getState().startDraft({
      kind: 'branch',
      projectId,
      repoId: repo.id,
      workspaceId: subject.id,
      parentId: placementParentId,
      order,
      ownsWorktree: true,
    })
    if (branchName) confirmCreateDraft(branchName, navigate)
    return
  }

  const siblingRows = renderedProjectRows(projectId)
  const wsId =
    subject.kind === 'workspace'
      ? subject.id
      : (siblingRows.find((r) => r.id === subject.id)?.workspaceId ?? null)
  if (!wsId) {
    toast.error('Start a thread from a workspace row — a folder has none to run it in')
    return
  }
  const landsOnTerminal = !IS_MOBILE && createSurfaceFor(startingProvider(projectId)) === 'terminal'
  if (!chatRow && subject.kind === 'workspace' && presentation === undefined && !landsOnTerminal) {
    const draftId = newDraftChatId()
    if (!openChatInOwnView(draftId, wsId)) {
      await navigateThenOpenChat(navigate, { projectId, repoId: repo.id, wsId }, draftId)
    }
    windowPaneStore.getState().paneActions.tagDraftThread(draftId, parentId)
    return
  }
  return startThread({
    inFlightKey: `thread:${parentId}`,
    projectId,
    workspaceId: wsId,
    parentId,
    presentation,
    onCreated: (chatId) => {
      if (!openChatInOwnView(chatId, wsId)) {
        void navigateThenOpenChat(navigate, { projectId, repoId: repo.id, wsId }, chatId)
      }
    },
  })
}

export function confirmCreateDraft(name: string, navigate: NavigateFn): void {
  const draft = treeUiStore.getState().draft
  treeUiStore.getState().cancelDraft()
  if (draft?.kind !== 'branch' || !draft.repoId) return
  const { projectId, repoId, parentId } = draft
  const provider = enabledProvider(projectId)
  if (!provider) return
  const inFlightKey = `workspace:${parentId}`
  if (createInFlight.has(inFlightKey)) return
  createInFlight.add(inFlightKey)
  const chatId = crypto.randomUUID()
  const surface = createSurfaceFor(provider)
  const label = name || 'New workspace'
  const chat = (state: OperationItem['state']): OperationItem => ({
    ref: { kind: 'chat', id: chatId },
    label,
    state,
    parentId,
  })
  const op = runOperation(
    {
      kind: 'branch.create',
      subject: { kind: 'repo', id: repoId },
      projectId,
      label: `Create ${label}`,
      items: [chat('accepted')],
    },
    (opId) =>
      createChatWithOwnWorktree(
        chatId,
        opId,
        projectId,
        repoId,
        provider.id,
        parentId,
        name,
        surface,
      )
        .then((created) => {
          if (surface) presetChatLandingPresentation(chatId, surface)
          const workspace: OperationItem = {
            ref: { kind: 'workspace', id: created.workspaceId },
            label,
            state: 'succeeded',
          }
          return { items: [chat('succeeded'), workspace] }
        })
        .finally(() => createInFlight.delete(inFlightKey)),
  )
  whenSettled(op.id).then(
    (settled) => {
      const wsId = settled.items.find((item) => item.ref.kind === 'workspace')?.ref.id
      if (!wsId || openChatInOwnView(chatId, wsId)) return
      void navigateThenOpenChat(navigate, { projectId, repoId, wsId }, chatId)
    },
    (err: unknown) => report(err, { action: 'Create workspace' }),
  )
}

export function cancelCreateDraft(): void {
  treeUiStore.getState().cancelDraft()
}
