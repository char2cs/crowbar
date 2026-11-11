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

/** Replaces TabBar's row for a pane holding a chat and no editor tabs: same PaneTopRow shell,
 *  with the chat's identity header in place of the tab strip. Closing a chat is a sidebar
 *  operation only, so this row has no close control. */
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
      // This row IS the chat's toolbar, so it takes the chat glass; `overlay` lets the chat surface
      // render full-height behind it.
      variant="chat-blur"
      overlay
    >
      <ChatBranchHeader chatId={pane.chatId ?? ''} wsId={wsId} className="h-full min-w-0 flex-1" />
      <OpenEditorButton paneId={pane.id} wsId={wsId} />
      <ChatHeaderSlot />
    </PaneTopRow>
  )
}
