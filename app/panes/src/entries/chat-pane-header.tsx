import { useRef } from 'react'
import { ChatHeaderSlot } from '@/features/agent/hooks/chat-header-slot'
import { ChatBranchHeader } from '@/features/tabs/components/chat-branch-header'
import { OpenEditorButton } from '@/features/tabs/components/open-editor-button'
import { PaneTopRow } from '@/features/tabs/components/pane-top-row'

interface ChatPaneHeaderProps {
  chatId: string
  wsId: string | null
  onOpenEditor?: () => void
}

export function ChatPaneHeader({ chatId, wsId, onOpenEditor }: ChatPaneHeaderProps) {
  const rowRef = useRef<HTMLDivElement>(null)
  return (
    <PaneTopRow
      rowRef={rowRef}
      isBottomPane={false}
      isAtLeftEdge={false}
      isAtTopEdge={false}
      variant="chat-blur"
      overlay
    >
      <ChatBranchHeader chatId={chatId} wsId={wsId} className="h-full min-w-0 flex-1" />
      {onOpenEditor && <OpenEditorButton wsId={wsId} onOpen={onOpenEditor} />}
      <ChatHeaderSlot />
    </PaneTopRow>
  )
}
