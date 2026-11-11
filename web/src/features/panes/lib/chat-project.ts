import { entityOf, entityStore } from '@/core/entities/entity-store'
import { chatHostOf, resolvedHostOf } from '@/lib/host-resolve'
import { resolveChatWorkspaceId } from '@/features/panes/lib/pane-chat-workspace'

/** A chat's project; null while nothing loaded can name it, which is why a view fixes its project
 *  on its record at creation. Never in a render path: only when minting a record, and in
 *  openChatIntoPane's drop check, which lets an unresolvable chat through. */
export function resolveChatProjectId(chatId: string, workspaceHint?: string | null): string | null {
  const hostId = chatHostOf(chatId)
  const chat = hostId ? entityOf(entityStore.getState(), 'chat', hostId, chatId) : undefined
  if (chat?.projectId) return chat.projectId
  const wsId = resolveChatWorkspaceId(chatId, workspaceHint)
  return wsId ? resolveWorkspaceProjectId(wsId) : null
}

/** `wsId`'s owning project, as its workspace entity names it: a fork's, a repo's own
 *  checkout's and a project home's alike. */
export function resolveWorkspaceProjectId(wsId: string): string | null {
  const hostId = resolvedHostOf({ wsId })
  if (!hostId) return null
  return entityOf(entityStore.getState(), 'workspace', hostId, wsId)?.projectId || null
}
