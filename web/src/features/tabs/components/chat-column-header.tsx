import { usePaneTopRowEdges } from '../hooks/use-pane-top-row-edges'
import { useSettingsStore } from '@/features/settings/store'
import { ChatBranchHeader } from './chat-branch-header'
import { PaneTopRow } from './pane-top-row'

interface ChatColumnHeaderProps {
  chatId: string
  wsId: string | null
  isBottomPane: boolean
}

/**
 * The chat's own top row when it sits ALONGSIDE a visible editor — side by
 * side (landscape) or stacked (portrait) presentation, where the redesign's
 * two boxes (chat interface, IDE sector) are both on screen at once. Chat
 * renders first, so this may genuinely be the pane's own top-left corner —
 * it carries the same `PaneTopRow` window-chrome contract TabBar's row does
 * (drag region, macOS traffic-light inset) for exactly that reason.
 *
 * Carries none of TabBar's pane-level actions (branch-review shortcut,
 * close-view) — those stay on the IDE sector's own row now that tabs no
 * longer span the whole pane (the redesign's actual bug: TabBar used to
 * sit above BOTH columns instead of being confined to the IDE sector's).
 */
export function ChatColumnHeader({ chatId, wsId, isBottomPane }: ChatColumnHeaderProps) {
  const sidebarPosition = useSettingsStore((s) => s.settings.sidebarPosition)
  const { rowRef, isAtLeftEdge, isAtTopEdge } = usePaneTopRowEdges([sidebarPosition])

  return (
    <PaneTopRow
      rowRef={rowRef}
      isBottomPane={isBottomPane}
      isAtLeftEdge={isAtLeftEdge}
      isAtTopEdge={isAtTopEdge}
      className="bg-chrome-bg"
    >
      <ChatBranchHeader chatId={chatId} wsId={wsId} className="h-full min-w-0 flex-1" />
    </PaneTopRow>
  )
}
