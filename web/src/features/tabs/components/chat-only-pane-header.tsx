import { BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
import { usePaneTopRowEdges } from '../hooks/use-pane-top-row-edges'
import { openBranchReviewForWorkspace } from '@/features/panes/utils/pane-command-actions'
import { useSettingsStore } from '@/features/settings/store'
import type { PaneGroup } from '@/features/panes/types/pane'
import { ChatBranchHeader } from './chat-branch-header'
import { BranchReviewShortcutButton } from './branch-review-shortcut-button'
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
 * does (drag region, macOS traffic-light inset), plus the same right-pinned
 * branch-review shortcut — only the middle (the tab strip) is gone, replaced
 * by the chat's own identity header. Closing a chat/view is a SIDEBAR
 * operation only (Recents' own ×, or a row's own close) — this row offers no
 * close control of its own.
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
      <ChatBranchHeader
        chatId={pane.chatId ?? ''}
        wsId={wsId}
        className="h-full min-w-0 flex-1"
      />
      <BranchReviewShortcutButton
        isBottomPane={isBottomPane}
        // THIS pane's own workspace — not whichever one happens to be
        // globally active, which is a different pane in a split showing a
        // different chat/branch entirely.
        onOpen={() => openBranchReviewForWorkspace(wsId)}
      />
    </PaneTopRow>
  )
}
