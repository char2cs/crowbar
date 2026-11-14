import type { LandingChatPresentation } from '@/features/settings/lib/chat-presentation'
import { openHomeChat, type NavigateFn } from './open-actions'
import { newDraftChatId } from '@/features/agent/lib/draft-chat'
import { createSurfaceFor } from '@/features/agent/api/agent-providers-api'
import { IS_MOBILE } from '@/utils/platform'
import { startThread, startingProvider } from './thread-create'

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
// (home is in no repo): opens a fresh draft chat there, or with Terminal the default, starts on
// the terminal like a fork (a draft is a Chat composer). The caller resolves `homeWorkspaceId`.
export function handleCreateHomeThread(
  projectId: string,
  homeWorkspaceId: string,
  navigate: NavigateFn,
): Promise<void> {
  if (IS_MOBILE || createSurfaceFor(startingProvider(projectId)) !== 'terminal') {
    return openHomeChat(projectId, homeWorkspaceId, newDraftChatId(), navigate)
  }
  return startThread({
    inFlightKey: `thread:home:${homeWorkspaceId}`,
    projectId,
    workspaceId: homeWorkspaceId,
    parentId: '',
    presentation: 'terminal',
    onCreated: (chatId) => openHomeChat(projectId, homeWorkspaceId, chatId, navigate),
  })
}
