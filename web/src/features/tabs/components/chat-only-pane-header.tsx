import { cn } from '@/lib/utils'
import { usePaneTopRowEdges } from '../hooks/use-pane-top-row-edges'
import { usePaneViewCloseControl } from '@/features/panes/hooks/use-pane-view-close-control'
import { openBranchReviewForActiveWorkspace } from '@/features/panes/utils/pane-command-actions'
import { useSettingsStore } from '@/features/settings/store'
import { IS_MAC } from '@/utils/platform'
import type { PaneGroup } from '@/features/panes/types/pane'
import { ChatBranchHeader } from './chat-branch-header'
import { BranchReviewShortcutButton } from './branch-review-shortcut-button'
import CloseViewButton from './close-view-button'

interface ChatOnlyPaneHeaderProps {
  pane: PaneGroup
  wsId: string | null
}

/**
 * TabBar's stand-in for a pane holding a chat and NO editor tabs at all —
 * per the chats/pane redesign, that pane has no IDE sector to speak of, so
 * this replaces TabBar's whole row rather than leaving it to draw an empty
 * tab strip. Carries the SAME window-chrome duties TabBar's row does (drag
 * region, macOS traffic-light inset) via the same `usePaneTopRowEdges`
 * geometry, plus the same right-pinned branch-review shortcut and
 * close-view control — only the middle (the tab strip) is gone, replaced by
 * the chat's own identity header.
 */
export function ChatOnlyPaneHeader({ pane, wsId }: ChatOnlyPaneHeaderProps) {
  const sidebarPosition = useSettingsStore((s) => s.settings.sidebarPosition)
  const { rowRef, isAtLeftEdge, isAtTopEdge } = usePaneTopRowEdges([sidebarPosition])
  const { isBottomPane, canClose, onCloseView } = usePaneViewCloseControl(pane)

  return (
    <div
      ref={rowRef}
      data-testid="pane-top-row"
      data-tab-bar-pane-id={pane.id}
      className={cn(
        // bg-chrome-bg, not bg-pane-background: there is no IDE sector in
        // this state at all (chatFillsPane) — this row IS the chat's own
        // toolbar, so it takes the chat's translucent tone rather than the
        // opaque one TabBar's row (the IDE sector's own header) carries.
        'relative flex shrink-0 items-center gap-1.5 overflow-hidden bg-chrome-bg px-2 py-1',
        IS_MAC ? 'h-[44px]' : 'h-[34px]',
        IS_MAC && !isBottomPane && isAtLeftEdge && isAtTopEdge && 'pl-[88px]',
      )}
      data-tauri-drag-region
    >
      <ChatBranchHeader
        chatId={pane.chatId ?? ''}
        wsId={wsId}
        className="h-full min-w-0 flex-1"
      />
      <BranchReviewShortcutButton
        isBottomPane={isBottomPane}
        onOpen={() => openBranchReviewForActiveWorkspace()}
      />
      <CloseViewButton
        isBottomPane={isBottomPane}
        disablePaneActions={isBottomPane}
        canClose={canClose}
        onCloseView={onCloseView}
      />
    </div>
  )
}
