import type { AgentChat } from '@/features/agent/api/agent-api'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import { useWorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'

/**
 * What a surface shows where a name would go while this store holds no record
 * to name. Never a word a chat could actually be called, so a load gap reads as
 * a gap instead of as a title.
 */
export const CHAT_TITLE_PENDING_LABEL = '—'

/**
 * The chat's display name, or `null` when `chats` carries no record for it.
 *
 * `UNTITLED_CHAT_LABEL` answers only for a record that IS here and has no title.
 * "This store has not got this chat" is a different fact and must not borrow
 * those words: a pane header was observed reading "Untitled chat" for a chat
 * both the sidebar and the daemon named, indistinguishable from a genuinely
 * untitled one until a reload.
 */
export function chatTitleIn(chats: readonly AgentChat[], chatId: string): string | null {
  const chat = chats.find((c) => c.id === chatId)
  if (!chat) return null
  return chat.title || UNTITLED_CHAT_LABEL
}

/** {@link chatTitleIn} against the active workspace store, narrow selector. */
export function useChatTitle(chatId: string): string | null {
  return useWorkspaceStoreContext((s) => chatTitleIn(s.agentChats.chats, chatId))
}
