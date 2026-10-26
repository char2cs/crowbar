import { useStore } from 'zustand'
import { chatTitleIn } from '@/features/agent/hooks/use-chat-title'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useWorkspaceStoreById } from '@/features/workspace/stores/hooks/use-workspace-store-by-id'

/** The focused chat's name, read from its own workspace's store; nothing while no chat is open. */
export function MobileChatTitle({ className }: { className?: string }) {
  const chatId = useStore(windowPaneStore, (s) => s.panes[s.activePaneId]?.chatId ?? null)
  const wsId = useStore(windowPaneStore, (s) => s.panes[s.activePaneId]?.workspaceId ?? '')
  const title = useWorkspaceStoreById(wsId, (s) =>
    chatId ? chatTitleIn(s.agentChats.chats, chatId) : null,
  )
  if (!chatId || title === null) return null
  return <h1 className={className}>{title}</h1>
}
