import { useShallow } from 'zustand/react/shallow'
import { selectAgentChat, useSelect } from '@/core/entities/select'

export interface RecentsChat {
  id: string
  title: string
  workspaceId: string
  working: boolean
}

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
