import { useShallow } from 'zustand/react/shallow'
import { selectAgentChat, useSelect } from '@/core/entities/select'

/** What a Recents member row draws: the same three facts from either source. */
export interface RecentsChat {
  id: string
  title: string
  workspaceId: string
  working: boolean
}

/** A Recents member's chat as the entity store holds it. Selects only what the row draws,
 *  so a chat's runner or version moving renders no row. */
export function useRecentsChat(workspaceId: string, chatId: string): RecentsChat | null {
  return useSelect(
    useShallow((s) => {
      const chat = selectAgentChat(s, chatId)
      if (!chat) return null
      return {
        id: chat.id,
        title: chat.title,
        workspaceId: chat.workspaceId || workspaceId,
        working: chat.working ?? false,
      }
    }),
  )
}
