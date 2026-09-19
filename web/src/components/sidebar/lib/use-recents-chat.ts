import { useCallback, useSyncExternalStore } from 'react'
import { getWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import { useSidebarStore } from '@/lib/store/sidebar'

/** What a Recents member row draws: the same three facts from either source. */
export interface RecentsChat {
  id: string
  title: string
  workspaceId: string
  working: boolean
}

/**
 * A Recents member's chat, from the workspace store when one is REGISTERED
 * (live title, turn spinner) and from the sidebar's own chat lists otherwise.
 *
 * Never `getOrCreateWorkspaceStore`: a persisted dormant entry names a chat
 * nobody has opened since the reload, and minting a store for it would leak
 * one per band row. The registry is re-checked on every render, so a store
 * that mounts later (the chat gets opened) takes over on the next render the
 * pane change itself triggers.
 */
export function useRecentsChat(workspaceId: string, chatId: string): RecentsChat | null {
  const store = getWorkspaceStore(workspaceId)
  const subscribe = useCallback(
    (onChange: () => void) => (store ? store.subscribe(onChange) : () => {}),
    [store],
  )
  const live = useSyncExternalStore(subscribe, () => {
    if (!store) return undefined
    const { agentChats } = store.getState()
    return agentChats.chats.find((c) => c.id === chatId)
  })
  const working = useSyncExternalStore(subscribe, () =>
    store ? (store.getState().agentChats.working[chatId] ?? false) : false,
  )
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
