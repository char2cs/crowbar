import type { Chat } from '@/lib/store/sidebar'
import type { LandingChatPresentation } from '@/features/settings/lib/chat-presentation'
import { getHomeTree, useHomeTreeStore } from '@/lib/store/home-tree'
import { rowsFromHome } from '@/components/sidebar/lib/rows-from-home'
import { getHomeOwningChatId } from '@/features/workspace/lib/home-workspace-resolver'
import { openHomeChat, type NavigateFn } from './open-actions'
import { startThread, untilLanded } from './thread-create'

// Creates in a project's home: it rides no repo, so its rows live in
// `useHomeTreeStore` and a new thread's landing is watched there.

/** `waitForRow`'s own twin for a PROJECT-HOME thread: home rides no repo at
 *  all (`resolveHomeRowScope`'s own doc), so its chats live in
 *  `useHomeTreeStore`, not `useSidebarStore` — a create there is never
 *  observed by `waitForRow`'s subscription, which only ever fires on the
 *  repo-scoped store.
 *
 *  `parentId` is required, and checked — this is `forkHasLanded`'s own shape,
 *  not `chatHasLanded`'s. A home chat's placement is NOT on the `Chat`
 *  aggregate `MintChat` commits (its `ParentID` defaults to the Go zero value
 *  `""`, i.e. root) — for a home row it lives on a SEPARATE `Node` aggregate,
 *  written by a second, later call in the same backend request
 *  (`CreateChat`'s own `MintChat` then `placeChat`, chats.go). The chat
 *  lifecycle hub broadcasts on the FIRST commit alone, with no idea the
 *  second is still in flight, so a reseed can land here showing the chat
 *  already existing but still parented at root — landing this promise (and
 *  clearing the pending placeholder) on existence alone hands rendering to a
 *  REAL row that is itself still momentarily wrong. Caught live: a fresh home
 *  thread inside a folder appeared at the top of the list for a beat before
 *  snapping into the folder — the exact shape `forkHasLanded`'s own doc
 *  describes for a fork's two-aggregate mint, fixed here the same way rather
 *  than a new one invented for it. */
function waitForHomeChat(projectId: string, chatId: string, parentId: string): Promise<void> {
  return waitForHomeTree(projectId, (chats) =>
    chats.some((c) => c.id === chatId && c.parentId === parentId),
  )
}

/** A root home row's wire `parentId` is `""` OR the home workspace id, so wait on the rendered slot. */
function waitForRootHomeChat(
  projectId: string,
  homeWorkspaceId: string,
  chatId: string,
): Promise<void> {
  return waitForHomeTree(projectId, (chats) =>
    rowsFromHome(
      homeWorkspaceId,
      chats,
      getHomeTree(projectId).folders,
      getHomeOwningChatId(projectId) ?? undefined,
    ).some((r) => r.id === chatId && r.parentId === null),
  )
}

function waitForHomeTree(projectId: string, landedIn: (chats: Chat[]) => boolean): Promise<void> {
  return untilLanded(
    (onChange) => useHomeTreeStore.subscribe(onChange),
    () => landedIn(getHomeTree(projectId).chats),
  )
}

/** A thread under a project-home chat or folder; it runs in the home workspace. */
export function startHomeRowThread(
  projectId: string,
  homeWorkspaceId: string,
  parentId: string,
  presentation?: LandingChatPresentation,
): void {
  void startThread({
    inFlightKey: `thread:${parentId}`,
    projectId,
    workspaceId: homeWorkspaceId,
    parentId,
    presentation,
    landed: (chatId) => waitForHomeChat(projectId, chatId, parentId),
  })
}

/**
 * The sidebar header's "start a thread on the project's home workspace"
 * button — NOT reachable through `handleCreate` above, which resolves its
 * `parentId` against the repo-scoped sidebar store (`resolveRow`) and has no
 * notion of project home at all (home-workspace-resolver.ts: "home is a
 * project-level concept, not a repo workspace" — it never appears in
 * `repos`). Creates directly against the resolved home workspace id, then
 * opens it the same way `navigateThenOpenChat` opens a freshly-forked repo
 * chat: navigate to project home, wait for it to become the active
 * workspace, then open the chat in its own pane. `homeWorkspaceId` is the
 * caller's job to resolve (home-workspace-resolver.ts's
 * `useHomeWorkspaceState`/`ensureHomeWorkspaceResolved`) — this function
 * only spends it.
 */
export function handleCreateHomeThread(
  projectId: string,
  homeWorkspaceId: string,
  navigate: NavigateFn,
  /** Same as `handleCreate`'s own optional 4th arg. */
  presentation?: LandingChatPresentation,
): Promise<void> {
  return startThread({
    inFlightKey: `thread:home:${projectId}`,
    projectId,
    workspaceId: homeWorkspaceId,
    parentId: '',
    presentation,
    landed: (chatId) => waitForRootHomeChat(projectId, homeWorkspaceId, chatId),
    onCreated: (chatId) => openHomeChat(projectId, homeWorkspaceId, chatId, navigate),
  })
}
