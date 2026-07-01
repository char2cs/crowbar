import {
  Chat,
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
import type { PaneContent } from '@/features/panes/types/pane-content'
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
  onClick?: () => void
  onMouseDown?: (e: React.MouseEvent) => void
  onDoubleClick: (e: React.MouseEvent) => void
  onContextMenu: (e: React.MouseEvent) => void
  onKeyDown: (e: React.KeyboardEvent) => void
  handleTabClose: (id: string) => void
  handleTabPin: (id: string) => void
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
        aria-label={`${buffer.name}${buffer.type === 'editor' && buffer.isDirty ? ' (unsaved)' : ''}${buffer.isPinned ? ' (pinned)' : ''}${buffer.isPreview ? ' (preview)' : ''}`}
        tabIndex={isActive ? 0 : -1}
        isActive={isActive}
        isDragged={isDraggedTab}
        className={cn('h-8', 'gap-1.5 pl-2.5 pr-8')}
        onClick={onClick}
        onMouseDown={onMouseDown}
        onDoubleClick={onDoubleClick}
        onContextMenu={onContextMenu}
        onKeyDown={onKeyDown}
        onAuxClick={handleAuxClick}
      >
        <div className="grid size-3.5 shrink-0 place-content-center">
          {buffer.type === 'crowbarChat' ? (
            <Chat className="text-muted-foreground" />
          ) : buffer.path === 'extensions://marketplace' ? (
            <Package className="text-muted-foreground" />
          ) : buffer.type === 'branchReview' ? (
            <GitPullRequest className="text-muted-foreground" />
          ) : buffer.type === 'diff' && isMultiFileDiff(buffer.diffData) ? (
            <GitBranch className="text-muted-foreground" />
          ) : buffer.type === 'terminal' ? (
            <Terminal className="text-muted-foreground" />
          ) : (
            <FileExplorerIcon
              fileName={getDiffIconName() ?? buffer.name}
              isDir={false}
              className="text-muted-foreground"
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
        {buffer.type === 'editor' && buffer.isDirty && (
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
          )}
          tooltip={buffer.isPinned ? 'Unpin tab' : 'Close'}
          tooltipSide="bottom"
          shortcut={buffer.isPinned ? undefined : 'mod+w'}
          tabIndex={-1}
          draggable={false}
        >
          {buffer.isPinned ? (
            <Pin className="pointer-events-none select-none size-3.5 fill-current text-accent" />
          ) : (
            <X className="pointer-events-none select-none size-3" />
          )}
        </Button>
      )}
    </div>
  )
})

function isMultiFileDiff(diffData: GitDiff | MultiFileDiff | undefined): diffData is MultiFileDiff {
  return Boolean(diffData && 'files' in diffData)
}

function getDiffFileName(diff: GitDiff): string {
  const filePath = diff.new_path || diff.old_path || diff.file_path || ''
  return getBaseName(filePath, filePath || 'diff')
}

export default TabBarItem
