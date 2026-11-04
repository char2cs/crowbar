import { useStore } from 'zustand'
import { useChatTitle } from '@/features/agent/hooks/use-chat-title'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'

/** The focused chat's name; nothing while no chat is open or its record has not arrived. */
export function MobileChatTitle({ className }: { className?: string }) {
  const chatId = useStore(windowPaneStore, (s) => s.panes[s.activePaneId]?.chatId ?? '')
  const title = useChatTitle(chatId)
  if (!chatId || title === null) return null
  return <h1 className={className}>{title}</h1>
}
