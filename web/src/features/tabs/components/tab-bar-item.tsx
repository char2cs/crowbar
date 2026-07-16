import {
  GitBranch,
  GitPullRequest,
  Package,
  PushPin as Pin,
  TerminalWindow as Terminal,
  X,
} from '@phosphor-icons/react'
import { memo, useCallback } from 'react'
import type { RefCallback } from 'react'
import { FileExplorerIcon } from '@/features/file-explorer/components/file-explorer-icon'
import { AgentChatTabIcon } from '@/features/agent/components/agent-chat-tab-icon'
import type { PaneContent } from '@/features/panes/types/pane-content'
import { sameRenderedBuffer } from './tab-bar-item-utils'
import { Button } from '@/components/ui/button'
import { Tab } from '@/components/ui/tabs'
import { getBaseName } from '@/utils/path-helpers'
import { cn } from '@/utils/cn'
import type { MultiFileDiff } from '@/features/git/types/git-diff-types'
import type { GitDiff } from '@/features/git/types/git-types'

interface TabBarItemProps {
  buffer: PaneContent
  displayName: string
  index: number
  isActive: boolean
  isDraggedTab: boolean
  showDropIndicatorBefore?: boolean
  tabRef?: RefCallback<HTMLDivElement>
  // Delegated handlers: the parent passes ONE stable reference of each (shared
  // by every item); the item re-supplies its own identity (buffer / index) when
  // it fires. This keeps the handler props referentially stable across TabBar
  // renders so the memo comparator below can bail out on unchanged tabs.
  onSelect?: (buffer: PaneContent) => void
  onMouseDown?: (e: React.MouseEvent) => void
  onDoubleClick: (e: React.MouseEvent, buffer: PaneContent) => void
  onContextMenu: (e: React.MouseEvent, buffer: PaneContent) => void
  onKeyDown: (e: React.KeyboardEvent, index: number) => void
  handleTabClose: (id: string) => void
  handleTabPin: (id: string) => void
}

const TabBarItem = memo(function TabBarItem({
  buffer,
  displayName,
  index,
  isActive,
  isDraggedTab,
  showDropIndicatorBefore = false,
  tabRef,
  onSelect,
  onMouseDown,
  onDoubleClick,
  onContextMenu,
  onKeyDown,
  handleTabClose,
  handleTabPin,
}: TabBarItemProps) {
  // Re-bind the delegated handlers to THIS item's buffer/index. These closures
  // live inside the memoized item, so they are only rebuilt when the item
  // actually re-renders — they never reach the parent and so never defeat memo.
  const handleClick = useCallback(() => onSelect?.(buffer), [onSelect, buffer])
  const handleDoubleClick = useCallback(
    (e: React.MouseEvent) => onDoubleClick(e, buffer),
    [onDoubleClick, buffer],
  )
  const handleContextMenu = useCallback(
    (e: React.MouseEvent) => onContextMenu(e, buffer),
    [onContextMenu, buffer],
  )
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => onKeyDown(e, index),
    [onKeyDown, index],
  )

  const getDiffIconName = () => {
    if (buffer.type !== 'diff') return buffer.name
    if (buffer.path === 'diff://working-tree/all-files') return null

    const diffData = buffer.diffData
    if (diffData && !('files' in diffData)) {
      return getDiffFileName(diffData)
    }

    return displayName
  }

  const handleAuxClick = useCallback(
    (e: React.MouseEvent) => {
      // Only handle middle click here
      if (e.button !== 1) return

      handleTabClose(buffer.id)
    },
    [handleTabClose, buffer.id],
  )

  // An editor tab with unsaved edits. When it's also the active tab we render
  // the whole pill in the primary-button style (the fill is the signal, so the
  // dot is dropped); inactive unsaved tabs keep the pill and show a bright dot.
  const isDirtyEditor = buffer.type === 'editor' && buffer.isDirty

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
        <div className="drop-indicator absolute top-1 bottom-1 left-0 z-20 w-0.5 bg-secondary" />
      ) : null}
      <Tab
        role="tab"
        aria-selected={isActive}
        aria-label={`${buffer.name}${buffer.type === 'editor' && buffer.isDirty ? ' (unsaved)' : ''}${buffer.isPinned ? ' (pinned)' : ''}${buffer.isPreview ? ' (preview)' : ''}`}
        tabIndex={isActive ? 0 : -1}
        isActive={isActive}
        isDragged={isDraggedTab}
        className={cn(
          'h-8',
          'gap-1.5 pl-2.5 pr-8',
          isActive &&
            isDirtyEditor &&
            'border-primary bg-primary text-primary-foreground shadow-primary/24',
        )}
        onClick={handleClick}
        onMouseDown={onMouseDown}
        onDoubleClick={handleDoubleClick}
        onContextMenu={handleContextMenu}
        onKeyDown={handleKeyDown}
        onAuxClick={handleAuxClick}
      >
        <div className="grid size-3.5 shrink-0 place-content-center">
          {buffer.path === 'extensions://marketplace' ? (
            <Package className="text-muted-foreground" />
          ) : buffer.type === 'branchReview' ? (
            <GitPullRequest className="text-muted-foreground" />
          ) : buffer.type === 'diff' && isMultiFileDiff(buffer.diffData) ? (
            <GitBranch className="text-muted-foreground" />
          ) : buffer.type === 'terminal' ? (
            <Terminal className="text-muted-foreground" />
          ) : buffer.type === 'agentChat' ? (
            <AgentChatTabIcon wsId={buffer.wsId} chatId={buffer.chatId} />
          ) : (
            <FileExplorerIcon
              fileName={getDiffIconName() ?? buffer.name}
              isDir={false}
              className={cn(
                'text-muted-foreground',
                isActive && isDirtyEditor && 'text-primary-foreground',
              )}
              size={14}
            />
          )}
        </div>
        <span
          className={cn(
            'ui-font text-[13px] min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap',
            !isActive && 'text-muted-foreground',
            buffer.isPreview && 'italic',
          )}
          title={buffer.path}
        >
          {displayName}
        </span>
        {isDirtyEditor && !isActive && (
          <div
            className="size-2 shrink-0 rounded-full bg-primary"
            title="Unsaved changes"
            role="img"
            aria-label="Unsaved changes"
          />
        )}
      </Tab>

      {/* Close / pin button — rendered OUTSIDE the Tab <button> to avoid
          invalid nested-button HTML that breaks event propagation. */}
      {!buffer.isUncloseable && (
        <Button
          variant="ghost"
          data-no-dnd
          onClick={(e) => {
            e.stopPropagation()
            if (buffer.isPinned) {
              handleTabPin(buffer.id)
            } else {
              handleTabClose(buffer.id)
            }
          }}
          className={cn(
            'absolute inset-y-0 my-auto right-1.5 !size-5 !min-h-0 !min-w-0 grid place-items-center cursor-pointer select-none !rounded-md !p-0 text-muted-foreground transition-opacity',
            buffer.isPinned || isActive ? 'opacity-60' : 'opacity-0 group-hover/tab:opacity-60',
            isActive && isDirtyEditor && 'text-primary-foreground',
          )}
          tooltip={buffer.isPinned ? 'Unpin tab' : 'Close'}
          tooltipSide="bottom"
          shortcut={buffer.isPinned ? undefined : 'mod+w'}
          tabIndex={-1}
          draggable={false}
        >
          {buffer.isPinned ? (
            <Pin className="pointer-events-none select-none size-3.5 fill-current text-secondary" />
          ) : (
            <X className="pointer-events-none select-none size-3" />
          )}
        </Button>
      )}
    </div>
  )
}, arePropsEqual)

/**
 * Compares ONLY the buffer fields this component actually renders. The buffer
 * object identity changes on every content flush (immer copy-on-write), so a
 * naive `prev.buffer === next.buffer` check would re-render the active tab on
 * every keystroke even though nothing it draws has changed. Enumerated from the
 * fields read in the render body: id, type, name, path, isPinned, isPreview,
 * isUncloseable, the editor dirty flag, the diff payload and the agent-chat ids.
 *
 * Exported and REUSED by TabBar's `useRenderedPaneBuffers` subscription
 * (tab-bar.tsx) so the field set that gates the WHOLE tab strip's re-render is
 * the exact same one that gates each individual tab's — the two can never
 * drift. Any field added here is automatically respected there. Keep the field
 * list in sync with the render body above.
 */

function arePropsEqual(prev: TabBarItemProps, next: TabBarItemProps): boolean {
  return (
    prev.displayName === next.displayName &&
    prev.index === next.index &&
    prev.isActive === next.isActive &&
    prev.isDraggedTab === next.isDraggedTab &&
    prev.showDropIndicatorBefore === next.showDropIndicatorBefore &&
    prev.tabRef === next.tabRef &&
    prev.onSelect === next.onSelect &&
    prev.onMouseDown === next.onMouseDown &&
    prev.onDoubleClick === next.onDoubleClick &&
    prev.onContextMenu === next.onContextMenu &&
    prev.onKeyDown === next.onKeyDown &&
    prev.handleTabClose === next.handleTabClose &&
    prev.handleTabPin === next.handleTabPin &&
    sameRenderedBuffer(prev.buffer, next.buffer)
  )
}

function isMultiFileDiff(diffData: GitDiff | MultiFileDiff | undefined): diffData is MultiFileDiff {
  return Boolean(diffData && 'files' in diffData)
}

function getDiffFileName(diff: GitDiff): string {
  const filePath = diff.new_path || diff.old_path || diff.file_path || ''
  return getBaseName(filePath, filePath || 'diff')
}

export default TabBarItem
