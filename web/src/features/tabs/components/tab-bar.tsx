import {
  DndContext,
  DragOverlay,
  closestCenter,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import { SortableContext, horizontalListSortingStrategy } from "@dnd-kit/sortable";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { nanoid } from "nanoid";
import { useEditorStateStore } from "@/features/editor/stores/state-store";
import { useFileSystemStore } from "@/features/file-system/controllers/store";
import { BOTTOM_PANE_ID } from "@/features/panes/constants/pane";
import {
  usePaneById,
  usePaneActions,
} from "@/features/workspace/stores/hooks/use-pane-store";
import { useBuffers, useBuffersByIds, useBufferActions } from "@/features/workspace/stores/hooks/use-buffer-store";
import { useWorkspaceStore, useWorkspaceStoreContext } from "@/features/workspace/stores/workspace-context";
import { splitEditorGroup } from "@/features/panes/utils/pane-command-actions";
import { useSettingsStore } from "@/features/settings/store";
import type { PaneContent } from "@/features/panes/types/pane-content";
import { useEditorAppStore } from "@/features/editor/stores/editor-app-store";
import { useSidebarStore } from "@/features/layout/stores/sidebar-store";
import { useWebViewerNavigationStore } from "@/features/web-viewer/stores/web-viewer-navigation-store";
import UnsavedChangesDialog from "@/features/window/components/unsaved-changes-dialog";
import { useSidebar } from "@/components/ui/sidebar";
import { getRelativePath } from "@/utils/path-helpers";
import { cn } from "@/utils/cn";
import { IS_MAC } from "@/utils/platform";
import TabBarItem from "./tab-bar-item";
import TabContextMenu from "./tab-context-menu";
import TabNavigationButtons from "./tab-navigation-buttons";
import TabNewButton from "./tab-new-button";
import SortableEditorTab from "./sortable-editor-tab";
import { useBufferDisplayName } from "../hooks/use-buffer-display-name";
import { useTabKeyboardNav } from "../hooks/use-tab-keyboard-nav";
import { useJumpNavigation } from "../hooks/use-jump-navigation";
import { useTabDrag } from "../hooks/use-tab-drag";
import { useTabBarScroll } from "../hooks/use-tab-bar-scroll";
import { NoDndPointerSensor } from "../lib/no-dnd-pointer-sensor";

const writeText = (text: string) => navigator.clipboard.writeText(text);

interface TabBarProps {
  paneId?: string;
  onTabClick?: (bufferId: string) => void;
  disablePaneActions?: boolean;
}

const TabBar = ({
  paneId,
  onTabClick: externalTabClick,
  disablePaneActions = false,
}: TabBarProps) => {
  const globalActiveBufferId = useWorkspaceStoreContext(s => s.paneActions.getActivePane()?.activeBufferId ?? null);
  const pendingClose = useWorkspaceStoreContext((s) => s.pendingClose);
  const pane = usePaneById(paneId ?? '');
  const { closePane, setActivePane, activatePaneBuffer, removeBufferFromPane, splitPane, moveBufferToPane, reorderPaneBuffers } = usePaneActions();
  const { closeBuffer, openContent, promotePreview: promotePreviewBuffer, setPinned, confirmPendingClose, setPendingClose } = useBufferActions();
  const workspaceStore = useWorkspaceStore();
  const allBuffers = useBuffers();
  const paneBufferIds = pane?.bufferIds ?? [];
  const paneSpecificBuffers = useBuffersByIds(paneBufferIds);
  const buffers = useMemo(
    () =>
      (pane ? paneSpecificBuffers : allBuffers).filter((buffer) => buffer.type !== "newTab"),
    [pane, paneSpecificBuffers, allBuffers],
  );
  const activeBufferCandidate = pane ? pane.activeBufferId : globalActiveBufferId;
  const activeBufferId =
    activeBufferCandidate && buffers.some((buffer) => buffer.id === activeBufferCandidate)
      ? activeBufferCandidate
      : null;

  function handleTabPin(bufferId: string) {
    const buf = workspaceStore.getState().buffers.find((b) => b.id === bufferId);
    if (buf) setPinned(bufferId, !buf.isPinned);
  }
  function handleCloseOtherTabs(keepBufferId: string) {
    const { buffers: allBufs } = workspaceStore.getState();
    const toClose = allBufs.filter((b) => b.id !== keepBufferId && !b.isPinned);
    toClose.forEach((b) => {
      if (paneId) removeBufferFromPane(paneId, b.id);
      closeBuffer(b.id);
    });
  }
  function handleCloseAllTabs() {
    const { buffers: allBufs } = workspaceStore.getState();
    const toClose = allBufs.filter((b) => !b.isPinned);
    toClose.forEach((b) => {
      if (paneId) removeBufferFromPane(paneId, b.id);
      closeBuffer(b.id);
    });
  }
  function handleCloseTabsToRight(bufferId: string) {
    const { buffers: allBufs } = workspaceStore.getState();
    const idx = allBufs.findIndex((b) => b.id === bufferId);
    if (idx === -1) return;
    const toClose = allBufs.slice(idx + 1).filter((b) => !b.isPinned);
    toClose.forEach((b) => {
      if (paneId) removeBufferFromPane(paneId, b.id);
      closeBuffer(b.id);
    });
  }
  const reorderBuffers = useCallback(
    (startIndex: number, endIndex: number) => {
      if (paneId) reorderPaneBuffers(paneId, startIndex, endIndex);
    },
    [paneId, reorderPaneBuffers],
  );
  const confirmCloseWithoutSaving = confirmPendingClose;
  const cancelPendingClose = () => setPendingClose(null);
  const convertPreviewToDefinite = promotePreviewBuffer;

  const handleTabClick = useCallback(
    (bufferId: string) => {
      if (paneId) {
        activatePaneBuffer(paneId, bufferId);
        setActivePane(paneId);
      }
      externalTabClick?.(bufferId);
    },
    [activatePaneBuffer, externalTabClick, paneId, setActivePane],
  );

  const handleTabClose = useCallback(
    (bufferId: string) => {
      const buf = workspaceStore.getState().buffers.find((b) => b.id === bufferId);
      if (buf && buf.type === 'editor' && buf.isDirty) {
        setPendingClose({ type: 'single', bufferId });
        return;
      }
      if (paneId) removeBufferFromPane(paneId, bufferId);
      closeBuffer(bufferId);
    },
    [closeBuffer, paneId, removeBufferFromPane, setPendingClose, workspaceStore],
  );

  const { handleSave } = useEditorAppStore.use.actions();
  const maxOpenTabs = useSettingsStore((state) => state.settings.maxOpenTabs);
  const updateActivePath = useSidebarStore((s) => s.updateActivePath);
  const sidebarPosition = useSettingsStore((s) => s.settings.sidebarPosition);
  const { open: sidebarOpen, toggleSidebar } = useSidebar();
  const rootFolderPath = useFileSystemStore.use.rootFolderPath?.() || undefined;
  const activeBuffer = useMemo(
    () => buffers.find((buffer) => buffer.id === activeBufferId) ?? null,
    [activeBufferId, buffers],
  );
  const activeWebViewerNavigation = useWebViewerNavigationStore((state) =>
    activeBuffer?.type === "webViewer" ? state.navigationByBufferId[activeBuffer.id] : undefined,
  );
  const usesWebViewerNavigation = activeBuffer?.type === "webViewer";
  const { canGoBack, canGoForward, handleJumpBack, handleJumpForward } = useJumpNavigation({
    usesWebViewerNavigation,
    activeWebViewerNavigation,
  });
  const allPanes = useWorkspaceStoreContext(s => s.panes)
  const mainPaneCount = Object.keys(allPanes).filter(id => id !== BOTTOM_PANE_ID).length
  const isInSplit = pane !== null && paneId !== null && mainPaneCount > 1
  const isBottomPane = paneId === BOTTOM_PANE_ID;

  const [contextMenu, setContextMenu] = useState<{
    isOpen: boolean;
    position: { x: number; y: number };
    buffer: PaneContent | null;
  }>({ isOpen: false, position: { x: 0, y: 0 }, buffer: null });

  const [srAnnouncement, setSrAnnouncement] = useState<string>("");

  const tabRefs = useRef<(HTMLDivElement | null)[]>([]);

  const handleRevealInFolder = useFileSystemStore.use.handleRevealInFolder?.();
  const { clearPositionCache } = useEditorStateStore.getState().actions;

  const sensors = useSensors(
    useSensor(NoDndPointerSensor, {
      activationConstraint: { distance: 6 },
    }),
  );

  const sortedBuffers = useMemo(() => {
    return [...buffers].sort((a, b) => {
      if (a.isPinned && !b.isPinned) return -1;
      if (!a.isPinned && b.isPinned) return 1;
      return 0;
    });
  }, [buffers]);
  const sortedBufferIds = useMemo(() => sortedBuffers.map((buffer) => buffer.id), [sortedBuffers]);

  const handleTabSelect = useCallback(
    (buffer: PaneContent) => {
      if (externalTabClick) {
        externalTabClick(buffer.id);
      } else {
        handleTabClick(buffer.id);
      }
      updateActivePath(buffer.path);
      setSrAnnouncement(
        `Switched to ${buffer.name}${buffer.type === "editor" && buffer.isDirty ? ", unsaved changes" : ""}`,
      );
    },
    [externalTabClick, handleTabClick, updateActivePath],
  );

  const { draggedBufferId, draggedBuffer, handleDragStart, handleDragMove, handleDragEnd, resetDrag } = useTabDrag({
    paneId,
    sortedBuffers,
    onTabSelect: handleTabSelect,
    onTabClick: handleTabClick,
    onReorderBuffers: reorderBuffers,
    onMoveBufferToPane: moveBufferToPane,
    onActivatePaneBuffer: activatePaneBuffer,
    onSplitPane: (targetPaneId, direction, bufferId, placement) =>
      splitPane(targetPaneId, direction, bufferId, placement) ?? undefined,
  });

  const { tabBarRef, isAtLeftEdge, handleWheel } = useTabBarScroll({
    sidebarPosition,
    draggedBufferId,
  });

  const getBufferDisplayName = useBufferDisplayName({ buffers, rootFolderPath });

  useEffect(() => {
    if (maxOpenTabs > 0 && buffers.length > maxOpenTabs) {
      const closableBuffers = buffers.filter((b) => !b.isPinned && b.id !== activeBufferId);
      let tabsToClose = buffers.length - maxOpenTabs;
      for (let i = 0; i < closableBuffers.length && tabsToClose > 0; i++) {
        handleTabClose(closableBuffers[i].id);
        tabsToClose--;
      }
    }
  }, [buffers, maxOpenTabs, activeBufferId, handleTabClose]);

  // Auto-scroll active tab into view
  useEffect(() => {
    const activeIndex = sortedBuffers.findIndex((buffer) => buffer.id === activeBufferId);
    if (activeIndex !== -1 && tabRefs.current[activeIndex] && tabBarRef.current) {
      const activeTab = tabRefs.current[activeIndex];
      const container = tabBarRef.current;
      if (activeTab) {
        const tabRect = activeTab.getBoundingClientRect();
        const containerRect = container.getBoundingClientRect();
        if (tabRect.left < containerRect.left || tabRect.right > containerRect.right) {
          activeTab.scrollIntoView({ behavior: "smooth", block: "nearest", inline: "center" });
        }
      }
    }
  }, [activeBufferId, sortedBuffers, tabBarRef]);

  useEffect(() => {
    tabRefs.current = tabRefs.current.slice(0, sortedBuffers.length);
  }, [sortedBuffers.length]);

  const handleDoubleClick = useCallback(
    (e: React.MouseEvent, index: number) => {
      e.preventDefault();
      e.stopPropagation();
      const buffer = sortedBuffers[index];
      if (!buffer) return;
      if (buffer.isPreview) {
        convertPreviewToDefinite(buffer.id);
        promotePreviewBuffer(buffer.id);
      }
    },
    [sortedBuffers, convertPreviewToDefinite, promotePreviewBuffer],
  );

  const handleContextMenu = useCallback((e: React.MouseEvent, buffer: PaneContent) => {
    e.preventDefault();
    const target = e.currentTarget as HTMLElement;
    const rect = target.getBoundingClientRect();
    setContextMenu({
      isOpen: true,
      position: { x: rect.left + rect.width * 0.5, y: rect.bottom + 4 },
      buffer,
    });
  }, []);

  const handleCopyPath = useCallback(async (path: string) => {
    await writeText(path);
  }, []);

  const handleCopyRelativePath = useCallback(async (path: string) => {
    if (!rootFolderPath) {
      await writeText(path);
      return;
    }
    await writeText(getRelativePath(path, rootFolderPath));
  }, [rootFolderPath]);

  const closeContextMenu = useCallback(() => {
    setContextMenu({ isOpen: false, position: { x: 0, y: 0 }, buffer: null });
  }, []);

  const handleSaveAndClose = useCallback(async () => {
    if (!pendingClose) return;
    const buffer = buffers.find((b) => b.id === pendingClose.bufferId);
    if (!buffer) return;
    await handleSave();
    if (paneId) removeBufferFromPane(paneId, pendingClose.bufferId);
    confirmCloseWithoutSaving();
  }, [pendingClose, buffers, handleSave, confirmCloseWithoutSaving, paneId, removeBufferFromPane]);

  const handleDiscardAndClose = useCallback(() => {
    if (!pendingClose) return;
    if (paneId) removeBufferFromPane(paneId, pendingClose.bufferId);
    confirmCloseWithoutSaving();
  }, [confirmCloseWithoutSaving, pendingClose, paneId, removeBufferFromPane]);

  const handleCancelClose = useCallback(() => {
    cancelPendingClose();
  }, [cancelPendingClose]);

  const closeTab = useCallback(
    (bufferId: string) => {
      handleTabClose(bufferId);
      clearPositionCache(bufferId);
    },
    [clearPositionCache, handleTabClose],
  );

  const handleKeyDown = useTabKeyboardNav({
    sortedBuffers,
    tabRefs,
    onTabClick: handleTabClick,
    onUpdateActivePath: updateActivePath,
    onAnnounce: setSrAnnouncement,
    onCloseTab: closeTab,
  });

  const MemoizedTabContextMenu = useMemo(() => TabContextMenu, []);

  const handleContextMenuCloseTab = useCallback(
    (bufferId: string) => {
      const buf = buffers.find((b) => b.id === bufferId);
      if (buf) closeTab(bufferId);
    },
    [buffers, closeTab],
  );

  const handleReloadTab = useCallback(
    (bufferId: string) => {
      const buf = buffers.find((b) => b.id === bufferId);
      if (buf && buf.path !== "extensions://marketplace") {
        if (paneId) removeBufferFromPane(paneId, bufferId);
        closeBuffer(bufferId);
        setTimeout(async () => {
          try {
            const content = buf.type === "editor" || buf.type === "diff" ? buf.content : "";
            const spec =
              buf.type === "diff"
                ? ({ type: "diff", path: buf.path, name: buf.name, content } as const)
                : ({ type: "editor", path: buf.path, name: buf.name, content } as const);
            openContent(spec);
          } catch (error) {
            console.error("Failed to reload buffer:", error);
          }
        }, 100);
      }
    },
    [buffers, closeBuffer, openContent, paneId, removeBufferFromPane],
  );

  const handleSplitRight = useMemo(
    () =>
      paneId
        ? (targetPaneId: string, bufferId: string) => splitEditorGroup(targetPaneId, "horizontal", bufferId)
        : undefined,
    [paneId],
  );

  const handleSplitDown = useMemo(
    () =>
      paneId
        ? (targetPaneId: string, bufferId: string) => splitEditorGroup(targetPaneId, "vertical", bufferId)
        : undefined,
    [paneId],
  );

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
          data-tab-bar-pane-id={paneId ?? ""}
          className={cn(
            'relative flex shrink-0 items-center gap-1.5 overflow-hidden px-2 py-1',
            IS_MAC ? 'h-[44px]' : 'h-[34px]',
            IS_MAC && !isBottomPane && isAtLeftEdge && 'pl-[80px]',
          )}
          role="tablist"
          aria-label="Open files"
          data-tauri-drag-region
          onWheel={handleWheel}
        >
          <TabNavigationButtons
            isBottomPane={isBottomPane}
            sidebarOpen={sidebarOpen}
            sidebarPosition={sidebarPosition}
            canGoBack={canGoBack}
            canGoForward={canGoForward}
            onToggleSidebar={toggleSidebar}
            onJumpBack={handleJumpBack}
            onJumpForward={handleJumpForward}
          />

          <SortableContext items={sortedBufferIds} strategy={horizontalListSortingStrategy}>
            <div className="tab-scrollbar flex min-w-0 flex-1 items-center gap-1.5 overflow-x-auto overflow-y-hidden [overscroll-behavior-x:contain]">
              {sortedBuffers.map((buffer, index) => (
                <SortableEditorTab
                  key={buffer.id}
                  id={buffer.id}
                  tabRef={(el) => { tabRefs.current[index] = el; }}
                >
                  <TabBarItem
                    buffer={buffer}
                    displayName={getBufferDisplayName(buffer)}
                    index={index}
                    isActive={buffer.id === activeBufferId}
                    isDraggedTab={buffer.id === draggedBufferId}
                    onClick={() => handleTabSelect(buffer)}
                    onDoubleClick={(e) => handleDoubleClick(e, index)}
                    onContextMenu={(e) => handleContextMenu(e, buffer)}
                    onKeyDown={(e) => handleKeyDown(e, index)}
                    handleTabClose={closeTab}
                    handleTabPin={handleTabPin}
                  />
                </SortableEditorTab>
              ))}
            </div>
          </SortableContext>

          {paneId && (
            <TabNewButton
              isBottomPane={isBottomPane}
              disablePaneActions={disablePaneActions}
              isInSplit={isInSplit}
              onNewConversation={() => { setActivePane(paneId); openContent({ type: 'crowbarChat', wsId: nanoid(), name: 'New Conversation' }); }}
              onNewTerminal={() => { setActivePane(paneId); openContent({ type: 'terminal' }); }}
              onOpenUrl={() => { setActivePane(paneId); openContent({ type: 'webViewer', url: 'https://' }); }}
              onClosePane={() => closePane(paneId)}
            />
          )}
        </div>

        <DragOverlay dropAnimation={null}>
          {draggedBuffer ? (
            <div className="tab-drag-preview ui-font flex items-center gap-1.5 rounded-lg border border-border/70 bg-background/95 px-2 py-1 ui-text-xs opacity-95 shadow-sm">
              <span className="max-w-[200px] truncate text-foreground">
                {draggedBuffer.name}
                {draggedBuffer.type === "editor" && draggedBuffer.isDirty && (
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
          fileName={buffers.find((b) => b.id === pendingClose.bufferId)?.name || ""}
          onSave={handleSaveAndClose}
          onDiscard={handleDiscardAndClose}
          onCancel={handleCancelClose}
        />
      )}

      <div role="status" aria-live="polite" aria-atomic="true" className="sr-only">
        {srAnnouncement}
      </div>
    </>
  );
};

export default TabBar;
