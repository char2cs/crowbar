import { useRef } from 'react'
import { Button } from '@/components/ui/button'
import { CrowbarWordmark } from '@/components/ui/crowbar-wordmark'
import { useMobileScreenStore } from '@/features/window/stores/mobile-screen-store'
import { useActivePaneId, usePaneById } from '@/features/workspace/stores/hooks/use-pane-store'
import type { PaneGroup } from '../types/pane'
import { usePaneWorkspace } from '../hooks/use-pane-workspace'
import { PaneChatView } from './pane-chat-view'

/**
 * The phone's stage: the focused pane's chat and nothing else — no view tabs, splits,
 * editor half or pane header. Other views stay in the store, so a desktop-shaped layout
 * collapses to its focused chat here without being rewritten.
 */
export function MobileChatStage() {
  const pane = usePaneById(useActivePaneId())
  if (!pane?.chatId) return <NoChat />
  return <FocusedChat key={pane.id} pane={pane} chatId={pane.chatId} />
}

function FocusedChat({ pane, chatId }: { pane: PaneGroup; chatId: string }) {
  const ref = useRef<HTMLDivElement>(null)
  const { wsId, chatWsId, chatStore } = usePaneWorkspace(pane)
  return (
    <div className="flex h-full min-h-0 flex-col">
      <PaneChatView
        ref={ref}
        paneId={pane.id}
        chatId={chatId}
        runnerId={pane.runnerId ?? ''}
        wsId={wsId}
        chatWsId={chatWsId}
        chatStore={chatStore}
        hidden={false}
        basis={null}
        alongsideEditor={false}
        chatFillsPane={false}
        isBottomPane={false}
        isActivePane
        isVisible
      />
    </div>
  )
}

function NoChat() {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-6">
      <CrowbarWordmark aria-hidden="true" className="h-auto w-32 text-muted-foreground" />
      <Button
        variant="secondary"
        onClick={() => useMobileScreenStore.getState().setChatListOpen(true)}
      >
        Choose a chat
      </Button>
    </div>
  )
}
