import type { LandingChatPresentation } from '@/features/settings/lib/chat-presentation'
import { openHomeChat, type NavigateFn } from './open-actions'
import { newDraftChatId } from '@/features/agent/lib/draft-chat'
import { draftLanding, finishDraftThread, type DraftPrompt, type DraftSpec } from './draft-thread'
import { startThread } from './thread-create'

// Creates in a project's home: it rides no repo, so its rows live in
// the entity store; the operation's row stands in until the chat lands there.

/** A thread under a project-home chat or folder; it runs in the home workspace. */
export function startHomeRowThread(
  projectId: string,
  homeWorkspaceId: string,
  parentId: string,
  presentation?: LandingChatPresentation,
): Promise<void> {
  return startThread({
    inFlightKey: `thread:${parentId}`,
    projectId,
    workspaceId: homeWorkspaceId,
    parentId,
    presentation,
  })
}

// The tree foot's "New thread" in the project home workspace, which `handleCreate` cannot reach
// (home is in no repo): opens a fresh draft chat there. The caller resolves `homeWorkspaceId`.
export function handleCreateHomeThread(
  projectId: string,
  homeWorkspaceId: string,
  navigate: NavigateFn,
): Promise<void> {
  return openHomeChat(projectId, homeWorkspaceId, newDraftChatId(), navigate)
}

/** Creates the chat a project-home draft was waiting to become, with its first prompt. */
export function promoteHomeDraft(
  draft: DraftSpec,
  prompt: DraftPrompt,
  typedSince: () => string,
): Promise<void> {
  const { projectId, workspaceId } = draft
  return startThread({
    inFlightKey: `draft:${draft.draftId}`,
    projectId,
    workspaceId,
    parentId: '',
    ...draftLanding(prompt),
    onCreated: (chatId) => finishDraftThread(draft, chatId, prompt, typedSince),
  })
}
