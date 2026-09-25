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
import { PANE_DROP_ATTR } from '@/components/sidebar/hooks/use-sidebar-drag'
import { SplitDropOverlay } from './split-drop-overlay'
import { PaneSash } from './pane-sash'
import { PaneAccentRing } from './pane-accent-ring'
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
  /** Whether this pane's VIEW is the one on screen — see PaneNodeRenderer.
   *  A parked view's panes stay mounted (so nothing they hold is torn down)
   *  but must do no work: every surface below is handed `isActive`/`isVisible`
   *  false, the same dormant state a background tab already runs in. */
  showing?: boolean
}

/**
 * One pane: its identity row, its chat view and its editor view (spec §7.2),
 * arranged by `usePanePresentation`. The chat reads its OWN workspace
 * (`usePaneWorkspace`), drags land through `usePaneDropHandlers`, and each tab
 * type's surface comes from the content registry.
 */
export function PaneContainer({
  pane,
  position = ROOT_PANE_POSITION,
  showing = true,
}: PaneContainerProps) {
  // A boolean, not the id: subscribing to `activePaneId` re-rendered every
  // pane whenever focus moved between any two.
  const isActiveInStore = useIsActivePane(pane.id)
  const { activateEditorTabInPane, setActivePane } = usePaneActions()
  const { wsId, chatWsId, chatStore } = usePaneWorkspace(pane)
  const sidebarPosition = useSettingsStore((state) => state.settings.sidebarPosition)
  // A pane in a parked view is never the active one, whatever `activePaneId`
  // says — folded in once so no surface is told it is live off screen.
  const isActivePane = showing && isActiveInStore

  // The ring answers "which pane has focus" — meaningless with one pane.
  const visiblePaneCount = useVisiblePaneCount()
  // A collapsed sidebar stops shielding the pane from the window frame.
  const sidebarOpen = useSidebarOptional()?.open ?? true
  const showActiveBorder = isActivePane && visiblePaneCount > 1
  // The box's own border stays neutral; `PaneAccentRing` carries the accent.
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

  // The chat view, sash and editor view each keep a stable key, so placing
  // them in either order (chat beside the sidebar) is never a remount.
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
      // splitSizes is always [chatPct, editorPct].
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

  // ALWAYS rendered under the same key — only `hidden` and its sizing change —
  // so nothing inside (a live terminal included) remounts when the chat comes
  // or goes or the presentation changes (spec §7.2: both surfaces stay
  // mounted; `display: none` dormancy is load-bearing).
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
      {/* Beside the chat, the tab strip belongs to the editor's own box. */}
      {chatVisibleAlongsideEditor && (
        <TabBar paneId={pane.id} wsId={chatWsId} onTabClick={handleTabClick} />
      )}
      <div className="relative min-h-0 flex-1 overflow-hidden">
        <PaneEditorViewContent
          paneId={pane.id}
          editorTabIds={pane.editorTabIds}
          activeEditorTabId={pane.activeEditorTabId}
          editorViewHidden={editorViewHidden}
          isActivePane={isActivePane}
          showing={showing}
        />
      </div>
    </div>
  )

  return (
    <div
      ref={containerRef}
      data-pane-container
      data-pane-id={pane.id}
      // Which view this pane belongs to — otherwise invisible from outside.
      data-view-id={pane.viewId ?? undefined}
      // The sidebar's drag arm hit-tests this to find the drop target (§8.1).
      {...{ [PANE_DROP_ATTR]: pane.id }}
      // Layout chrome: clicking anywhere just focuses the pane; every real
      // control inside keeps its own role.
      role="presentation"
      className={cn(
        'relative flex h-full w-full flex-col overflow-hidden',
        // File drags only: tab drags get the SplitDropOverlay zone instead.
        drop.isDragOver &&
          !drop.isTabDragOver &&
          !drop.internalHoverZone &&
          'ring-2 ring-secondary',
        // A sidebar-row drag hovering this pane (§8.2), unless the overlay
        // is already drawing its zone.
        !drop.internalHoverZone && 'data-[pane-hit]:ring-2 data-[pane-hit]:ring-secondary',
      )}
      // Mousedown activation is a native listener (usePaneActivation); this
      // catches a keyboard-triggered click, which has no mousedown.
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
        // ONE rounded, bordered box around the identity row and the content,
        // painting the translucent chat fill (the editor paints its own
        // opaque one over it). `data-pane-content` is also the hook for the
        // drag-time flattening rule in index.css.
        data-pane-content=""
        className="relative z-[1] flex min-h-0 flex-1 flex-col overflow-hidden bg-pane-chrome-bg transition-colors duration-150"
        style={paneContentStyle}
      >
        {/* A header spans the whole pane only when one surface fills it; it
            reads the chat's own workspace (a no-op wrap for a chatless pane). */}
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
        {/* One stable parent with keyed children, never a `chatId ? A : B`
            branch: a ternary reindexed the editor view on a chat landing and
            remounted it, live terminals included. */}
        <div
          ref={viewsContainerRef}
          className={cn('relative flex min-h-0 flex-1 overflow-hidden', isStacked && 'flex-col')}
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
  )
}
