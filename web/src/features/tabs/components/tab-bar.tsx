import { DndContext, closestCenter, useSensor, useSensors } from '@dnd-kit/core'
import { SortableContext, horizontalListSortingStrategy } from '@dnd-kit/sortable'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useStore } from 'zustand'
import { useEditorStateStore } from '@/features/editor/stores/state-store'
import { usePaneById, usePaneActions } from '@/features/workspace/stores/hooks/use-pane-store'
import { useBufferActions } from '@/features/workspace/stores/hooks/use-buffer-store'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
import {
  openBranchReviewForWorkspace,
  togglePaneEditor,
} from '@/features/panes/utils/pane-command-actions'
import { usePaneCanReviewBranch } from '@/features/panes/hooks/use-pane-can-review-branch'
import { useHasViewStrip } from '@/features/panes/hooks/use-tab-view-ids'
import { BuildReadout } from '@/components/layout/build-readout'
import { useSettingsStore } from '@/features/settings/store'
import type { PaneContent } from '@/features/panes/types/pane-content'
import { useSidebarStore } from '@/features/layout/stores/sidebar-store'
import { useSidebar } from '@/components/ui/sidebar'
import TabBarItem from './tab-bar-item'
import { TabBarContextMenu, type TabMenuState } from './tab-bar-context-menu'
import { TabCloseConfirm } from './tab-close-confirm'
import { TabDragPreview } from './tab-drag-preview'
import TabNavigationButtons from './tab-navigation-buttons'
import TabAddButton from './tab-add-button'
import SortableEditorTab from './sortable-editor-tab'
import { SplitToggleButton } from './split-toggle-button'
import { BranchReviewShortcutButton } from './branch-review-shortcut-button'
import { ChatTabItem } from './chat-tab-item'
import { PaneTopRow } from './pane-top-row'
import { TabOverflowMenu } from './tab-overflow-menu'
import { usePaneNewTabActions } from '../hooks/use-pane-new-tab-actions'
import { useBufferDisplayName } from '../hooks/use-buffer-display-name'
import { useTabKeyboardNav } from '../hooks/use-tab-keyboard-nav'
import { useTabDrag } from '../hooks/use-tab-drag'
import { useTabBarScroll } from '../hooks/use-tab-bar-scroll'
import { useRenderedPaneBuffers } from '../hooks/use-rendered-pane-buffers'
import { NoDndPointerSensor } from '../lib/no-dnd-pointer-sensor'

interface TabBarProps {
  paneId?: string
  /** THIS pane's own workspace (see pane-container.tsx's `wsId` resolution)
   *  — needed to open branch review, or a new file/terminal, for the right
   *  workspace rather than whichever one happens to be globally active. */
  wsId?: string | null
  onTabClick?: (bufferId: string) => void
  /** Draw the chat as the strip's first tab (collapsed presentation). pane-container.tsx owns
   *  when; TabBar only draws it. */
  showChatTab?: boolean
}

// react-doctor-disable-next-line no-giant-component -- accepted: cohesive tab strip — drag/reorder, overflow scroll and active-tab tracking share one dnd context and scroll ref.
const TabBar = ({
  paneId,
  wsId,
  onTabClick: externalTabClick,
  showChatTab = false,
}: TabBarProps) => {
  const globalActiveBufferId = useStore(
    windowPaneStore,
    (s) => s.paneActions.getActivePane()?.activeEditorTabId ?? null,
  )
  const pane = usePaneById(paneId ?? '')
  const {
    setActivePane,
    activateEditorTabInPane,
    activateChatInPane,
    removeEditorTabFromPane,
    reorderEditorTabs,
  } = usePaneActions()
  const { closeBuffer, promotePreview, setPinned, setPendingClose } = useBufferActions()
  const paneBufferIds = pane?.editorTabIds ?? []
  const hasEditorTabs = paneBufferIds.length > 0
  // Gated on rendered fields (see useRenderedPaneBuffers), so content flushes don't re-render.
  const buffers = useRenderedPaneBuffers(paneBufferIds)
  const activeBufferCandidate = pane ? pane.activeEditorTabId : globalActiveBufferId
  const activeBufferId =
    activeBufferCandidate && buffers.some((buffer) => buffer.id === activeBufferCandidate)
      ? activeBufferCandidate
      : null
  // Read defensively (`!== false`) — see PaneGroup.chatSelected's own doc on
  // why a pane restored from an old layout may not have this field at all.
  const chatIsSelected = showChatTab && pane?.chatSelected !== false

  const handleTabPin = useCallback(
    (bufferId: string) => {
      const buf = windowPaneStore.getState().buffers.find((b) => b.id === bufferId)
      if (buf) setPinned(bufferId, !buf.isPinned)
    },
    [setPinned],
  )

  const handleTabClick = useCallback(
    (bufferId: string) => {
      if (paneId) {
        activateEditorTabInPane(paneId, bufferId)
        setActivePane(paneId)
      }
      externalTabClick?.(bufferId)
    },
    [activateEditorTabInPane, externalTabClick, paneId, setActivePane],
  )

  const handleChatTabSelect = useCallback(() => {
    if (!paneId) return
    activateChatInPane(paneId)
    setActivePane(paneId)
  }, [activateChatInPane, paneId, setActivePane])

  const handleTabClose = useCallback(
    (bufferId: string) => {
      const buf = windowPaneStore.getState().buffers.find((b) => b.id === bufferId)
      // Every close affordance funnels through here; a pane's sole editor tab
      // (isUncloseable) has none, so it is honoured here too.
      if (buf?.isUncloseable) return
      if (buf && buf.type === 'editor' && buf.isDirty) {
        setPendingClose({ type: 'single', bufferId })
        return
      }
      if (paneId) removeEditorTabFromPane(paneId, bufferId)
      closeBuffer(bufferId)
    },
    [closeBuffer, paneId, removeEditorTabFromPane, setPendingClose],
  )

  const updateActivePath = useSidebarStore((s) => s.updateActivePath)
  const sidebarPosition = useSettingsStore((s) => s.settings.sidebarPosition)
  const { open: sidebarOpen, toggleSidebar } = useSidebar()
  const isBottomPane = paneId === BOTTOM_PANE_ID
  // The last pane in a window, holding nothing: no control names or acts on its content. The
  // row stays: it carries the traffic-light inset, the window's drag region and the only way
  // back to a collapsed sidebar.
  const isEmptyPane = pane !== null && !pane.chatId && !hasEditorTabs

  // A thread or a project home has no branch of its own to review, so it hides the review
  // shortcut; an editor-only pane (no chat) on a branch keeps it.
  const canReview = usePaneCanReviewBranch(paneId ?? '', wsId ?? null)

  const [contextMenu, setContextMenu] = useState<TabMenuState>({
    isOpen: false,
    position: { x: 0, y: 0 },
    buffer: null,
  })

  const [srAnnouncement, setSrAnnouncement] = useState<string>('')

  const tabRefs = useRef<(HTMLDivElement | null)[]>([])

  const { clearPositionCache } = useEditorStateStore.use.actions()

  const sensors = useSensors(
    useSensor(NoDndPointerSensor, {
      activationConstraint: { distance: 6 },
    }),
  )

  const sortedBuffers = useMemo(() => {
    return [...buffers].sort((a, b) => {
      if (a.isPinned && !b.isPinned) return -1
      if (!a.isPinned && b.isPinned) return 1
      return 0
    })
  }, [buffers])
  const sortedBufferIds = useMemo(() => sortedBuffers.map((buffer) => buffer.id), [sortedBuffers])

  // `reorderEditorTabs` (pane-slice) takes the moved tab's OWN id and a target
  // index, not a pair of indices — translate dnd-kit's oldIndex/newIndex pair
  // (from `sortedBufferIds`, the array it actually dragged) into that shape.
  const reorderBuffers = useCallback(
    (startIndex: number, endIndex: number) => {
      const tabId = sortedBufferIds[startIndex]
      if (paneId && tabId) reorderEditorTabs(paneId, tabId, endIndex)
    },
    [paneId, reorderEditorTabs, sortedBufferIds],
  )

  const handleTabSelect = useCallback(
    (buffer: PaneContent) => {
      if (externalTabClick) {
        externalTabClick(buffer.id)
      } else {
        handleTabClick(buffer.id)
      }
      updateActivePath(buffer.path ?? '')
      setSrAnnouncement(
        `Switched to ${buffer.name}${buffer.type === 'editor' && buffer.isDirty ? ', unsaved changes' : ''}`,
      )
    },
    [externalTabClick, handleTabClick, updateActivePath],
  )

  const { draggedBufferId, draggedBuffer, handleDragStart, handleDragEnd, resetDrag } = useTabDrag({
    sortedBuffers,
    onTabSelect: handleTabSelect,
    onTabClick: handleTabClick,
    onReorderBuffers: reorderBuffers,
  })

  const { tabBarRef, scrollRef, isAtLeftEdge, isAtRightEdge, isAtTopEdge, handleWheel } =
    useTabBarScroll({
      sidebarPosition,
      draggedBufferId,
    })

  const handleToggleSplit = useCallback(() => {
    if (paneId) togglePaneEditor(paneId)
  }, [paneId])

  const { onNewTab } = usePaneNewTabActions(wsId, paneId ?? '')

  // The sidebar-reopen toggle is a fallback for a collapsed sidebar, shown on the window-edge pane
  // on the sidebar's side; `isAtTopEdge` because a view-tab strip above the panes owns it instead.
  const showReopenToggleLeft =
    !sidebarOpen && !isBottomPane && sidebarPosition === 'left' && isAtLeftEdge && isAtTopEdge
  const showReopenToggleRight =
    !sidebarOpen && !isBottomPane && sidebarPosition === 'right' && isAtRightEdge && isAtTopEdge

  // The readout travels with the toggle; the view strip carries both once it exists.
  const hasViewStrip = useHasViewStrip()
  const showReadoutLeft = showReopenToggleLeft && !hasViewStrip
  const showReadoutRight = showReopenToggleRight && !hasViewStrip

  const getBufferDisplayName = useBufferDisplayName({ buffers })

  const overflowTabs = useMemo(
    () => sortedBuffers.map((buffer) => ({ id: buffer.id, name: getBufferDisplayName(buffer) })),
    [sortedBuffers, getBufferDisplayName],
  )
  const handleOverflowSelect = useCallback(
    (id: string) => {
      const buffer = sortedBuffers.find((b) => b.id === id)
      if (buffer) handleTabSelect(buffer)
    },
    [sortedBuffers, handleTabSelect],
  )

  // Auto-scroll active tab into view
  useEffect(() => {
    const activeIndex = sortedBuffers.findIndex((buffer) => buffer.id === activeBufferId)
    if (activeIndex !== -1 && tabRefs.current[activeIndex] && scrollRef.current) {
      const activeTab = tabRefs.current[activeIndex]
      const container = scrollRef.current
      if (activeTab) {
        const tabRect = activeTab.getBoundingClientRect()
        const containerRect = container.getBoundingClientRect()
        if (tabRect.left < containerRect.left || tabRect.right > containerRect.right) {
          activeTab.scrollIntoView({ behavior: 'smooth', block: 'nearest', inline: 'center' })
        }
      }
    }
  }, [activeBufferId, sortedBuffers, scrollRef])

  useEffect(() => {
    tabRefs.current = tabRefs.current.slice(0, sortedBuffers.length)
  }, [sortedBuffers.length])

  // Takes the buffer, not an index into sortedBuffers (rebuilt every keystroke), so the handler
  // reference stays stable and TabBarItem's memo holds.
  const handleDoubleClick = useCallback(
    (e: React.MouseEvent, buffer: PaneContent) => {
      e.preventDefault()
      e.stopPropagation()
      if (buffer.isPreview) promotePreview(buffer.id)
    },
    [promotePreview],
  )

  const handleContextMenu = useCallback((e: React.MouseEvent, buffer: PaneContent) => {
    e.preventDefault()
    const target = e.currentTarget as HTMLElement
    const rect = target.getBoundingClientRect()
    setContextMenu({
      isOpen: true,
      position: { x: rect.left + rect.width * 0.5, y: rect.bottom + 4 },
      buffer,
    })
  }, [])

  const closeContextMenu = useCallback(() => {
    setContextMenu((menu) => ({ ...menu, isOpen: false }))
  }, [])

  const closeTab = useCallback(
    (bufferId: string) => {
      handleTabClose(bufferId)
      clearPositionCache(bufferId)
    },
    [clearPositionCache, handleTabClose],
  )

  const handleKeyDown = useTabKeyboardNav({
    sortedBuffers,
    tabRefs,
    onTabClick: handleTabClick,
    onUpdateActivePath: updateActivePath,
    onAnnounce: setSrAnnouncement,
    onCloseTab: closeTab,
  })

  // Keyboard nav needs the index-addressed list, so its handler changes on every flush; a
  // latest-ref keeps the reference handed to tabs stable.
  const handleKeyDownRef = useRef(handleKeyDown)
  handleKeyDownRef.current = handleKeyDown
  const handleTabKeyDown = useCallback(
    (e: React.KeyboardEvent, index: number) => handleKeyDownRef.current(e, index),
    [],
  )

  // One stable ref-callback per index, cached so the prop handed to each
  // SortableEditorTab keeps its identity across renders (a fresh ref callback
  // would make React detach+reattach the node on every render).
  const tabRefCallbacks = useRef(new Map<number, (el: HTMLDivElement | null) => void>())
  const getTabRefCallback = useCallback((index: number) => {
    const cache = tabRefCallbacks.current
    let cb = cache.get(index)
    if (!cb) {
      cb = (el: HTMLDivElement | null) => {
        tabRefs.current[index] = el
      }
      cache.set(index, cb)
    }
    return cb
  }, [])

  const handleContextMenuCloseTab = useCallback(
    (bufferId: string) => {
      const buf = buffers.find((b) => b.id === bufferId)
      if (buf) closeTab(bufferId)
    },
    [buffers, closeTab],
  )

  return (
    <>
      <DndContext
        sensors={sensors}
        collisionDetection={closestCenter}
        onDragStart={handleDragStart}
        onDragEnd={handleDragEnd}
        onDragCancel={resetDrag}
      >
        <PaneTopRow
          rowRef={tabBarRef}
          paneId={paneId ?? ''}
          isBottomPane={isBottomPane}
          isAtLeftEdge={isAtLeftEdge}
          isAtTopEdge={isAtTopEdge}
          // Blurred only while the strip's chat tab is the selected entry.
          variant={chatIsSelected ? 'chat-blur' : 'opaque'}
        >
          {/* The split toggle leads the row, outside the scroller; an empty pane has no chat for
              a second view to sit beside. */}
          {paneId && !isEmptyPane && (
            <SplitToggleButton active={pane?.editorOpen ?? false} onToggle={handleToggleSplit} />
          )}

          {showReopenToggleLeft && (
            <TabNavigationButtons
              isBottomPane={isBottomPane}
              sidebarOpen={sidebarOpen}
              sidebarPosition={sidebarPosition}
              onToggleSidebar={toggleSidebar}
            />
          )}
          {showReadoutLeft && (
            <div className="relative w-0 shrink-0 self-stretch">
              <BuildReadout className="left-2" />
            </div>
          )}

          {/* A pane holding only its chat draws no tab-strip scroller at all
              (spec §7.1) — the second view is reached through open files or
              `+`, never a tab standing for the idea of one. */}
          {hasEditorTabs && (
            <>
              <SortableContext items={sortedBufferIds} strategy={horizontalListSortingStrategy}>
                {/* The empty area of this scroll container is a window drag handle.
                  Tauri only drags on the exact element with the attribute, so the
                  tab children remain interactive and reorderable. */}
                <div
                  ref={scrollRef}
                  data-testid="editor-tab-scroller"
                  role="tablist"
                  aria-label="Open files"
                  data-tauri-drag-region
                  onWheel={handleWheel}
                  className="tab-scrollbar flex min-w-0 flex-1 items-center gap-1.5 overflow-x-auto overflow-y-hidden [overscroll-behavior-x:contain]"
                >
                  {/* The chat as "just another tab" (spec redesign) — first in
                    the strip, outside the sortable list: never draggable,
                    never closable, no reason to reorder the one entry that
                    isn't an editor tab at all. */}
                  {showChatTab && pane?.chatId && (
                    <ChatTabItem
                      chatId={pane.chatId}
                      isActive={chatIsSelected}
                      onSelect={handleChatTabSelect}
                    />
                  )}
                  {sortedBuffers.map((buffer, index) => (
                    <SortableEditorTab
                      key={buffer.id}
                      id={buffer.id}
                      tabRef={getTabRefCallback(index)}
                    >
                      <TabBarItem
                        buffer={buffer}
                        displayName={getBufferDisplayName(buffer)}
                        index={index}
                        isActive={buffer.id === activeBufferId && !chatIsSelected}
                        isDraggedTab={buffer.id === draggedBufferId}
                        onSelect={handleTabSelect}
                        onDoubleClick={handleDoubleClick}
                        onContextMenu={handleContextMenu}
                        onKeyDown={handleTabKeyDown}
                        handleTabClose={closeTab}
                        handleTabPin={handleTabPin}
                      />
                    </SortableEditorTab>
                  ))}

                  {/* Flows immediately after the last tab and shifts as tabs
                    open/close — NOT a SortableEditorTab, so it never joins
                    sortedBufferIds and is never draggable. Last child inside
                    the scroller (spec §7.1). Opens a New Tab page, where
                    what lands in this pane is chosen. */}
                  {paneId && <TabAddButton isBottomPane={isBottomPane} onNewTab={onNewTab} />}
                </div>
              </SortableContext>
              <TabOverflowMenu
                scrollRef={scrollRef}
                tabs={overflowTabs}
                activeId={chatIsSelected ? null : activeBufferId}
                onSelect={handleOverflowSelect}
              />
            </>
          )}

          {/* Shortcut into GitPanel's own "Review this branch" action
              (git-panel.tsx) — branch review's real home stays the git
              file-explorer card; this is just a faster way to reach it from
              the IDE sector's own row. Pinned at the right edge. */}
          {paneId && pane && !isEmptyPane && canReview && (
            <BranchReviewShortcutButton
              isBottomPane={isBottomPane}
              // THIS pane's own workspace — not whichever one happens to be
              // globally active, which is a different pane in a split
              // showing a different chat/branch entirely.
              onOpen={() => openBranchReviewForWorkspace(wsId, paneId)}
            />
          )}

          {showReadoutRight && (
            <div className="relative w-0 shrink-0 self-stretch">
              <BuildReadout className="right-2" />
            </div>
          )}
          {showReopenToggleRight && (
            <TabNavigationButtons
              isBottomPane={isBottomPane}
              sidebarOpen={sidebarOpen}
              sidebarPosition={sidebarPosition}
              onToggleSidebar={toggleSidebar}
            />
          )}
        </PaneTopRow>

        <TabDragPreview buffer={draggedBuffer} />
      </DndContext>

      <TabBarContextMenu
        menu={contextMenu}
        paneId={paneId}
        wsId={wsId}
        onClose={closeContextMenu}
        onPin={handleTabPin}
        onCloseTab={handleContextMenuCloseTab}
      />
      <TabCloseConfirm paneId={paneId} />

      <div role="status" aria-live="polite" aria-atomic="true" className="sr-only">
        {srAnnouncement}
      </div>
    </>
  )
}

export default TabBar
