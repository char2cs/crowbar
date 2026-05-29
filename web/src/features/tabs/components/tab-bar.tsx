import {
  DndContext,
  DragOverlay,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragMoveEvent,
  type DragStartEvent,
} from "@dnd-kit/core";

/**
 * Custom PointerSensor that skips drag activation when the pointer event
 * originates on a `[data-no-dnd]` element (e.g. the tab close button).
 * Without this, DndKit captures the pointer on the close button's pointerdown,
 * which fires handleDragStart → handleTabSelect before the close button's
 * onClick can run, causing the tab to be activated instead of closed.
 */
class NoDndPointerSensor extends PointerSensor {
  static activators = [
    {
      eventName: 'onPointerDown' as const,
      handler: ({ nativeEvent: event }: { nativeEvent: PointerEvent }): boolean => {
        if (!event.isPrimary || event.button !== 0) return false;
        const target = event.target as HTMLElement | null;
        if (target?.closest('[data-no-dnd]')) return false;
        return true;
      },
    },
  ];
}
import { SortableContext, horizontalListSortingStrategy, useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import {
  ArrowLeft,
  ArrowRight,
  GlobeHemisphereWest as Globe,
  Plus,
  SidebarSimple as PanelLeftClose,
  TerminalWindow as Terminal,
} from "@phosphor-icons/react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
// Use browser clipboard API instead of Tauri clipboard plugin
const writeText = (text: string) => navigator.clipboard.writeText(text);
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useJumpListStore } from "@/features/editor/stores/jump-list-store";
import { useEditorStateStore } from "@/features/editor/stores/state-store";
import { navigateToJumpEntry } from "@/features/editor/utils/jump-navigation";
import { useFileSystemStore } from "@/features/file-system/controllers/store";
import { formatDiffBufferLabel } from "@/features/git/utils/diff-buffer-label";
import { BOTTOM_PANE_ID } from "@/features/panes/constants/pane";
import {
  usePaneRoot,
  useBottomRoot,
  usePaneActions,
} from "@/features/workspace/stores/hooks/use-pane-store";
import { useBuffers, useBufferActions } from "@/features/workspace/stores/hooks/use-buffer-store";
import { useWorkspaceStore, useWorkspaceStoreContext } from "@/features/workspace/stores/workspace-context";
import { splitEditorGroup } from "@/features/panes/utils/pane-command-actions";
import { getPaneSplitDropOptions } from "@/features/panes/utils/pane-drop-zones";
import { findPaneGroup } from "@/features/panes/utils/pane-tree";
import { useSettingsStore } from "@/features/settings/store";
import type { PaneContent } from "@/features/panes/types/pane-content";
import { useEditorAppStore } from "@/features/editor/stores/editor-app-store";
import { useSidebarStore } from "@/features/layout/stores/sidebar-store";
import { useTerminalStore } from "@/features/terminal/stores/terminal-store";
import { useWebViewerNavigationStore } from "@/features/web-viewer/stores/web-viewer-navigation-store";
import UnsavedChangesDialog from "@/features/window/components/unsaved-changes-dialog";
import { useUIState } from "@/features/window/stores/ui-state-store";
import { Button } from "@/components/ui/button";
import { useSidebar } from "@/components/ui/sidebar";
import { getRelativePath } from "@/utils/path-helpers";
import { cn } from "@/utils/cn";
import { IS_MAC } from "@/utils/platform";
import { calculateDisplayNames } from "../utils/path-shortener";
import {
  clearInternalTabDragData,
  resolveDropTarget,
  setInternalTabDragHoverTarget,
  setInternalTabDragData,
} from "../utils/internal-tab-drag";
import TabBarItem from "./tab-bar-item";
import TabContextMenu from "./tab-context-menu";

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
  // Get everything from stores
  const allBuffers = useBuffers();
  const globalActiveBufferId = useWorkspaceStoreContext(s => s.paneActions.getActivePane()?.activeBufferId ?? null);
  const pendingClose = null as ({ bufferId: string } | null);  // workspace buffer slice tracks pending close via local state
  const paneRoot = usePaneRoot();
  const bottomRoot = useBottomRoot();
  const { closePane, setActivePane, activatePaneBuffer, removeBufferFromPane, splitPane, moveBufferToPane, reorderPaneBuffers } = usePaneActions();
  const { closeBuffer, openContent, promotePreview: promotePreviewBuffer, setPinned, confirmPendingClose, setPendingClose } = useBufferActions();
  const workspaceStore = useWorkspaceStore();

  // Filter buffers by paneId if provided
  const pane = paneId
    ? paneId === BOTTOM_PANE_ID
      ? findPaneGroup(bottomRoot, BOTTOM_PANE_ID)
      : findPaneGroup(paneRoot, paneId)
    : null;
  const buffers = (
    pane ? allBuffers.filter((b) => pane.bufferIds.includes(b.id)) : allBuffers
  ).filter((buffer) => buffer.type !== "newTab");
  const activeBufferCandidate = pane ? pane.activeBufferId : globalActiveBufferId;
  const activeBufferId =
    activeBufferCandidate && buffers.some((buffer) => buffer.id === activeBufferCandidate)
      ? activeBufferCandidate
      : null;
  // Inline replacements for legacy buffer-store tab actions
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
  function reorderBuffers(startIndex: number, endIndex: number) {
    if (paneId) reorderPaneBuffers(paneId, startIndex, endIndex);
  }
  const confirmCloseWithoutSaving = confirmPendingClose;
  const cancelPendingClose = () => setPendingClose(null);
  const convertPreviewToDefinite = promotePreviewBuffer;

  // Workspace-aware tab interactions
  function handleTabClick(bufferId: string) {
    if (paneId) {
      activatePaneBuffer(paneId, bufferId);
      setActivePane(paneId);
    }
    externalTabClick?.(bufferId);
  }
  function handleTabClose(bufferId: string, _event?: React.MouseEvent) {
    if (paneId) {
      removeBufferFromPane(paneId, bufferId);
    }
    closeBuffer(bufferId);
  }
  const { handleSave } = useEditorAppStore.use.actions();
  const horizontalTabScroll = useSettingsStore((state) => state.settings.horizontalTabScroll);
  const maxOpenTabs = useSettingsStore((state) => state.settings.maxOpenTabs);
  const updateActivePath = useSidebarStore((s) => s.updateActivePath)
  const sidebarPosition = useSettingsStore((s) => s.settings.sidebarPosition)
  const { open: sidebarOpen, toggleSidebar } = useSidebar()
  const rootFolderPath = useFileSystemStore.use.rootFolderPath?.() || undefined;
  const jumpListActions = useJumpListStore.use.actions();
  const activeBuffer = useMemo(
    () => buffers.find((buffer) => buffer.id === activeBufferId) ?? null,
    [activeBufferId, buffers],
  );
  const activeWebViewerNavigation = useWebViewerNavigationStore((state) =>
    activeBuffer?.type === "webViewer" ? state.navigationByBufferId[activeBuffer.id] : undefined,
  );
  const usesWebViewerNavigation = activeBuffer?.type === "webViewer";
  const canGoBack = usesWebViewerNavigation
    ? Boolean(activeWebViewerNavigation?.canGoBack)
    : jumpListActions.canGoBack();
  const canGoForward = usesWebViewerNavigation
    ? Boolean(activeWebViewerNavigation?.canGoForward)
    : jumpListActions.canGoForward();
  const isInSplit = paneRoot.type === "split";
  const isBottomPane = paneId === BOTTOM_PANE_ID;

  const [draggedBufferId, setDraggedBufferId] = useState<string | null>(null);

  const [contextMenu, setContextMenu] = useState<{
    isOpen: boolean;
    position: { x: number; y: number };
    buffer: PaneContent | null;
  }>({ isOpen: false, position: { x: 0, y: 0 }, buffer: null });

  const [srAnnouncement, setSrAnnouncement] = useState<string>("");

  const tabBarRef = useRef<HTMLDivElement>(null);
  const tabRefs = useRef<(HTMLDivElement | null)[]>([]);
  const dragPointRef = useRef<{ x: number; y: number } | null>(null);
  const pointerPointRef = useRef<{ x: number; y: number } | null>(null);

  const [isAtLeftEdge, setIsAtLeftEdge] = useState(false);
  useLayoutEffect(() => {
    const el = tabBarRef.current;
    if (!el) return;
    function check() {
      setIsAtLeftEdge((el?.getBoundingClientRect().left ?? 1) < 10);
    }
    check();
    const ro = new ResizeObserver(check);
    ro.observe(el);
    window.addEventListener('resize', check);
    return () => { ro.disconnect(); window.removeEventListener('resize', check); };
  }, [sidebarPosition]);
  const handleRevealInFolder = useFileSystemStore.use.handleRevealInFolder?.();
  const { clearPositionCache } = useEditorStateStore.getState().actions;
  const terminalSessions = useTerminalStore((state) => state.sessions);
  const sensors = useSensors(
    useSensor(NoDndPointerSensor, {
      activationConstraint: {
        distance: 6,
      },
    }),
  );

  const getDirectoryLabel = useCallback((directory?: string) => {
    if (!directory) return "";
    const normalized = directory.replace(/[\\/]+$/, "");
    return normalized.split(/[\\/]/).pop() || directory;
  }, []);

  const getCommandLabel = useCallback((command?: string) => {
    if (!command) return "";
    const firstSegment = command.trim().split(/\s+/)[0];
    return firstSegment?.split(/[\\/]/).pop() || "";
  }, []);

  const isUsefulTerminalTitle = useCallback((title?: string) => {
    if (!title) return false;
    const trimmed = title.trim();
    if (!trimmed || trimmed === "Default Terminal") return false;
    if (trimmed.length > 28) return false;
    if (trimmed.includes("@")) return false;
    if (trimmed.includes("/") || trimmed.includes("\\")) return false;

    for (const char of trimmed) {
      const code = char.charCodeAt(0);
      if ((code >= 0 && code <= 31) || code === 127 || code === 155) {
        return false;
      }
    }

    return true;
  }, []);

  const handleJumpBack = useCallback(async () => {
    if (usesWebViewerNavigation) {
      activeWebViewerNavigation?.goBack?.();
      return;
    }

    const wsState = workspaceStore.getState();
    const editorState = useEditorStateStore.getState();
    const activePaneGroup = findPaneGroup(wsState.paneRoot, wsState.activePaneId);
    const currentActiveBufferId = activePaneGroup?.activeBufferId ?? null;
    const currentActiveBuffer = currentActiveBufferId
      ? wsState.buffers.find((b) => b.id === currentActiveBufferId)
      : undefined;

    const currentPosition =
      currentActiveBufferId && currentActiveBuffer?.path
        ? {
            bufferId: currentActiveBufferId,
            filePath: currentActiveBuffer.path,
            line: editorState.cursorPosition.line,
            column: editorState.cursorPosition.column,
            offset: editorState.cursorPosition.offset,
            scrollTop: editorState.scrollTop,
            scrollLeft: editorState.scrollLeft,
          }
        : undefined;

    const entry = jumpListActions.goBack(currentPosition);
    if (entry) {
      await navigateToJumpEntry(entry);
    }
  }, [activeWebViewerNavigation, jumpListActions, usesWebViewerNavigation, workspaceStore]);

  const handleJumpForward = useCallback(async () => {
    if (usesWebViewerNavigation) {
      activeWebViewerNavigation?.goForward?.();
      return;
    }

    const entry = jumpListActions.goForward();
    if (entry) {
      await navigateToJumpEntry(entry);
    }
  }, [activeWebViewerNavigation, jumpListActions, usesWebViewerNavigation]);

  const canScrollTabsHorizontally = useCallback(() => {
    const container = tabBarRef.current;
    if (!container) return false;

    return container.scrollWidth > container.clientWidth + 1;
  }, []);

  // Optional wheel-to-horizontal scrolling for overflowing tab strips.
  const handleWheel = useCallback(
    (e: React.WheelEvent<HTMLDivElement>) => {
      const container = tabBarRef.current;
      if (!container) return;
      if (!horizontalTabScroll) return;
      if (draggedBufferId) return;
      if (e.ctrlKey || e.metaKey) return;
      if (!canScrollTabsHorizontally()) return;

      const hasHorizontalIntent = Math.abs(e.deltaX) > 0;
      const hasShiftedVerticalIntent = e.shiftKey && Math.abs(e.deltaY) > 0;
      const hasVerticalFallback = Math.abs(e.deltaX) === 0 && Math.abs(e.deltaY) > 0;

      if (!hasHorizontalIntent && !hasShiftedVerticalIntent && !hasVerticalFallback) {
        return;
      }

      const delta = hasHorizontalIntent ? e.deltaX : e.deltaY;
      if (delta === 0) return;

      const maxScrollLeft = container.scrollWidth - container.clientWidth;
      if (maxScrollLeft <= 0) return;

      const nextScrollLeft = Math.max(0, Math.min(container.scrollLeft + delta, maxScrollLeft));
      if (nextScrollLeft === container.scrollLeft) return;

      e.preventDefault();
      container.scrollLeft = nextScrollLeft;
    },
    [canScrollTabsHorizontally, draggedBufferId, horizontalTabScroll],
  );

  const sortedBuffers = useMemo(() => {
    return [...buffers].sort((a, b) => {
      if (a.isPinned && !b.isPinned) return -1;
      if (!a.isPinned && b.isPinned) return 1;
      return 0;
    });
  }, [buffers]);
  const sortedBufferIds = useMemo(() => sortedBuffers.map((buffer) => buffer.id), [sortedBuffers]);
  const draggedBuffer = useMemo(
    () => sortedBuffers.find((buffer) => buffer.id === draggedBufferId) ?? null,
    [draggedBufferId, sortedBuffers],
  );

  // Calculate display names for tabs with minimal distinguishing paths
  const displayNames = useMemo(() => {
    return calculateDisplayNames(buffers, rootFolderPath);
  }, [buffers, rootFolderPath]);

  const getBufferDisplayName = useCallback(
    (buffer: PaneContent) => {
      if (buffer.type === "terminal") {
        const session = terminalSessions.get(buffer.sessionId);
        if (session?.customName) {
          return session.name?.trim() || buffer.name;
        }

        const title = session?.title?.trim();
        if (isUsefulTerminalTitle(title)) return title!;

        const commandLabel = getCommandLabel(buffer.initialCommand);
        if (commandLabel) return commandLabel;

        const dirLabel = getDirectoryLabel(session?.currentDirectory || buffer.workingDirectory);
        if (dirLabel) return dirLabel;
      }

      if (buffer.type === "diff") {
        return formatDiffBufferLabel(displayNames.get(buffer.id) || buffer.name, buffer.path);
      }

      return displayNames.get(buffer.id) ?? buffer.name;
    },
    [displayNames, getCommandLabel, getDirectoryLabel, isUsefulTerminalTitle, terminalSessions],
  );

  useEffect(() => {
    if (maxOpenTabs > 0 && buffers.length > maxOpenTabs && handleTabClose) {
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

        // Check if tab is out of view
        if (tabRect.left < containerRect.left || tabRect.right > containerRect.right) {
          activeTab.scrollIntoView({
            behavior: "smooth",
            block: "nearest",
            inline: "center",
          });
        }
      }
    }
  }, [activeBufferId, sortedBuffers]);

  const handleDoubleClick = useCallback(
    (e: React.MouseEvent, index: number) => {
      e.preventDefault();
      e.stopPropagation();
      const buffer = sortedBuffers[index];
      if (!buffer) return;
      // Convert preview tab to definite on double-click — update both stores
      if (buffer.isPreview) {
        convertPreviewToDefinite(buffer.id);    // legacy store
        promotePreviewBuffer(buffer.id);        // workspace store (also clears pane pointer)
      }
    },
    [sortedBuffers, convertPreviewToDefinite, promotePreviewBuffer],
  );

  const handleContextMenu = useCallback((e: React.MouseEvent, buffer: PaneContent) => {
    e.preventDefault();

    // Get the tab element that was right-clicked
    const target = e.currentTarget as HTMLElement;
    const rect = target.getBoundingClientRect();

    // Position the menu relative to the tab element
    // This approach is zoom-independent because getBoundingClientRect()
    // already accounts for zoom scaling
    const x = rect.left + rect.width * 0.5; // Center horizontally on the tab
    const y = rect.bottom + 4; // Position just below the tab with small offset

    setContextMenu({
      isOpen: true,
      position: { x, y },
      buffer,
    });
  }, []);

  const handleCopyPath = useCallback(
    async (path: string) => {
      await writeText(path);
    },
    [writeText],
  );

  const handleCopyRelativePath = useCallback(
    async (path: string) => {
      if (!rootFolderPath) {
        // If no project is open, copy the full path
        await writeText(path);
        return;
      }

      await writeText(getRelativePath(path, rootFolderPath));
    },
    [rootFolderPath, writeText],
  );

  const closeContextMenu = () => {
    setContextMenu({ isOpen: false, position: { x: 0, y: 0 }, buffer: null });
  };

  const handleSaveAndClose = useCallback(async () => {
    if (!pendingClose) return;

    const buffer = buffers.find((b) => b.id === pendingClose.bufferId);
    if (!buffer) return;

    // Save the file
    await handleSave();

    // Then proceed with closing
    confirmCloseWithoutSaving();
  }, [pendingClose, buffers, handleSave, confirmCloseWithoutSaving]);

  const handleDiscardAndClose = useCallback(() => {
    confirmCloseWithoutSaving();
  }, [confirmCloseWithoutSaving]);

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

  const getClientPoint = (event: Event) => {
    const candidate = event as Partial<MouseEvent>;
    if (typeof candidate.clientX === "number" && typeof candidate.clientY === "number") {
      return { x: candidate.clientX, y: candidate.clientY };
    }
    return null;
  };

  const getDragPoint = (event: DragMoveEvent | DragEndEvent) => {
    if (pointerPointRef.current) return pointerPointRef.current;

    const rect = event.active.rect.current.translated ?? event.active.rect.current.initial;
    if (!rect) return dragPointRef.current;
    return {
      x: rect.left + rect.width / 2,
      y: rect.top + rect.height / 2,
    };
  };

const handleDragStart = useCallback(
    (event: DragStartEvent) => {
      const buffer = sortedBuffers.find((item) => item.id === String(event.active.id));
      if (!buffer) return;

      setDraggedBufferId(buffer.id);
      pointerPointRef.current = getClientPoint(event.activatorEvent);
      setInternalTabDragData({
        source: "pane",
        bufferId: buffer.id,
        paneId,
      });
      handleTabSelect(buffer);
    },
    [handleTabSelect, paneId, sortedBuffers],
  );

  const handleDragMove = useCallback((event: DragMoveEvent) => {
    const point = getDragPoint(event);
    if (!point) return;

    dragPointRef.current = point;

    // Update cross-pane hover state whenever the pointer is over a different pane
    // or a split zone of any pane. The old isPointOutsideTabBar gate broke horizontal
    // splits: both tab bars sit at the same Y, so verticalSlop=64 never triggered.
    const dropTarget = resolveDropTarget(point);
    if (dropTarget.paneId !== null && (dropTarget.paneId !== paneId || dropTarget.zone !== "center")) {
      setInternalTabDragHoverTarget(dropTarget);
    } else {
      // Hovering over the source tab bar (reorder mode) — clear any stale indicator
      setInternalTabDragHoverTarget({ paneId: null, zone: null });
    }
  }, [paneId]);

  const resetDrag = useCallback(() => {
    setDraggedBufferId(null);
    dragPointRef.current = null;
    pointerPointRef.current = null;
    clearInternalTabDragData();
  }, []);

  useEffect(() => {
    if (!draggedBufferId) return;

    const updatePointerPoint = (event: PointerEvent) => {
      pointerPointRef.current = { x: event.clientX, y: event.clientY };
    };

    window.addEventListener("pointermove", updatePointerPoint, true);
    return () => window.removeEventListener("pointermove", updatePointerPoint, true);
  }, [draggedBufferId]);

  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      const activeId = String(event.active.id);
      const dragged = sortedBuffers.find((buffer) => buffer.id === activeId);
      const point = getDragPoint(event);
      const target = point ? resolveDropTarget(point) : { paneId: null, zone: null };

      if (
        dragged &&
        paneId &&
        target.paneId &&
        (target.paneId !== paneId || (target.zone && target.zone !== "center"))
      ) {
        const splitOptions = target.zone ? getPaneSplitDropOptions(target.zone) : null;
        let destinationPaneId: string | null;
        if (splitOptions && target.paneId) {
          destinationPaneId = splitPane(target.paneId, splitOptions.direction, undefined, splitOptions.placement) ?? null;
        } else {
          destinationPaneId = target.paneId ?? null;
        }
        if (!destinationPaneId) {
          resetDrag();
          return;
        }
        if (paneId) moveBufferToPane(dragged.id, paneId, destinationPaneId);
        activatePaneBuffer(destinationPaneId, dragged.id);
        if (destinationPaneId === BOTTOM_PANE_ID) {
          useUIState.getState().setBottomPaneActiveTab("buffers");
          useUIState.getState().setIsBottomPaneVisible(true);
        }
      } else if (event.over && reorderBuffers) {
        const oldIndex = sortedBuffers.findIndex((buffer) => buffer.id === activeId);
        const newIndex = sortedBuffers.findIndex((buffer) => buffer.id === String(event.over?.id));
        if (oldIndex !== -1 && newIndex !== -1 && oldIndex !== newIndex) {
          reorderBuffers(oldIndex, newIndex);
          if (dragged) {
            handleTabClick(dragged.id);
          }
        }
      }

      resetDrag();
    },
    [activatePaneBuffer, handleTabClick, moveBufferToPane, paneId, reorderBuffers, resetDrag, sortedBuffers, splitPane],
  );

  useEffect(() => {
    tabRefs.current = tabRefs.current.slice(0, sortedBuffers.length);
  }, [sortedBuffers.length]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent, index: number) => {
      const buffer = sortedBuffers[index];
      if (!buffer) return;

      switch (e.key) {
        case "ArrowLeft":
          e.preventDefault();
          if (index > 0) {
            const prevBuffer = sortedBuffers[index - 1];
            handleTabClick(prevBuffer.id);
            updateActivePath(prevBuffer.path);
            setSrAnnouncement(
              `Switched to ${prevBuffer.name}${prevBuffer.type === "editor" && prevBuffer.isDirty ? ", unsaved changes" : ""}`,
            );
            tabRefs.current[index - 1]?.focus();
          }
          break;

        case "ArrowRight":
          e.preventDefault();
          if (index < sortedBuffers.length - 1) {
            const nextBuffer = sortedBuffers[index + 1];
            handleTabClick(nextBuffer.id);
            updateActivePath(nextBuffer.path);
            setSrAnnouncement(
              `Switched to ${nextBuffer.name}${nextBuffer.type === "editor" && nextBuffer.isDirty ? ", unsaved changes" : ""}`,
            );
            tabRefs.current[index + 1]?.focus();
          }
          break;

        case "Home":
          e.preventDefault();
          if (sortedBuffers.length > 0) {
            const firstBuffer = sortedBuffers[0];
            handleTabClick(firstBuffer.id);
            updateActivePath(firstBuffer.path);
            setSrAnnouncement(
              `Switched to ${firstBuffer.name}${firstBuffer.type === "editor" && firstBuffer.isDirty ? ", unsaved changes" : ""}`,
            );
            tabRefs.current[0]?.focus();
          }
          break;

        case "End":
          e.preventDefault();
          if (sortedBuffers.length > 0) {
            const lastIndex = sortedBuffers.length - 1;
            const lastBuffer = sortedBuffers[lastIndex];
            handleTabClick(lastBuffer.id);
            updateActivePath(lastBuffer.path);
            setSrAnnouncement(
              `Switched to ${lastBuffer.name}${lastBuffer.type === "editor" && lastBuffer.isDirty ? ", unsaved changes" : ""}`,
            );
            tabRefs.current[lastIndex]?.focus();
          }
          break;

        case "Delete":
        case "Backspace":
          if (!buffer.isPinned) {
            e.preventDefault();
            setSrAnnouncement(`Closed ${buffer.name}`);
            closeTab(buffer.id);
          } else {
            setSrAnnouncement(`Cannot close pinned tab ${buffer.name}`);
          }
          break;

        case "Enter":
        case " ":
          e.preventDefault();
          handleTabClick(buffer.id);
          updateActivePath(buffer.path);
          setSrAnnouncement(
            `Activated ${buffer.name}${buffer.type === "editor" && buffer.isDirty ? ", unsaved changes" : ""}`,
          );
          break;
      }
    },
    [sortedBuffers, handleTabClick, updateActivePath, closeTab],
  );

  const MemoizedTabContextMenu = useMemo(() => TabContextMenu, []);

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
            IS_MAC && !isBottomPane && sidebarPosition === 'right' && isAtLeftEdge && 'pl-[80px]',
          )}
          role="tablist"
          aria-label="Open files"
          data-tauri-drag-region
          onWheel={handleWheel}
        >
          {!isBottomPane && (
            <Button
              type="button"
              onClick={toggleSidebar}
              variant="ghost"
              compact
              className={cn(
                "h-6 w-6 shrink-0 rounded-full p-0 text-muted-foreground",
                sidebarPosition === 'right' && "scale-x-[-1]",
              )}
              tooltip={sidebarOpen ? "Hide Sidebar" : "Show Sidebar"}
              tooltipSide="bottom"
              aria-label={sidebarOpen ? "Hide sidebar" : "Show sidebar"}
            >
              <PanelLeftClose size={14} />
            </Button>
          )}

          <div className="flex shrink-0 items-center gap-0.5">
            <Button
              type="button"
              onClick={handleJumpBack}
              disabled={!canGoBack}
              variant="ghost"
              className="h-6 w-6 shrink-0 rounded-full p-0 text-muted-foreground"
              tooltip="Go Back"
              tooltipSide="bottom"
              commandId="navigation.goBack"
              aria-label="Go back to previous location"
              compact
            >
              <ArrowLeft />
            </Button>
            <Button
              type="button"
              onClick={handleJumpForward}
              disabled={!canGoForward}
              variant="ghost"
              className="h-6 w-6 shrink-0 rounded-full p-0 text-muted-foreground"
              tooltip="Go Forward"
              tooltipSide="bottom"
              commandId="navigation.goForward"
              aria-label="Go forward to next location"
              compact
            >
              <ArrowRight />
            </Button>
          </div>

          <SortableContext items={sortedBufferIds} strategy={horizontalListSortingStrategy}>
            <div className="scrollbar-hidden flex min-w-0 flex-1 items-center gap-1.5 overflow-x-auto overflow-y-hidden [overscroll-behavior-x:contain]">
              {sortedBuffers.map((buffer, index) => (
                <SortableEditorTab
                  key={buffer.id}
                  id={buffer.id}
                  tabRef={(el) => {
                    tabRefs.current[index] = el;
                  }}
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

          <div className="flex shrink-0 items-center gap-1 pl-0.5">
            {paneId && !isBottomPane && (
              <DropdownMenu>
                <DropdownMenuTrigger className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full p-0 text-muted-foreground hover:bg-muted hover:text-foreground focus:outline-none" aria-label="New tab">
                  <Plus weight="bold" size={12} />
                </DropdownMenuTrigger>
                <DropdownMenuContent side="bottom" align="start" className="min-w-[140px]">
                  <DropdownMenuItem onClick={() => { if (paneId) setActivePane(paneId); openContent({ type: 'terminal' }); }}>
                    <Terminal className="text-muted-foreground" />
                    New Terminal
                  </DropdownMenuItem>
                  <DropdownMenuItem onClick={() => { if (paneId) setActivePane(paneId); openContent({ type: 'webViewer', url: 'https://' }); }}>
                    <Globe className="text-muted-foreground" />
                    Open URL
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            )}
            {paneId && !disablePaneActions && !isBottomPane && isInSplit && (
              <Button
                type="button"
                onClick={() => closePane(paneId)}
                variant="ghost"
                compact
                className="h-6 w-6 shrink-0 rounded-full p-0 text-muted-foreground"
                tooltip="Close Split"
                tooltipSide="bottom"
                aria-label="Close split pane"
              >
                <PanelLeftClose />
              </Button>
            )}
          </div>
        </div>

        <DragOverlay dropAnimation={null}>
          {draggedBuffer ? (
            <div className="tab-drag-preview ui-font flex items-center gap-1.5 rounded-lg border border-border/70 bg-background/95 px-2 py-1 ui-text-xs opacity-95 shadow-sm">
              <span className="max-w-[200px] truncate text-foreground">{draggedBuffer.name}</span>
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
        onCloseTab={(bufferId) => {
          const buffer = buffers.find((b) => b.id === bufferId);
          if (buffer) {
            closeTab(bufferId);
          }
        }}
        onCloseOthers={handleCloseOtherTabs}
        onCloseAll={handleCloseAllTabs}
        onCloseToRight={handleCloseTabsToRight}
        onCopyPath={handleCopyPath}
        onCopyRelativePath={handleCopyRelativePath}
        onReload={(bufferId: string) => {
          const buffer = buffers.find((b) => b.id === bufferId);
          if (buffer && buffer.path !== "extensions://marketplace") {
            if (paneId) removeBufferFromPane(paneId, bufferId);
            closeBuffer(bufferId);
            setTimeout(async () => {
              try {
                const content =
                  buffer.type === "editor" || buffer.type === "diff" ? buffer.content : "";
                const spec = buffer.type === "image"
                  ? ({ type: "image", path: buffer.path, name: buffer.name } as const)
                  : buffer.type === "diff"
                  ? ({ type: "diff", path: buffer.path, name: buffer.name, content } as const)
                  : ({ type: "editor", path: buffer.path, name: buffer.name, content } as const);
                openContent(spec);
              } catch (error) {
                console.error("Failed to reload buffer:", error);
              }
            }, 100);
          }
        }}
        onRevealInFinder={handleRevealInFolder ?? undefined}
        onSplitRight={
          paneId
            ? (targetPaneId, bufferId) => {
                splitEditorGroup(targetPaneId, "horizontal", bufferId);
              }
            : undefined
        }
        onSplitDown={
          paneId
            ? (targetPaneId, bufferId) => {
                splitEditorGroup(targetPaneId, "vertical", bufferId);
              }
            : undefined
        }
      />

      {pendingClose && (
        <UnsavedChangesDialog
          fileName={buffers.find((b) => b.id === pendingClose.bufferId)?.name || ""}
          onSave={handleSaveAndClose}
          onDiscard={handleDiscardAndClose}
          onCancel={handleCancelClose}
        />
      )}

      {/* Screen reader live region for announcements */}
      <div role="status" aria-live="polite" aria-atomic="true" className="sr-only">
        {srAnnouncement}
      </div>
    </>
  );
};

interface SortableEditorTabProps {
  id: string;
  children: ReactNode;
  tabRef: (element: HTMLDivElement | null) => void;
}

function SortableEditorTab({ id, children, tabRef }: SortableEditorTabProps) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id,
  });

  return (
    <div
      ref={(element) => {
        setNodeRef(element);
        tabRef(element);
      }}
      style={{
        transform: CSS.Transform.toString(transform),
        transition,
      }}
      className={isDragging ? "relative z-10 opacity-40" : "relative"}
      {...attributes}
      {...listeners}
    >
      {children}
    </div>
  );
}

export default TabBar;
