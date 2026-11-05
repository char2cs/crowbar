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

/** The chat's top row beside a visible editor (side-by-side or stacked). Chat renders first, so it
 *  may be the window's top-left corner and carries PaneTopRow's chrome contract. Pane-level
 *  actions (branch review, close view) stay on the IDE sector's row. */
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
