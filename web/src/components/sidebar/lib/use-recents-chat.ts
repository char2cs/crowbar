import { useChatStore } from '@/features/agent/stores/chat-store'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import { useSidebarStore } from '@/lib/store/sidebar'

/** What a Recents member row draws: the same three facts from either source. */
export interface RecentsChat {
  id: string
  title: string
  workspaceId: string
  working: boolean
}

/** A Recents member's chat: the ChatStore's live record when one is held,
 *  else the sidebar's own chat lists. */
export function useRecentsChat(workspaceId: string, chatId: string): RecentsChat | null {
  const live = useChatStore((s) => s.chats[chatId])
  const working = live?.working ?? false
  const repoChat = useSidebarStore((s) => {
    for (const repo of s.repos) {
      const chat = repo.chats?.find((c) => c.id === chatId)
      if (chat) return chat
    }
    return undefined
  })
  const homeChat = useHomeTreeStore((s) => {
    for (const tree of Object.values(s.trees)) {
      const chat = tree.chats.find((c) => c.id === chatId)
      if (chat) return chat
    }
    return undefined
  })

  if (live) {
    return { id: live.id, title: live.title, workspaceId: live.workspaceId, working }
  }
  const record = repoChat ?? homeChat
  if (!record) return null
  return {
    id: record.id,
    title: record.title,
    workspaceId: record.workspaceId || workspaceId,
    working,
  }
}
