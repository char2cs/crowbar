import { entityOf, entityStore } from '@/core/entities/entity-store'
import { chatHostOf, resolvedHostOf } from '@/lib/host-resolve'
import { resolveChatWorkspaceId } from '@/features/panes/lib/pane-chat-workspace'

export function resolveChatProjectId(chatId: string, workspaceHint?: string | null): string | null {
  const hostId = chatHostOf(chatId)
  const chat = hostId ? entityOf(entityStore.getState(), 'chat', hostId, chatId) : undefined
  if (chat?.projectId) return chat.projectId
  const wsId = resolveChatWorkspaceId(chatId, workspaceHint)
  return wsId ? resolveWorkspaceProjectId(wsId) : null
}

function resolveWorkspaceProjectId(wsId: string): string | null {
  const hostId = resolvedHostOf({ wsId })
  if (!hostId) return null
  return entityOf(entityStore.getState(), 'workspace', hostId, wsId)?.projectId || null
}
