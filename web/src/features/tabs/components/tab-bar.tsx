import { DndContext, DragOverlay, closestCenter, useSensor, useSensors } from '@dnd-kit/core'
import { SortableContext, horizontalListSortingStrategy } from '@dnd-kit/sortable'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useEditorStateStore } from '@/features/editor/stores/state-store'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
import { usePaneById, usePaneActions } from '@/features/workspace/stores/hooks/use-pane-store'
import { useBufferActions } from '@/features/workspace/stores/hooks/use-buffer-store'
import {
  useWorkspaceStore,
  useWorkspaceStoreContext,
} from '@/features/workspace/stores/workspace-context'
import { splitEditorGroup } from '@/features/panes/utils/pane-command-actions'
import { useSettingsStore } from '@/features/settings/store'
import type { PaneContent } from '@/features/panes/types/pane-content'
import { useEditorAppStore } from '@/features/editor/stores/editor-app-store'
import { useSidebarStore } from '@/features/layout/stores/sidebar-store'
import UnsavedChangesDialog from '@/features/window/components/unsaved-changes-dialog'
import { useSidebar } from '@/components/ui/sidebar'
import { getRelativePath } from '@/utils/path-helpers'
import { cn } from '@/utils/cn'
import { IS_MAC } from '@/utils/platform'
import TabBarItem from './tab-bar-item'
import { sameRenderedBuffer } from './tab-bar-item-utils'
import TabContextMenu from './tab-context-menu'
import TabNavigationButtons from './tab-navigation-buttons'
import TabAddButton from './tab-add-button'
import CloseSplitButton from './close-split-button'
import SortableEditorTab from './sortable-editor-tab'
import { useBufferDisplayName } from '../hooks/use-buffer-display-name'
import { useTabKeyboardNav } from '../hooks/use-tab-keyboard-nav'
import { useTabDrag } from '../hooks/use-tab-drag'
import { useTabBarScroll } from '../hooks/use-tab-bar-scroll'
import { NoDndPointerSensor } from '../lib/no-dnd-pointer-sensor'

const writeText = (text: string) => navigator.clipboard.writeText(text)

/**
 * Subscribes to a pane's buffers PROJECTED to only the fields the tab strip
 * paints — so TabBar re-renders when a tab's *appearance* changes, never on a
 * mere content flush. `s.buffers` is a brand-new array on every keystroke
 * (immer copy-on-write) and the edited buffer gets a fresh identity, so a plain
 * `useBuffersByIds` subscription re-rendered the entire TabBar (sort + remap)
 * on every keystroke in any open file. This selector instead returns the SAME
 * array reference across content flushes and yields a new one only when a
 * rendered field actually moves.
 *
 * The rendered-field set is defined once by `sameRenderedBuffer`
 * (tab-bar-item.tsx) and reused here as the per-buffer equality, so this
 * strip-level gate and TabBarItem's per-tab memo can never fall out of sync.
 * Real `PaneContent` objects are returned (not a reduced tuple) because the tab
 * strip's own hooks — drag, keyboard-nav, display-name — need the full buffer;
 * they only ever read rendered fields, so returning a stale-but-rendered-equal
 * object is safe. Handlers that need live non-rendered fields (e.g. reload,
 * which reads `content`) read `workspaceStore.getState()` instead.
 */
function useRenderedPaneBuffers(paneBufferIds: string[]): PaneContent[] {
  const prev = useRef<PaneContent[]>([])
  return useWorkspaceStoreContext((s) => {
    const map = new Map(s.buffers.map((b) => [b.id, b]))
    const next: PaneContent[] = []
    for (const id of paneBufferIds) {
      const b = map.get(id)
      if (b) next.push(b)
    }
    const p = prev.current
    if (p.length === next.length && next.every((b, i) => sameRenderedBuffer(p[i], b))) {
      return p
    }
    prev.current = next
    return next
  })
}

interface TabBarProps {
  paneId?: string
  onTabClick?: (bufferId: string) => void
  disablePaneActions?: boolean
}

// react-doctor-disable-next-line no-giant-component -- accepted: cohesive tab strip — drag/reorder, overflow scroll and active-tab tracking share one dnd context and scroll ref.
const TabBar = ({
  paneId,
  onTabClick: externalTabClick,
  disablePaneActions = false,
}: TabBarProps) => {
  const globalActiveBufferId = useWorkspaceStoreContext(
    (s) => s.paneActions.getActivePane()?.activeBufferId ?? null,
  )
  const pendingClose = useWorkspaceStoreContext((s) => s.pendingClose)
  const pane = usePaneById(paneId ?? '')
  const {
    closePane,
    setActivePane,
    activatePaneBuffer,
    removeBufferFromPane,
    splitPane,
    moveBufferToPane,
    reorderPaneBuffers,
  } = usePaneActions()
  const {
    closeBuffer,
    openContent,
    openNewTab,
    promotePreview: promotePreviewBuffer,
    setPinned,
    confirmPendingClose,
    setPendingClose,
  } = useBufferActions()
  const workspaceStore = useWorkspaceStore()
  const paneBufferIds = pane?.bufferIds ?? []
  // Projected, rendered-field-gated subscription (see useRenderedPaneBuffers):
  // TabBar no longer re-renders on content flushes, only on tab-appearance
  // changes.
  const buffers = useRenderedPaneBuffers(paneBufferIds)
  const activeBufferCandidate = pane ? pane.activeBufferId : globalActiveBufferId
  const activeBufferId =
    activeBufferCandidate && buffers.some((buffer) => buffer.id === activeBufferCandidate)
      ? activeBufferCandidate
      : null

  const handleTabPin = useCallback(
    (bufferId: string) => {
      const buf = workspaceStore.getState().buffers.find((b) => b.id === bufferId)
      if (buf) setPinned(bufferId, !buf.isPinned)
    },
    [workspaceStore, setPinned],
  )
  function handleCloseOtherTabs(keepBufferId: string) {
    const { buffers: allBufs } = workspaceStore.getState()
    // isUncloseable filters out the sole New Tab a pane is holding — closing it
    // would just respawn another (see pane-slice's removeBufferFromPane) and, in
    // the single-pane case, strand the pane tab-less in the meantime (I2).
    const toClose = allBufs.filter((b) => b.id !== keepBufferId && !b.isPinned && !b.isUncloseable)
    toClose.forEach((b) => {
      if (paneId) removeBufferFromPane(paneId, b.id)
      closeBuffer(b.id)
    })
  }
  function handleCloseAllTabs() {
    const { buffers: allBufs } = workspaceStore.getState()
    // Same isUncloseable guard as handleCloseOtherTabs (I2). NOTE: this still
    // iterates every buffer in the WORKSPACE while only ever removing from the
    // current pane (paneId) — a separate, pre-existing bug this fix does not
    // attempt, since narrowing the buffer set is a larger behavior change than
    // the uncloseable-tab regression it was found alongside.
    const toClose = allBufs.filter((b) => !b.isPinned && !b.isUncloseable)
    toClose.forEach((b) => {
      if (paneId) removeBufferFromPane(paneId, b.id)
      closeBuffer(b.id)
    })
  }
  function handleCloseTabsToRight(bufferId: string) {
    const { buffers: allBufs } = workspaceStore.getState()
    const idx = allBufs.findIndex((b) => b.id === bufferId)
    if (idx === -1) return
    const toClose = allBufs.slice(idx + 1).filter((b) => !b.isPinned && !b.isUncloseable)
    toClose.forEach((b) => {
      if (paneId) removeBufferFromPane(paneId, b.id)
      closeBuffer(b.id)
    })
  }
  const reorderBuffers = useCallback(
    (startIndex: number, endIndex: number) => {
      if (paneId) reorderPaneBuffers(paneId, startIndex, endIndex)
    },
    [paneId, reorderPaneBuffers],
  )
  const confirmCloseWithoutSaving = confirmPendingClose
  // Stable identity: handleCancelClose memoizes on this, so a fresh function each
  // render would defeat that memo. setPendingClose is a stable store action.
  const cancelPendingClose = useCallback(() => setPendingClose(null), [setPendingClose])
  const convertPreviewToDefinite = promotePreviewBuffer

  const handleTabClick = useCallback(
    (bufferId: string) => {
      if (paneId) {
        activatePaneBuffer(paneId, bufferId)
        setActivePane(paneId)
      }
      externalTabClick?.(bufferId)
    },
    [activatePaneBuffer, externalTabClick, paneId, setActivePane],
  )

  const handleTabClose = useCallback(
    (bufferId: string) => {
      const buf = workspaceStore.getState().buffers.find((b) => b.id === bufferId)
      // The single choke point every close affordance funnels through (× button,
      // middle-click's handleAuxClick, the context menu's single "Close", and
      // Delete/Backspace in keyboard nav all call this via `closeTab`). A pane's
      // sole New Tab has no close affordance at all — closing it would just
      // respawn another (pane-slice's removeBufferFromPane) and strand the pane
      // tab-less in between (I2) — so honour the same flag the × button's
      // visibility already does, here too.
      if (buf?.isUncloseable) return
      if (buf && buf.type === 'editor' && buf.isDirty) {
        setPendingClose({ type: 'single', bufferId })
        return
      }
      if (paneId) removeBufferFromPane(paneId, bufferId)
      closeBuffer(bufferId)
    },
    [closeBuffer, paneId, removeBufferFromPane, setPendingClose, workspaceStore],
  )

  const { handleSave } = useEditorAppStore.use.actions()
  const updateActivePath = useSidebarStore((s) => s.updateActivePath)
  const sidebarPosition = useSettingsStore((s) => s.settings.sidebarPosition)
  const { open: sidebarOpen, toggleSidebar } = useSidebar()
  const rootFolderPath = useFileSystemStore.use.rootFolderPath?.() || undefined
  // Subscribe to the DERIVED count, not the whole `panes` record: a number is
  // referentially stable, so TabBar no longer re-renders on every pane mutation
  // (another pane's active-buffer swap, buffer add/remove, etc.) — only when the
  // number of main panes actually changes.
  const mainPaneCount = useWorkspaceStoreContext(
    (s) => Object.keys(s.panes).filter((id) => id !== BOTTOM_PANE_ID).length,
  )
  const isInSplit = pane !== null && paneId !== null && mainPaneCount > 1
  const isBottomPane = paneId === BOTTOM_PANE_ID

  const [contextMenu, setContextMenu] = useState<{
    isOpen: boolean
    position: { x: number; y: number }
    buffer: PaneContent | null
  }>({ isOpen: false, position: { x: 0, y: 0 }, buffer: null })

  const [srAnnouncement, setSrAnnouncement] = useState<string>('')

  const tabRefs = useRef<(HTMLDivElement | null)[]>([])

  const handleRevealInFolder = useFileSystemStore.use.handleRevealInFolder?.()
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

  const handleTabSelect = useCallback(
    (buffer: PaneContent) => {
      if (externalTabClick) {
        externalTabClick(buffer.id)
      } else {
        handleTabClick(buffer.id)
      }
      updateActivePath(buffer.path)
      setSrAnnouncement(
        `Switched to ${buffer.name}${buffer.type === 'editor' && buffer.isDirty ? ', unsaved changes' : ''}`,
      )
    },
    [externalTabClick, handleTabClick, updateActivePath],
  )

  const {
    draggedBufferId,
    draggedBuffer,
    handleDragStart,
    handleDragMove,
    handleDragEnd,
    resetDrag,
  } = useTabDrag({
    paneId,
    sortedBuffers,
    onTabSelect: handleTabSelect,
    onTabClick: handleTabClick,
    onReorderBuffers: reorderBuffers,
    onMoveBufferToPane: moveBufferToPane,
    onActivatePaneBuffer: activatePaneBuffer,
    onSplitPane: (targetPaneId, direction, bufferId, placement) =>
      splitPane(targetPaneId, direction, bufferId, placement) ?? undefined,
  })

  const { tabBarRef, isAtLeftEdge, isAtRightEdge, isAtTopEdge, handleWheel } = useTabBarScroll({
    sidebarPosition,
    draggedBufferId,
  })

  // The tab-bar sidebar-toggle is only a fallback for reopening a collapsed
  // sidebar (when open, the sidebar header owns the toggle). It appears on the
  // window-edge pane matching the configured sidebar side.
  const showReopenToggleLeft =
    !sidebarOpen && !isBottomPane && sidebarPosition === 'left' && isAtLeftEdge
  const showReopenToggleRight =
    !sidebarOpen && !isBottomPane && sidebarPosition === 'right' && isAtRightEdge

  const getBufferDisplayName = useBufferDisplayName({ buffers, rootFolderPath })

  // The max-open-tabs cap is NOT enforced here any more. It lives in
  // buffer-slice's openContent, next to the engine's own budget and the
  // eviction that budget already runs — see the note there. A tab bar watching
  // the workspace-wide buffer list and closing tabs through its own pane's
  // handler meant every open pane raced to trim one shared list.

  // Auto-scroll active tab into view
  useEffect(() => {
    const activeIndex = sortedBuffers.findIndex((buffer) => buffer.id === activeBufferId)
    if (activeIndex !== -1 && tabRefs.current[activeIndex] && tabBarRef.current) {
      const activeTab = tabRefs.current[activeIndex]
      const container = tabBarRef.current
      if (activeTab) {
        const tabRect = activeTab.getBoundingClientRect()
        const containerRect = container.getBoundingClientRect()
        if (tabRect.left < containerRect.left || tabRect.right > containerRect.right) {
          activeTab.scrollIntoView({ behavior: 'smooth', block: 'nearest', inline: 'center' })
        }
      }
    }
  }, [activeBufferId, sortedBuffers, tabBarRef])

  useEffect(() => {
    tabRefs.current = tabRefs.current.slice(0, sortedBuffers.length)
  }, [sortedBuffers.length])

  // Takes the item's own buffer (not an index into sortedBuffers) so its deps
  // stay stable across content flushes — sortedBuffers is rebuilt on every
  // keystroke, and closing over it here would give every tab a fresh handler
  // reference and defeat TabBarItem's memo.
  const handleDoubleClick = useCallback(
    (e: React.MouseEvent, buffer: PaneContent) => {
      e.preventDefault()
      e.stopPropagation()
      if (buffer.isPreview) {
        convertPreviewToDefinite(buffer.id)
        promotePreviewBuffer(buffer.id)
      }
    },
    [convertPreviewToDefinite, promotePreviewBuffer],
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

  const handleCopyPath = useCallback(async (path: string) => {
    await writeText(path)
  }, [])

  const handleCopyRelativePath = useCallback(
    async (path: string) => {
      if (!rootFolderPath) {
        await writeText(path)
        return
      }
      await writeText(getRelativePath(path, rootFolderPath))
    },
    [rootFolderPath],
  )

  const closeContextMenu = useCallback(() => {
    setContextMenu({ isOpen: false, position: { x: 0, y: 0 }, buffer: null })
  }, [])

  const handleSaveAndClose = useCallback(async () => {
    if (!pendingClose) return
    const buffer = buffers.find((b) => b.id === pendingClose.bufferId)
    if (!buffer) return
    await handleSave()
    if (paneId) removeBufferFromPane(paneId, pendingClose.bufferId)
    confirmCloseWithoutSaving()
  }, [pendingClose, buffers, handleSave, confirmCloseWithoutSaving, paneId, removeBufferFromPane])

  const handleDiscardAndClose = useCallback(() => {
    if (!pendingClose) return
    if (paneId) removeBufferFromPane(paneId, pendingClose.bufferId)
    confirmCloseWithoutSaving()
  }, [confirmCloseWithoutSaving, pendingClose, paneId, removeBufferFromPane])

  const handleCancelClose = useCallback(() => {
    cancelPendingClose()
  }, [cancelPendingClose])

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

  // Keyboard nav genuinely needs the whole (index-addressed) buffer list to find
  // neighbours, so useTabKeyboardNav's callback is rebuilt whenever sortedBuffers
  // changes — i.e. on every content flush. Route it through a latest-ref so the
  // reference handed to every tab stays stable (memo-friendly) while still
  // invoking the up-to-date logic.
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

  const MemoizedTabContextMenu = useMemo(() => TabContextMenu, [])

  const handleContextMenuCloseTab = useCallback(
    (bufferId: string) => {
      const buf = buffers.find((b) => b.id === bufferId)
      if (buf) closeTab(bufferId)
    },
    [buffers, closeTab],
  )

  const handleReloadTab = useCallback(
    (bufferId: string) => {
      // Read from getState(), not the rendered-field-gated `buffers`: reload
      // needs the buffer's LIVE `content`, which that projection deliberately
      // does not track (it can hold a content-stale object reference).
      const buf = workspaceStore.getState().buffers.find((b) => b.id === bufferId)
      if (buf && buf.path !== 'extensions://marketplace') {
        if (paneId) removeBufferFromPane(paneId, bufferId)
        closeBuffer(bufferId)
        setTimeout(async () => {
          try {
            const content = buf.type === 'editor' || buf.type === 'diff' ? buf.content : ''
            const spec =
              buf.type === 'diff'
                ? ({ type: 'diff', path: buf.path, name: buf.name, content } as const)
                : ({ type: 'editor', path: buf.path, name: buf.name, content } as const)
            openContent(spec)
          } catch (error) {
            console.error('Failed to reload buffer:', error)
          }
        }, 100)
      }
    },
    [workspaceStore, closeBuffer, openContent, paneId, removeBufferFromPane],
  )

  const handleSplitRight = useMemo(
    () =>
      paneId
        ? (targetPaneId: string, bufferId: string) =>
            splitEditorGroup(targetPaneId, 'horizontal', bufferId)
        : undefined,
    [paneId],
  )

  const handleSplitDown = useMemo(
    () =>
      paneId
        ? (targetPaneId: string, bufferId: string) =>
            splitEditorGroup(targetPaneId, 'vertical', bufferId)
        : undefined,
    [paneId],
  )

  return (
    <>
      <DndContext
        sensors={sensors}
        collisionDetection={closestCenter}
        onDragStart={handleDragStart}
        onDragMove={handleDragMove}
        onDragEnd={handleDragEnd}
        onDragCancel={resetDrag}
      >
        <div
          ref={tabBarRef}
          data-tab-bar-pane-id={paneId ?? ''}
          className={cn(
            'relative flex shrink-0 items-center gap-1.5 overflow-hidden px-2 py-1',
            IS_MAC ? 'h-[44px]' : 'h-[34px]',
            // Traffic-light inset: only the tab bar that actually sits under
            // the macOS window controls (window top-left) reserves the space —
            // in a vertical split the lower pane is at the left edge too but
            // nowhere near the traffic lights.
            IS_MAC && !isBottomPane && isAtLeftEdge && isAtTopEdge && 'pl-[88px]',
          )}
          role="tablist"
          aria-label="Open files"
          data-tauri-drag-region
          onWheel={handleWheel}
        >
          {showReopenToggleLeft && (
            <TabNavigationButtons
              isBottomPane={isBottomPane}
              sidebarOpen={sidebarOpen}
              sidebarPosition={sidebarPosition}
              onToggleSidebar={toggleSidebar}
            />
          )}

          <SortableContext items={sortedBufferIds} strategy={horizontalListSortingStrategy}>
            {/* The empty area of this scroll container is a window drag handle.
                Tauri only drags on the exact element with the attribute, so the
                tab children remain interactive and reorderable. */}
            <div
              data-tauri-drag-region
              className="tab-scrollbar flex min-w-0 flex-1 items-center gap-1.5 overflow-x-auto overflow-y-hidden [overscroll-behavior-x:contain]"
            >
              {sortedBuffers.map((buffer, index) => (
                <SortableEditorTab key={buffer.id} id={buffer.id} tabRef={getTabRefCallback(index)}>
                  <TabBarItem
                    buffer={buffer}
                    displayName={getBufferDisplayName(buffer)}
                    index={index}
                    isActive={buffer.id === activeBufferId}
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
                  sortedBufferIds and is never draggable. */}
              {paneId && (
                <TabAddButton
                  isBottomPane={isBottomPane}
                  onNewTab={() => {
                    setActivePane(paneId)
                    openNewTab(paneId)
                  }}
                />
              )}
            </div>
          </SortableContext>

          {/* A pane action, not a tab action — stays pinned at the right
              edge, outside the scrolling tab container. */}
          {paneId && (
            <CloseSplitButton
              isBottomPane={isBottomPane}
              disablePaneActions={disablePaneActions}
              isInSplit={isInSplit}
              onClosePane={() => closePane(paneId)}
            />
          )}

          {showReopenToggleRight && (
            <TabNavigationButtons
              isBottomPane={isBottomPane}
              sidebarOpen={sidebarOpen}
              sidebarPosition={sidebarPosition}
              onToggleSidebar={toggleSidebar}
            />
          )}
        </div>

        <DragOverlay dropAnimation={null}>
          {draggedBuffer ? (
            <div className="tab-drag-preview ui-font flex items-center gap-1.5 rounded-lg border border-border/70 bg-background/95 px-2 py-1 ui-text-xs opacity-95 shadow-sm">
              <span className="max-w-[200px] truncate text-foreground">
                {draggedBuffer.name}
                {draggedBuffer.type === 'editor' && draggedBuffer.isDirty && (
                  <span className="ml-1 text-muted-foreground">•</span>
                )}
              </span>
            </div>
          ) : null}
        </DragOverlay>
      </DndContext>

      <MemoizedTabContextMenu
        isOpen={contextMenu.isOpen}
        position={contextMenu.position}
        buffer={contextMenu.buffer}
        paneId={paneId}
        onClose={closeContextMenu}
        onPin={handleTabPin}
        onCloseTab={handleContextMenuCloseTab}
        onCloseOthers={handleCloseOtherTabs}
        onCloseAll={handleCloseAllTabs}
        onCloseToRight={handleCloseTabsToRight}
        onCopyPath={handleCopyPath}
        onCopyRelativePath={handleCopyRelativePath}
        onReload={handleReloadTab}
        onRevealInFinder={handleRevealInFolder ?? undefined}
        onSplitRight={handleSplitRight}
        onSplitDown={handleSplitDown}
      />

      {pendingClose && (
        <UnsavedChangesDialog
          fileName={buffers.find((b) => b.id === pendingClose.bufferId)?.name || ''}
          onSave={handleSaveAndClose}
          onDiscard={handleDiscardAndClose}
          onCancel={handleCancelClose}
        />
      )}

      <div role="status" aria-live="polite" aria-atomic="true" className="sr-only">
        {srAnnouncement}
      </div>
    </>
  )
}

export default TabBar
