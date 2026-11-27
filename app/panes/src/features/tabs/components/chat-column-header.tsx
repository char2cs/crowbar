import { usePaneTopRowEdges } from '../hooks/use-pane-top-row-edges'
import { useSettingsStore } from '@/features/settings/store'
import { ChatHeaderSlot } from '@/features/agent/hooks/chat-header-slot'
import { ChatBranchHeader } from './chat-branch-header'
import { PaneTopRow } from './pane-top-row'

interface ChatColumnHeaderProps {
  chatId: string
  wsId: string | null
  isBottomPane: boolean
}

export function ChatColumnHeader({ chatId, wsId, isBottomPane }: ChatColumnHeaderProps) {
  const sidebarPosition = useSettingsStore((s) => s.settings.sidebarPosition)
  const { rowRef, isAtLeftEdge, isAtTopEdge } = usePaneTopRowEdges([sidebarPosition])

  return (
    <PaneTopRow
      rowRef={rowRef}
      isBottomPane={isBottomPane}
      isAtLeftEdge={isAtLeftEdge}
      isAtTopEdge={isAtTopEdge}
      variant="chat-blur"
      overlay
    >
      <ChatBranchHeader chatId={chatId} wsId={wsId} className="h-full min-w-0 flex-1" />
      <ChatHeaderSlot />
    </PaneTopRow>
  )
}
