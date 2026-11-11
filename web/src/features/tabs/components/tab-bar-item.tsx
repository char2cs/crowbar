import { LockIcon, PushPin as Pin, X } from '@phosphor-icons/react'
import { memo, useCallback } from 'react'
import type { RefCallback } from 'react'
import type { PaneContent } from '@/features/panes/types/pane-content'
import { sidebarSurfaceOf } from '@/features/panes/lib/sidebar-surface'
import { sameRenderedBuffer } from './tab-bar-item-utils'
import { TabBufferIcon } from './tab-buffer-icon'
import { Button } from '@/components/ui/button'
import { Tab } from '@/components/ui/tabs'
import { WithTooltip } from '@/components/ui/tooltip'
import { cn } from '@/utils/cn'
import { useWorkspaceReadOnly } from '@/features/editor/hooks/use-workspace-read-only'

interface TabBarItemProps {
  buffer: PaneContent
  displayName: string
  index: number
  isActive: boolean
  isDraggedTab: boolean
  showDropIndicatorBefore?: boolean
  tabRef?: RefCallback<HTMLDivElement>
  // Delegated handlers: one stable reference shared by every item, which passes its own buffer /
  // index when it fires, so the memo comparator below can skip unchanged tabs.
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

  const handleAuxClick = useCallback(
    (e: React.MouseEvent) => {
      // Only handle middle click here
      if (e.button !== 1) return

      handleTabClose(buffer.id)
    },
    [handleTabClose, buffer.id],
  )

  // Unsaved editor tab: when active, a blue border over the normal active fill; when inactive,
  // a bright dot.
  const isDirtyEditor = buffer.type === 'editor' && buffer.isDirty
  const readOnly = useWorkspaceReadOnly(buffer.type === 'editor' ? buffer.workspaceId : null)

  return (
    /* `group/tab` drives the close button's hover reveal. The close/pin Button is a SIBLING of the
       Tab <button>: nested buttons are invalid HTML, and dnd-kit's PointerSensor took the close
       button's pointerdown as a drag start (selecting the tab) before its onClick ran. */
    <div
      ref={tabRef}
      data-sidebar-surface={sidebarSurfaceOf(buffer.type)}
      className="group/tab relative max-w-[200px]"
    >
      {showDropIndicatorBefore ? (
        <div className="drop-indicator absolute top-1 bottom-1 left-0 z-20 w-0.5 bg-secondary" />
      ) : null}
      <WithTooltip
        side="bottom"
        content={
          <>
            <div>{displayName}</div>
            {buffer.type === 'editor' && <div className="text-muted-foreground">{buffer.path}</div>}
            {readOnly && (
              <div className="text-muted-foreground">Read-only: this branch is locked</div>
            )}
          </>
        }
        trigger={
          <Tab
            role="tab"
            aria-selected={isActive}
            aria-label={`${buffer.name}${buffer.type === 'editor' && buffer.isDirty ? ' (unsaved)' : ''}${buffer.isPinned ? ' (pinned)' : ''}${buffer.isPreview ? ' (preview)' : ''}${readOnly ? ' (read-only)' : ''}`}
            tabIndex={isActive ? 0 : -1}
            isActive={isActive}
            isDragged={isDraggedTab}
            variant="ghost"
            className={cn(
              'h-8 max-w-full',
              'gap-1.5 pl-2.5 pr-8',
              // Unsaved+active signals with the border only; the fill stays the ghost
              // variant's muted active one. Shadows: TAB_ACTIVE's (view-row.tsx).
              isActive &&
                isDirtyEditor &&
                'rounded-sm border-primary shadow-xs shadow-black/10 ' +
                  'not-disabled:inset-shadow-[0_1px_var(--elevated-highlight)] ' +
                  'active:inset-shadow-[0_1px_--theme(--color-black/8%)] active:shadow-none',
            )}
            onClick={handleClick}
            onMouseDown={onMouseDown}
            onDoubleClick={handleDoubleClick}
            onContextMenu={handleContextMenu}
            onKeyDown={handleKeyDown}
            onAuxClick={handleAuxClick}
          >
            <div className="grid size-3.5 shrink-0 place-content-center">
              <TabBufferIcon buffer={buffer} />
            </div>
            <span
              className={cn(
                'ui-font text-[13px] min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap',
                !isActive && 'text-muted-foreground',
                buffer.isPreview && 'italic',
              )}
            >
              {displayName}
            </span>
            {readOnly && <LockIcon className="size-3 shrink-0 text-muted-foreground" />}
            {isDirtyEditor && !isActive && (
              <div
                className="size-2 shrink-0 rounded-full bg-primary"
                title="Unsaved changes"
                role="img"
                aria-label="Unsaved changes"
              />
            )}
          </Tab>
        }
      />

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
            <Pin className="pointer-events-none select-none size-3.5 fill-current text-secondary" />
          ) : (
            <X className="pointer-events-none select-none size-3" />
          )}
        </Button>
      )}
    </div>
  )
}, arePropsEqual)

// Memo comparator. Buffer identity changes on every content flush (immer), so buffer fields go
// through sameRenderedBuffer, which TabBar's useRenderedPaneBuffers also uses.

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

export default TabBarItem
