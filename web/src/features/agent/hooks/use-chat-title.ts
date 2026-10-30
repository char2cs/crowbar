import type { AgentChat } from '@/features/agent/api/agent-chat-reads'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import { useChatStore } from '@/features/agent/stores/chat-store'

/**
 * What a surface shows where a name would go while this store holds no record
 * to name. Never a word a chat could actually be called, so a load gap reads as
 * a gap instead of as a title.
 */
export const CHAT_TITLE_PENDING_LABEL = '—'

/**
 * The chat's display name, or `null` when no record is held: a missing record
 * must not borrow the untitled label, or a load gap reads as a name.
 */
export function chatTitleOf(chat: AgentChat | undefined): string | null {
  if (!chat) return null
  return chat.title || UNTITLED_CHAT_LABEL
}

export function useChatTitle(chatId: string): string | null {
  return useChatStore((s) => chatTitleOf(s.chats[chatId]))
}
