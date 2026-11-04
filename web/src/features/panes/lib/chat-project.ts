import { entityOf, entityStore } from '@/core/entities/entity-store'
import { chatHostOf, resolvedHostOf } from '@/lib/host-resolve'
import { resolveChatWorkspaceId } from '@/features/panes/lib/pane-chat-workspace'

/**
 * Which PROJECT a chat belongs to — the derivation the pane store deliberately
 * never performs.
 *
 * A view's project is fixed on its record when the record is created,
 * precisely because this is not always answerable (before the projects scope
 * lands, nothing is known). Never used in a render path: only when a record is
 * minted, and in the drop refusal (`openChatIntoPane`), where an unresolvable chat is
 * ALLOWED through rather than refused.
 *
 * Null means nothing loaded right now can name a project for this chat.
 */
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
