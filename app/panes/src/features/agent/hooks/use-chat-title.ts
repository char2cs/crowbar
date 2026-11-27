import type { AgentChat } from '@/features/agent/api/agent-chat-reads'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import { selectAgentChat, useSelect } from '@/core/entities/select'

function chatTitleOf(chat: AgentChat | undefined): string | null {
  if (!chat) return null
  return chat.title || UNTITLED_CHAT_LABEL
}

export function useChatTitle(chatId: string): string | null {
  return useSelect((s) => chatTitleOf(selectAgentChat(s, chatId)))
}
