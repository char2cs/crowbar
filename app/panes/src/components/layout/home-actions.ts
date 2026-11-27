import type { LandingChatPresentation } from '@/features/settings/lib/chat-presentation'
import { openHomeChat, type NavigateFn } from './open-actions'
import { newDraftChatId } from '@/features/agent/lib/draft-chat'
import { createSurfaceFor } from '@/features/agent/api/agent-providers-api'
import { IS_MOBILE } from '@/utils/platform'
import { startThread, startingProvider } from './thread-create'

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
