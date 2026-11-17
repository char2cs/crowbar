import { useRef } from 'react'
import { ChatHeaderSlot } from '@/features/agent/hooks/chat-header-slot'
import { ChatBranchHeader } from '@/features/tabs/components/chat-branch-header'
import { PaneTopRow } from '@/features/tabs/components/pane-top-row'

/** The chat's identity row for the shell's chat pane: title and branch on the left, the chat surface's own controls (chat/terminal switch) on the right. The shell draws the window chrome, so the row never reserves the traffic-light inset. */
export function ChatPaneHeader({ chatId, wsId }: { chatId: string; wsId: string }) {
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
      <ChatHeaderSlot />
    </PaneTopRow>
  )
}
