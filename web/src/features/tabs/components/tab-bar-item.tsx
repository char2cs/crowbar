import {
  Pulse as Activity,
  Database,
  GitBranch,
  GitPullRequest,
  GlobeHemisphereWest as Globe,
  MagnifyingGlass as Search,
  ChatCircleText as MessageSquare,
  Package,
  PushPin as Pin,
  Sparkle as Sparkles,
  TerminalWindow as Terminal,
  WarningCircle,
  X,
} from "@phosphor-icons/react";
import { memo, useCallback, useEffect, useState } from "react";
import type { RefCallback } from "react";
import { FileExplorerIcon } from "@/features/file-explorer/components/file-explorer-icon";
import type { PaneContent } from "@/features/panes/types/pane-content";
import { Button } from "@/components/ui/button";
import { Tab } from "@/components/ui/tabs";
import { getBaseName } from "@/utils/path-helpers";
import { cn } from "@/utils/cn";
import type { MultiFileDiff } from "@/features/git/types/git-diff-types";
import type { GitDiff } from "@/features/git/types/git-types";

interface TabBarItemProps {
  buffer: PaneContent;
  displayName: string;
  index: number;
  isActive: boolean;
  isDraggedTab: boolean;
  showDropIndicatorBefore?: boolean;
  tabRef?: RefCallback<HTMLDivElement>;
  onClick?: () => void;
  onMouseDown?: (e: React.MouseEvent) => void;
  onDoubleClick: (e: React.MouseEvent) => void;
  onContextMenu: (e: React.MouseEvent) => void;
  onKeyDown: (e: React.KeyboardEvent) => void;
  handleTabClose: (id: string) => void;
  handleTabPin: (id: string) => void;
}

const TabBarItem = memo(function TabBarItem({
  buffer,
  displayName,
  isActive,
  isDraggedTab,
  showDropIndicatorBefore = false,
  tabRef,
  onClick,
  onMouseDown,
  onDoubleClick,
  onContextMenu,
  onKeyDown,
  handleTabClose,
  handleTabPin,
}: TabBarItemProps) {
  const [faviconError, setFaviconError] = useState(false);

  const getDiffIconName = () => {
    if (buffer.type !== "diff") return buffer.name;
    if (buffer.path === "diff://working-tree/all-files") return null;

    const diffData = buffer.diffData;
    if (diffData && !("files" in diffData)) {
      return getDiffFileName(diffData);
    }

    return displayName;
  };

  // Reset favicon error when favicon URL changes
  useEffect(() => {
    setFaviconError(false);
  }, [buffer.type === "webViewer" ? buffer.favicon : undefined]);

  const handleAuxClick = useCallback(
    (e: React.MouseEvent) => {
      // Only handle middle click here
      if (e.button !== 1) return;

      handleTabClose(buffer.id);
    },
    [handleTabClose, buffer.id],
  );

  return (
    /*
     * The outer div carries `group/tab` so that the close button's
     * `group-hover/tab:opacity-100` class still works on hover.
     *
     * IMPORTANT: the close/pin Button is rendered here as a SIBLING of the
     * Tab <button>, NOT nested inside it.  Nesting <button> inside <button>
     * is invalid HTML and causes browsers to fire events unpredictably —
     * in particular DndKit's PointerSensor captured the pointerdown from the
     * close button and called handleDragStart → handleTabSelect before the
     * close button's own onClick could run.
     */
    <div ref={tabRef} className="group/tab relative">
      {showDropIndicatorBefore ? (
        <div className="drop-indicator absolute top-1 bottom-1 left-0 z-20 w-0.5 bg-accent" />
      ) : null}
      <Tab
        role="tab"
        aria-selected={isActive}
        aria-label={`${buffer.name}${buffer.type === "editor" && buffer.isDirty ? " (unsaved)" : ""}${buffer.isPinned ? " (pinned)" : ""}${buffer.isPreview ? " (preview)" : ""}`}
        tabIndex={isActive ? 0 : -1}
        isActive={isActive}
        isDragged={isDraggedTab}
        className={cn("h-5 pl-2 pr-6", isActive && "bg-muted/80")}
        onClick={onClick}
        onMouseDown={onMouseDown}
        onDoubleClick={onDoubleClick}
        onContextMenu={onContextMenu}
        onKeyDown={onKeyDown}
        onAuxClick={handleAuxClick}
      >
        <div className="grid size-3 shrink-0 place-content-center">
          {buffer.path === "extensions://marketplace" ? (
            <Package className="text-muted-foreground" />
          ) : buffer.type === "diff" && isMultiFileDiff(buffer.diffData) ? (
            <GitBranch className="text-muted-foreground" />
          ) : buffer.type === "terminal" ? (
            <Terminal className="text-muted-foreground" />
          ) : buffer.type === "agent" ? (
            <Sparkles className="text-muted-foreground" />
          ) : buffer.type === "webViewer" ? (
            buffer.favicon && !faviconError ? (
              <img
                src={buffer.favicon}
                alt=""
                className="size-3 object-contain"
                onError={() => setFaviconError(true)}
              />
            ) : (
              <Globe className="text-muted-foreground" />
            )
          ) : buffer.type === "database" ? (
            <Database className="text-muted-foreground" />
          ) : buffer.type === "pullRequest" ? (
            buffer.authorAvatarUrl ? (
              <img
                src={buffer.authorAvatarUrl}
                alt=""
                className="size-3 rounded-full object-cover"
                loading="lazy"
              />
            ) : (
              <GitPullRequest className="text-muted-foreground" />
            )
          ) : buffer.type === "githubIssue" ? (
            buffer.authorAvatarUrl ? (
              <img
                src={buffer.authorAvatarUrl}
                alt=""
                className="size-3 rounded-full object-cover"
                loading="lazy"
              />
            ) : (
              <MessageSquare className="text-muted-foreground" />
            )
          ) : buffer.type === "githubAction" ? (
            <Activity className="text-muted-foreground" />
          ) : buffer.type === "globalSearch" ? (
            <Search className="text-muted-foreground" />
          ) : buffer.type === "diagnostics" ? (
            <WarningCircle className="text-muted-foreground" />
          ) : buffer.type === "references" ? (
            <Search className="text-muted-foreground" />
          ) : (
            <FileExplorerIcon
              fileName={getDiffIconName() ?? buffer.name}
              isDir={false}
              className="text-muted-foreground"
              size={12}
            />
          )}
        </div>
        <span
          className={cn(
            "ui-font ui-text-sm min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap",
            isActive ? "text-foreground" : "text-muted-foreground",
            buffer.isPreview && "italic",
          )}
          title={buffer.path}
        >
          {displayName}
        </span>
        {buffer.type === "editor" && buffer.isDirty && (
          <div
            className="size-2 shrink-0 rounded-full bg-accent"
            title="Unsaved changes"
            role="img"
            aria-label="Unsaved changes"
          />
        )}
      </Tab>

      {/* Close / pin button — rendered OUTSIDE the Tab <button> to avoid
          invalid nested-button HTML that breaks event propagation. */}
      <Button
        type="button"
        compact
        variant="ghost"
        data-no-dnd
        onClick={(e) => {
          e.stopPropagation();
          if (buffer.isPinned) {
            handleTabPin(buffer.id);
          } else {
            handleTabClose(buffer.id);
          }
        }}
        className={cn(
          "-translate-y-1/2 absolute top-1/2 right-1 h-4 min-w-4 cursor-pointer select-none rounded-sm px-0 text-muted-foreground transition-opacity",
          "hover:text-foreground",
          buffer.isPinned || isActive ? "opacity-100" : "opacity-0 group-hover/tab:opacity-100",
        )}
        tooltip={buffer.isPinned ? "Unpin tab" : "Close"}
        shortcut={buffer.isPinned ? undefined : "mod+w"}
        tabIndex={-1}
        draggable={false}
      >
        {buffer.isPinned ? (
          <Pin className="pointer-events-none select-none fill-current text-accent" />
        ) : (
          <X className="pointer-events-none select-none" />
        )}
      </Button>
    </div>
  );
});

function isMultiFileDiff(diffData: GitDiff | MultiFileDiff | undefined): diffData is MultiFileDiff {
  return Boolean(diffData && "files" in diffData);
}

function getDiffFileName(diff: GitDiff): string {
  const filePath = diff.new_path || diff.old_path || diff.file_path || "";
  return getBaseName(filePath, filePath || "diff");
}

export default TabBarItem;
