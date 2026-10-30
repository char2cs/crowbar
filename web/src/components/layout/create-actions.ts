import { useSidebarStore } from '@/lib/store/sidebar'
import { createChatWithOwnWorktree } from '@/features/agent/api/agent-chat-creates'
import type { LandingChatPresentation } from '@/features/settings/lib/chat-presentation'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import { useCreateDraftStore } from '@/lib/store/create-draft'
import { runOperation, whenSettled } from '@/lib/store/run-operation'
import { report } from '@/lib/errors/report'
import type { OperationItem } from '@/lib/store/operations'
import { toast } from '@/features/window/stores/toast-store'
import { resolveHomeRowScope } from '@/lib/store/home-tree'
import { rowsFromRepo } from '@/components/sidebar/lib/rows-from-repo'
import { resolveHomeOwnerId } from '@/components/sidebar/lib/workspace-owners'
import {
  navigateThenOpenChat,
  openChatInOwnView,
  resolveChatRow,
  resolveRow,
  type NavigateFn,
} from './open-actions'
import { startHomeRowThread } from './home-actions'
import { createInFlight, enabledProvider, startThread } from './thread-create'

// The sidebar's "+" on a row: a fork (named first, then minted) or a thread.

/**
 * Creates a fork ('workspace') or a thread under `parentId`: a fork opens a
 * naming input at its final slot, a thread is drawn from its operation at once.
 * A project-home row can only thread: home has no worktree to fork.
 */
export function handleCreate(
  parentId: string,
  kind: 'workspace' | 'thread',
  navigate: NavigateFn,
  /** The surface the new thread is created and landed on; undefined takes
   *  the user's default (`createSurfaceFor`). Never applies to a fork. */
  presentation?: LandingChatPresentation,
): void {
  const homeRow = resolveHomeRowScope(parentId)
  if (homeRow) {
    if (kind === 'thread') {
      startHomeRowThread(homeRow.projectId, homeRow.homeWorkspaceId, parentId, presentation)
    }
    return
  }

  const currentRepos = useSidebarStore.getState().repos
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
    // The daemon places by CHAT id, and the clicked row's own id is only that
    // id for a branch row (a locked branch, the repo home — `rows-from-repo.ts`
    // draws those AS their owning `branch` chat). A REGULAR fork's row is id'd
    // from its `Workspace`, because its owner is an ordinary conversation
    // already drawn beside it, so its owning chat has to be read off the
    // workspace record instead. Falls back to the clicked row for the ids that
    // name no workspace of this repo (the repo home, a folder) and for a frame
    // that carries no owner yet.
    const owningChatId = repo.workspaces.find((w) => w.id === subject.id)?.owningChatId
    // A bubble's ground workspace can ALSO be the repo home — never in
    // `repo.workspaces` — so it resolves the way the home row's own id was rendered.
    const placementParentId =
      owningChatId ||
      (subject.id === repo.defaultWorkspaceId
        ? resolveHomeOwnerId(subject.id, repo.defaultOwningChatId, repo.chats ?? [])
        : parentId)
    // One fork in flight per parent; a fork with no provider says so here.
    if (createInFlight.has(`workspace:${placementParentId}`) || !enabledProvider()) return
    // EVERY sibling counts, any kind: the daemon appends a fork by counting all
    // rows under the same parent (owning_chat.go's placeOwningRow).
    const order = rowsFromRepo(repo).filter((r) => r.parentId === placementParentId).length
    useCreateDraftStore.getState().start({
      kind: 'branch',
      projectId,
      repoId: repo.id,
      workspaceId: subject.id,
      parentId: placementParentId,
      order,
      ownsWorktree: true,
    })
    return
  }

  // A thread runs in a real workspace: a `workspace` subject is one; a folder
  // takes its nearest owning workspace, stamped on its row at build time.
  const siblingRows = rowsFromRepo(repo)
  const wsId =
    subject.kind === 'workspace'
      ? subject.id
      : (siblingRows.find((r) => r.id === subject.id)?.workspaceId ?? null)
  if (!wsId) {
    toast.error('Start a thread from a workspace row — a folder has none to run it in')
    return
  }
  void startThread({
    inFlightKey: `thread:${parentId}`,
    projectId,
    workspaceId: wsId,
    parentId,
    presentation,
    onCreated: (chatId) => {
      announceTreeChange(repo.id)
      if (!openChatInOwnView(chatId, wsId)) {
        void navigateThenOpenChat(navigate, { projectId, repoId: repo.id, wsId }, chatId)
      }
    },
  })
}

/**
 * Sends the fork being named as a local operation, whose row stands in (or keeps
 * the daemon's reason) until its chat and workspace arrive; then the chat opens.
 */
export function confirmCreateDraft(name: string, navigate: NavigateFn): void {
  const draft = useCreateDraftStore.getState().draft
  useCreateDraftStore.getState().cancel()
  if (draft?.kind !== 'branch' || !draft.repoId) return
  const provider = enabledProvider()
  if (!provider) return
  const { projectId, repoId, parentId } = draft
  const inFlightKey = `workspace:${parentId}`
  if (createInFlight.has(inFlightKey)) return
  createInFlight.add(inFlightKey)
  const chatId = crypto.randomUUID()
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
      createChatWithOwnWorktree(chatId, opId, projectId, repoId, provider.id, parentId, name)
        .then((created) => {
          announceTreeChange(repoId)
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
  useCreateDraftStore.getState().cancel()
}

/**
 * Tell `repoId`'s sidebar tree to re-read its rows after this client created
 * one.
 *
 * See `removal-commit.ts`'s `bumpRepoTree` for the full story — same signal,
 * same reason, the create half. The daemon really does mint the chat and its
 * worktree (measured live: the repo's chat count went 9 -> 11 on two clicks),
 * but `openRepoTreeSubscription` reseeds `crowbar_chats` only on this
 * generation moving, and the only thing that normally moves it is a chat frame
 * arriving for a MOUNTED workspace of this repo. Fork from the sidebar with no
 * workspace of that repo open — on the project-home route, say — and the row
 * never appeared at all. The button had worked; nothing had drawn it.
 */
function announceTreeChange(repoId: string): void {
  if (repoId) useFolderSignalStore.getState().bump(repoId)
}
