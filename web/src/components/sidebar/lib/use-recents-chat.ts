import { selectAgentChat, useSelect } from '@/core/entities/select'

/** What a Recents member row draws: the same three facts from either source. */
export interface RecentsChat {
  id: string
  title: string
  workspaceId: string
  working: boolean
}

/** A Recents member's chat as the entity store holds it. */
export function useRecentsChat(workspaceId: string, chatId: string): RecentsChat | null {
  const chat = useSelect((s) => selectAgentChat(s, chatId))
  if (!chat) return null
  return {
    id: chat.id,
    title: chat.title,
    workspaceId: chat.workspaceId || workspaceId,
    working: chat.working ?? false,
  }
}
