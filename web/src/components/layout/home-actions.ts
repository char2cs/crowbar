import type { LandingChatPresentation } from '@/features/settings/lib/chat-presentation'
import { openHomeChat, type NavigateFn } from './open-actions'
import { startThread } from './thread-create'

// Creates in a project's home: it rides no repo, so its rows live in
// the entity store; the operation's row stands in until the chat lands there.

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
 * `useHomeWorkspaceState`) — this function
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
    onCreated: (chatId) => openHomeChat(projectId, homeWorkspaceId, chatId, navigate),
  })
}
