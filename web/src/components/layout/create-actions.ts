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
import { draftLanding, finishDraftThread, type DraftPrompt, type DraftSpec } from './draft-thread'
import { startHomeRowThread } from './home-actions'
import {
  createInFlight,
  enabledProvider,
  readUnreadHostProviders,
  startingProvider,
  startThread,
} from './thread-create'

// The sidebar's "+" on a row: a fork (named first, then minted) or a thread.

/** Creates a fork ('workspace') or a thread under `parentId`: a fork opens a
 *  naming input at its final slot, a thread is drawn from its operation at once.
 *  A project-home row can only thread: home has no worktree to fork. */
export async function handleCreate(
  parentId: string,
  kind: 'workspace' | 'thread',
  navigate: NavigateFn,
  /** The surface the new thread is created and landed on; undefined takes
   *  the user's default (`createSurfaceFor`). Never applies to a fork. */
  presentation?: LandingChatPresentation,
  /** A fork's branch name, already asked for (the phone); otherwise it is named inline. */
  branchName?: string,
): Promise<void> {
  const homeRow = resolveHomeRowScope(parentId)
  if (homeRow) {
    if (kind !== 'thread') return
    return startHomeRowThread(homeRow.projectId, homeRow.homeWorkspaceId, parentId, presentation)
  }

  const currentRepos = readSelect(selectRepos)
  // A bubble's Fork/Thread resolve against its GROUND workspace
  // (`Chat.workspaceId`); with no ground at all there is nothing to act on.
  const chatRow = resolveChatRow(currentRepos, parentId)
  if (chatRow && !chatRow.chat.workspaceId) return
  const found = resolveRow(currentRepos, chatRow?.chat.workspaceId ?? parentId)
  if (!found) return
  const { repo, subject } = found
  const { projectId } = repo
  if (!projectId) return

  if (kind === 'workspace') {
    // The daemon places by CHAT id. A branch row's id already is one; a regular fork's row is id'd
    // by its workspace, so read the owner off the record. Falls back to the clicked id for the repo
    // home, a folder, or a frame with no owner yet.
    const owningChatId = repo.workspaces.find((w) => w.id === subject.id)?.owningChatId
    // A bubble's ground workspace can ALSO be the repo home — never in
    // `repo.workspaces` — so it resolves the way the home row's own id was rendered.
    const placementParentId =
      owningChatId ||
      (subject.id === repo.defaultWorkspaceId
        ? resolveHomeOwnerId(subject.id, repo.defaultOwningChatId, repo.chats ?? [])
        : parentId)
    // One fork in flight per parent; a remote host's providers are read on first use.
    const inFlightKey = `workspace:${placementParentId}`
    if (createInFlight.has(inFlightKey)) return
    const read = readUnreadHostProviders(projectId)
    if (read) {
      // Held while the host is asked, so a burst of clicks asks (and fails) once.
      createInFlight.add(inFlightKey)
      const answered = await read.finally(() => createInFlight.delete(inFlightKey))
      if (!answered) return
    }
    if (!enabledProvider(projectId)) return
    // EVERY sibling counts, any kind: the daemon appends a fork by counting all
    // rows under the same parent (owning_chat.go's placeOwningRow).
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
    // The phone asks for the name first, so there is nothing to type inline.
    if (branchName) confirmCreateDraft(branchName, navigate)
    return
  }

  // A thread runs in a real workspace: a `workspace` subject is one; a folder
  // takes its nearest owning workspace, stamped on its row at build time.
  const siblingRows = renderedProjectRows(projectId)
  const wsId =
    subject.kind === 'workspace'
      ? subject.id
      : (siblingRows.find((r) => r.id === subject.id)?.workspaceId ?? null)
  if (!wsId) {
    toast.error('Start a thread from a workspace row — a folder has none to run it in')
    return
  }
  // A plain new thread on a workspace is a draft: no chat exists until its first send. A draft
  // is a Chat composer, so with Terminal the default it starts on the terminal like a fork.
  const landsOnTerminal = createSurfaceFor(startingProvider(projectId)) === 'terminal'
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

/** Sends the fork being named as a local operation, whose row stands in (or keeps
 *  the daemon's reason) until its chat and workspace arrive; then the chat opens. */
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

/** Closes the naming input; nothing was sent, so nothing rolls back. */
export function cancelCreateDraft(): void {
  treeUiStore.getState().cancelDraft()
}

/** Creates the chat a repo-workspace draft was waiting to become, with its first prompt. */
export function promoteRepoDraft(draft: DraftSpec, prompt: DraftPrompt): Promise<void> {
  const { projectId, workspaceId } = draft
  return startThread({
    inFlightKey: `draft:${draft.draftId}`,
    projectId,
    workspaceId,
    parentId: draft.parentId || workspaceId,
    ...draftLanding(prompt),
    onCreated: (chatId) => finishDraftThread(draft, chatId, prompt),
  })
}
