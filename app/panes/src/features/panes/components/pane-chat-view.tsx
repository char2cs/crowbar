import { lazy, Suspense, useLayoutEffect, useState, type ContextType, type Ref } from 'react'
import { loadedAgentChatPane, preloadAgentChatPane } from '@/features/panes/lib/chat-surface-loader'
import { pendingChatSurfaces } from '@/features/panes/stores/pending-chat-surfaces'
import { sidebarSurfaceRoot } from '@/features/panes/lib/sidebar-surface'
import { ChatColumnHeader } from '@/features/tabs/components/chat-column-header'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { cn } from '@/lib/utils'

const LazyAgentChatPane = lazy(() => preloadAgentChatPane().then((pane) => ({ default: pane })))

interface PaneChatViewProps {
  ref: Ref<HTMLDivElement>
  paneId: string
  chatId: string
  runnerId: string
  wsId: string
  chatWsId: string | null
  chatStore: ContextType<typeof WorkspaceStoreContext>
  hidden: boolean
  basis: number | null
  alongsideEditor: boolean
  chatFillsPane: boolean
  isBottomPane: boolean
  isActivePane: boolean
  isVisible: boolean
}

export function PaneChatView({
  ref,
  paneId,
  chatId,
  runnerId,
  wsId,
  chatWsId,
  chatStore,
  hidden,
  basis,
  alongsideEditor,
  chatFillsPane,
  isBottomPane,
  isActivePane,
  isVisible,
}: PaneChatViewProps) {
  const [Surface] = useState(() => loadedAgentChatPane() ?? LazyAgentChatPane)
  useLayoutEffect(() => {
    pendingChatSurfaces.getState().begin(paneId)
    return () => pendingChatSurfaces.getState().end(paneId)
  }, [paneId])
  return (
    <div
      ref={ref}
      data-chat-view=""
      {...sidebarSurfaceRoot('chats')}
      hidden={hidden}
      className={cn(
        'relative flex min-h-0 min-w-0 flex-col overflow-hidden outline-none',
        basis === null ? 'h-full w-full flex-1' : 'shrink grow-0',
      )}
      style={basis === null ? undefined : { flexBasis: `${basis}%` }}
    >
      <WorkspaceStoreContext.Provider value={chatStore}>
        {alongsideEditor && (
          <ChatColumnHeader chatId={chatId} wsId={chatWsId} isBottomPane={isBottomPane} />
        )}
        <div className="relative min-h-0 flex-1 overflow-hidden">
          <Suspense fallback={null}>
            <Surface
              chatId={chatId}
              runnerId={runnerId}
              wsId={wsId}
              paneId={paneId}
              isActivePane={isActivePane}
              belowOverlayHeader={chatFillsPane || alongsideEditor}
              isVisible={isVisible}
            />
          </Suspense>
        </div>
      </WorkspaceStoreContext.Provider>
    </div>
  )
}
