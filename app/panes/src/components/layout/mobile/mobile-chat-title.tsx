import { useStore } from 'zustand'
import { useChatTitle } from '@/features/agent/hooks/use-chat-title'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'

export function MobileChatTitle({ className }: { className?: string }) {
  const chatId = useStore(windowPaneStore, (s) => s.panes[s.activePaneId]?.chatId ?? '')
  const title = useChatTitle(chatId)
  if (!chatId || title === null) return null
  return <h1 className={className}>{title}</h1>
}
