import { BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
import { usePaneTopRowEdges } from '../hooks/use-pane-top-row-edges'
import { useSettingsStore } from '@/features/settings/store'
import type { PaneGroup } from '@/features/panes/types/pane'
import { ChatHeaderSlot } from '@/features/agent/hooks/chat-header-slot'
import { ChatBranchHeader } from './chat-branch-header'
import { OpenEditorButton } from './open-editor-button'
import { PaneTopRow } from './pane-top-row'

interface ChatOnlyPaneHeaderProps {
  pane: PaneGroup
  wsId: string | null
}

/**
 * TabBar's stand-in for a pane holding a chat and NO editor tabs at all —
 * per the chats/pane redesign, that pane has no IDE sector to speak of, so
 * this replaces TabBar's whole row rather than leaving it to draw an empty
 * tab strip. Renders through the same `PaneTopRow` shell TabBar's own row
 * does (drag region, macOS traffic-light inset); the middle (the tab strip)
 * is replaced by the chat's own identity header. Branch review is reached
 * from the editor area (Open editor, then Review Branch). Closing a chat/view
 * is a SIDEBAR operation only — this row offers no close control of its own.
 */
export function ChatOnlyPaneHeader({ pane, wsId }: ChatOnlyPaneHeaderProps) {
  const sidebarPosition = useSettingsStore((s) => s.settings.sidebarPosition)
  const { rowRef, isAtLeftEdge, isAtTopEdge } = usePaneTopRowEdges([sidebarPosition])
  const isBottomPane = pane.id === BOTTOM_PANE_ID

  return (
    <PaneTopRow
      rowRef={rowRef}
      paneId={pane.id}
      isBottomPane={isBottomPane}
      isAtLeftEdge={isAtLeftEdge}
      isAtTopEdge={isAtTopEdge}
      // chat-blur, not opaque: there is no IDE sector in this state at all
      // (chatFillsPane) — this row IS the chat's own toolbar, so it takes
      // the chat's own glass (steals the composer's progressive-blur
      // dissolve) rather than the flat fill TabBar's row (the IDE sector's
      // own header) carries. `overlay` floats this out of flex flow, which
      // is what lets the chat surface below render full-height, right up
      // behind it.
      variant="chat-blur"
      overlay
    >
      <ChatBranchHeader chatId={pane.chatId ?? ''} wsId={wsId} className="h-full min-w-0 flex-1" />
      <OpenEditorButton paneId={pane.id} wsId={wsId} />
      <ChatHeaderSlot />
    </PaneTopRow>
  )
}
