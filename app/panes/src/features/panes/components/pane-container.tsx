import { useCallback, useMemo, useRef, useState } from 'react'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { useSettingsStore } from '@/features/settings/store'
import { buildInnerViewStyle, buildPaneContentStyle } from '../utils/pane-border'
import { useSidebarOptional } from '@/components/ui/sidebar'
import { cn } from '@/lib/utils'
import { ROOT_PANE_POSITION, type PanePosition } from '../types/pane'
import TabBar from '@/features/tabs/components/tab-bar'
import { ChatOnlyPaneHeader } from '@/features/tabs/components/chat-only-pane-header'
import { BOTTOM_PANE_ID } from '../constants/pane'
import {
  useIsActivePane,
  usePaneActions,
  useVisiblePaneCount,
} from '@/features/workspace/stores/hooks/use-pane-store'
import type { PaneGroup } from '../types/pane'
import { PANE_DROP_ATTR } from '@/components/sidebar/lib/sidebar-drag-hover'
import { SplitDropOverlay } from './split-drop-overlay'
import { PaneSash } from './pane-sash'
import { PaneAccentRing } from './pane-accent-ring'
import { PaneHandle } from './pane-handle'
import { ChatHeaderSlotProvider } from '@/features/agent/hooks/chat-header-slot'
import { PaneChatView } from './pane-chat-view'
import { PaneEditorViewContent } from './pane-editor-view-content'
import {
  SPLIT_DEFAULT_SIZES,
  SPLIT_MIN_HALF_PX,
  SPLIT_MIN_STACKED_PX,
} from '@/features/agent/hooks/use-chat-presentation'
import { usePaneWorkspace } from '../hooks/use-pane-workspace'
import { usePaneDropHandlers } from '../hooks/use-pane-drop-handlers'
import { usePanePresentation } from '../hooks/use-pane-presentation'
import { usePaneActivation } from '../hooks/use-pane-activation'

interface PaneContainerProps {
  pane: PaneGroup
  position?: PanePosition
  showing?: boolean
}

export function PaneContainer({
  pane,
  position = ROOT_PANE_POSITION,
  showing = true,
}: PaneContainerProps) {
  const isActiveInStore = useIsActivePane(pane.id)
  const { activateEditorTabInPane, setActivePane } = usePaneActions()
  const { wsId, chatWsId, chatStore } = usePaneWorkspace(pane)
  const sidebarPosition = useSettingsStore((state) => state.settings.sidebarPosition)
  const isActivePane = showing && isActiveInStore

  const visiblePaneCount = useVisiblePaneCount()
  const sidebarOpen = useSidebarOptional()?.open ?? true
  const showActiveBorder = isActivePane && visiblePaneCount > 1
  const paneContentStyle = useMemo(
    () => buildPaneContentStyle(position, sidebarPosition, false, sidebarOpen),
    [position, sidebarPosition, sidebarOpen],
  )

  const containerRef = useRef<HTMLDivElement>(null)
  const drop = usePaneDropHandlers(pane.id, containerRef)
  usePaneActivation(pane.id, containerRef)

  const viewsContainerRef = useRef<HTMLDivElement>(null)
  const chatViewRef = useRef<HTMLDivElement>(null)
  const editorViewRef = useRef<HTMLDivElement>(null)
  const [splitSizes, setSplitSizes] = useState<[number, number]>(SPLIT_DEFAULT_SIZES)
  const {
    presentation,
    chatFillsPane,
    showChatTab,
    chatVisibleAlongsideEditor,
    showTopLevelHeader,
    editorViewHidden,
    chatViewHidden,
    isStacked,
    chatIsFirst,
    editorFacingChatEdge,
  } = usePanePresentation(pane, sidebarPosition, viewsContainerRef)
  const isBottomPane = pane.id === BOTTOM_PANE_ID

  const editorInnerViewStyle = useMemo(
    () => (chatVisibleAlongsideEditor ? buildInnerViewStyle(editorFacingChatEdge) : undefined),
    [chatVisibleAlongsideEditor, editorFacingChatEdge],
  )

  const handlePaneClick = useCallback(() => {
    if (!isActivePane) setActivePane(pane.id)
  }, [isActivePane, pane.id, setActivePane])

  const handleTabClick = useCallback(
    (tabId: string) => {
      activateEditorTabInPane(pane.id, tabId)
      setActivePane(pane.id)
    },
    [pane.id, activateEditorTabInPane, setActivePane],
  )

  const chatViewNode = pane.chatId && (
    <PaneChatView
      key="chat-view"
      ref={chatViewRef}
      paneId={pane.id}
      chatId={pane.chatId}
      runnerId={pane.runnerId ?? ''}
      wsId={wsId}
      chatWsId={chatWsId}
      chatStore={chatStore}
      hidden={chatViewHidden}
      basis={presentation === 'tabs' || chatFillsPane ? null : splitSizes[0]}
      alongsideEditor={chatVisibleAlongsideEditor}
      chatFillsPane={chatFillsPane}
      isBottomPane={isBottomPane}
      isActivePane={isActivePane}
      isVisible={showing && !chatViewHidden}
    />
  )

  const sashNode = pane.chatId && presentation !== 'tabs' && !chatFillsPane && (
    <PaneSash
      key="chat-editor-sash"
      direction={presentation === 'stacked' ? 'vertical' : 'horizontal'}
      sizes={chatIsFirst ? splitSizes : [splitSizes[1], splitSizes[0]]}
      containerRef={viewsContainerRef}
      firstPaneRef={chatIsFirst ? chatViewRef : editorViewRef}
      secondPaneRef={chatIsFirst ? editorViewRef : chatViewRef}
      onResizeCommit={(sizes) => setSplitSizes(chatIsFirst ? sizes : [sizes[1], sizes[0]])}
      minPx={presentation === 'stacked' ? SPLIT_MIN_STACKED_PX : SPLIT_MIN_HALF_PX}
    />
  )

  const editorViewNode = (
    <div
      key="editor-view"
      ref={editorViewRef}
      data-editor-view=""
      hidden={editorViewHidden}
      className={cn(
        'relative flex min-h-0 flex-col overflow-hidden bg-pane-background',
        Boolean(pane.chatId) && presentation !== 'tabs' ? 'shrink grow-0' : 'w-full flex-1',
      )}
      style={{
        ...(Boolean(pane.chatId) && presentation !== 'tabs'
          ? { flexBasis: `${splitSizes[1]}%` }
          : undefined),
        ...editorInnerViewStyle,
      }}
    >
      {chatVisibleAlongsideEditor && (
        <TabBar paneId={pane.id} wsId={chatWsId} onTabClick={handleTabClick} />
      )}
      <div className="relative min-h-0 flex-1 overflow-hidden">
        <PaneEditorViewContent
          paneId={pane.id}
          wsId={chatWsId}
          editorTabIds={pane.editorTabIds}
          activeEditorTabId={pane.activeEditorTabId}
          editorViewHidden={editorViewHidden}
          isActivePane={isActivePane}
          showing={showing}
          dropTargeted={drop.isDragOver || drop.isTabDragOver || !!drop.internalHoverZone}
        />
      </div>
    </div>
  )

  return (
    <ChatHeaderSlotProvider>
      <WorkspaceStoreContext.Provider value={chatStore}>
        <div
          ref={containerRef}
          data-pane-container
          data-pane-id={pane.id}
          data-view-id={pane.viewId ?? undefined}
          {...{ [PANE_DROP_ATTR]: pane.id }}
          role="presentation"
          className={cn(
            'group/pane relative flex h-full w-full flex-col overflow-hidden',
            drop.isDragOver &&
              !drop.isTabDragOver &&
              !drop.internalHoverZone &&
              'ring-2 ring-secondary',
            !drop.internalHoverZone && 'data-[pane-hit]:ring-2 data-[pane-hit]:ring-secondary',
          )}
          onClick={handlePaneClick}
          onDragOver={drop.handleDragOver}
          onDragLeave={drop.handleDragLeave}
          onDrop={drop.handleDrop}
        >
          {drop.isDragOver && !drop.isTabDragOver && !drop.internalHoverZone && (
            <div className="pointer-events-none absolute inset-0 z-40 bg-secondary/10" />
          )}
          <SplitDropOverlay
            visible={drop.isTabDragOver || !!drop.internalHoverZone}
            onDrop={drop.handleSplitDrop}
            activeZoneOverride={drop.internalHoverZone}
          />
          <div
            data-pane-content=""
            className="relative z-[1] flex min-h-0 flex-1 flex-col overflow-hidden bg-pane-chrome-bg transition-colors duration-150"
            style={paneContentStyle}
          >
            <PaneHandle pane={pane} />
            {showTopLevelHeader && (
              <WorkspaceStoreContext.Provider value={chatStore}>
                {chatFillsPane ? (
                  <ChatOnlyPaneHeader pane={pane} wsId={chatWsId} />
                ) : (
                  <TabBar
                    paneId={pane.id}
                    wsId={chatWsId}
                    onTabClick={handleTabClick}
                    showChatTab={showChatTab}
                  />
                )}
              </WorkspaceStoreContext.Provider>
            )}
            <div
              ref={viewsContainerRef}
              className={cn(
                'relative flex min-h-0 flex-1 overflow-hidden',
                isStacked && 'flex-col',
              )}
            >
              {chatIsFirst ? (
                <>
                  {chatViewNode}
                  {sashNode}
                  {editorViewNode}
                </>
              ) : (
                <>
                  {editorViewNode}
                  {sashNode}
                  {chatViewNode}
                </>
              )}
            </div>
          </div>
          <PaneAccentRing
            position={position}
            sidebarPosition={sidebarPosition}
            sidebarOpen={sidebarOpen}
            visible={showActiveBorder}
          />
        </div>
      </WorkspaceStoreContext.Provider>
    </ChatHeaderSlotProvider>
  )
}
