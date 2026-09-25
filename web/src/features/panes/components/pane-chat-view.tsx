import { lazy, Suspense, type ContextType, type Ref } from 'react'
import { ChatColumnHeader } from '@/features/tabs/components/chat-column-header'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { cn } from '@/lib/utils'

const AgentChatPane = lazy(() =>
  import('@/features/agent/components/agent-chat-pane').then((m) => ({
    default: m.AgentChatPane,
  })),
)

interface PaneChatViewProps {
  ref: Ref<HTMLDivElement>
  paneId: string
  chatId: string
  runnerId: string
  /** The pane's workspace, and the chat's own (with its store). */
  wsId: string
  chatWsId: string | null
  chatStore: ContextType<typeof WorkspaceStoreContext>
  hidden: boolean
  /** Null when the chat fills its box; else its share of the split, in percent. */
  basis: number | null
  /** The chat shares the pane with a visible editor: it gets its own header. */
  alongsideEditor: boolean
  chatFillsPane: boolean
  isBottomPane: boolean
  isActivePane: boolean
  isVisible: boolean
}

/** A pane's chat view: the chat surface, and its column header beside an editor. */
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
  return (
    <div
      ref={ref}
      data-chat-view=""
      hidden={hidden}
      className={cn(
        // No fill: the shared `data-pane-content` box already paints it.
        'relative flex min-h-0 min-w-0 flex-col overflow-hidden',
        basis === null ? 'h-full w-full flex-1' : 'shrink grow-0',
      )}
      // The sash maps the split onto whichever view is visually first.
      style={basis === null ? undefined : { flexBasis: `${basis}%` }}
    >
      {/* The chat surface and its header read the CHAT's workspace store
          off context; everything else in the pane keeps the ambient one. */}
      <WorkspaceStoreContext.Provider value={chatStore}>
        {alongsideEditor && (
          <ChatColumnHeader chatId={chatId} wsId={chatWsId} isBottomPane={isBottomPane} />
        )}
        <div className="relative min-h-0 flex-1 overflow-hidden">
          <Suspense fallback={null}>
            <AgentChatPane
              chatId={chatId}
              runnerId={runnerId}
              wsId={wsId}
              paneId={paneId}
              isActivePane={isActivePane}
              // Both overlay headers float above the chat with no flex space
              // of their own; the collapsed header reserves its own.
              belowOverlayHeader={chatFillsPane || alongsideEditor}
              // The dormant-chat revive fires on this: a parked or covered
              // chat claiming to be visible would spawn a CLI nobody sees.
              isVisible={isVisible}
            />
          </Suspense>
        </div>
      </WorkspaceStoreContext.Provider>
    </div>
  )
}
