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
  wsId?: string | null
  onTabClick?: (bufferId: string) => void
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
  const buffers = useRenderedPaneBuffers(paneBufferIds)
  const activeBufferCandidate = pane ? pane.activeEditorTabId : globalActiveBufferId
  const activeBufferId =
    activeBufferCandidate && buffers.some((buffer) => buffer.id === activeBufferCandidate)
      ? activeBufferCandidate
      : null
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
      if (buf?.isUncloseable) return
      if (buf && buf.type === 'editor' && buf.isDirty) {
        if (paneId) setPendingClose({ bufferId, paneId })
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
  const isEmptyPane = pane !== null && !pane.chatId && !hasEditorTabs

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

  const showReopenToggleLeft =
    !sidebarOpen && !isBottomPane && sidebarPosition === 'left' && isAtLeftEdge && isAtTopEdge
  const showReopenToggleRight =
    !sidebarOpen && !isBottomPane && sidebarPosition === 'right' && isAtRightEdge && isAtTopEdge

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

  const handleKeyDownRef = useRef(handleKeyDown)
  handleKeyDownRef.current = handleKeyDown
  const handleTabKeyDown = useCallback(
    (e: React.KeyboardEvent, index: number) => handleKeyDownRef.current(e, index),
    [],
  )

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
          variant={chatIsSelected ? 'chat-blur' : 'opaque'}
        >
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

          {hasEditorTabs && (
            <>
              <SortableContext items={sortedBufferIds} strategy={horizontalListSortingStrategy}>
                <div
                  ref={scrollRef}
                  data-testid="editor-tab-scroller"
                  role="tablist"
                  aria-label="Open files"
                  data-tauri-drag-region
                  onWheel={handleWheel}
                  className="tab-scrollbar flex min-w-0 flex-1 items-center gap-1.5 overflow-x-auto overflow-y-hidden [overscroll-behavior-x:contain]"
                >
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

          {paneId && pane && !isEmptyPane && canReview && (
            <BranchReviewShortcutButton
              isBottomPane={isBottomPane}
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
