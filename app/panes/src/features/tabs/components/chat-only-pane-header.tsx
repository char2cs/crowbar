import { BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
import { usePaneTopRowEdges } from '../hooks/use-pane-top-row-edges'
import { useSettingsStore } from '@/features/settings/store'
import type { PaneGroup } from '@/features/panes/types/pane'
import { ChatHeaderSlot } from '@/features/agent/hooks/chat-header-slot'
import { ChatBranchHeader } from './chat-branch-header'
import { togglePaneEditor } from '@/features/panes/utils/pane-command-actions'
import { OpenEditorButton } from './open-editor-button'
import { PaneTopRow } from './pane-top-row'

interface ChatOnlyPaneHeaderProps {
  pane: PaneGroup
  wsId: string | null
}

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
      variant="chat-blur"
      overlay
    >
      <ChatBranchHeader chatId={pane.chatId ?? ''} wsId={wsId} className="h-full min-w-0 flex-1" />
      <OpenEditorButton wsId={wsId} onOpen={() => togglePaneEditor(pane.id)} />
      <ChatHeaderSlot />
    </PaneTopRow>
  )
}
