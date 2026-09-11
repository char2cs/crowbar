import type { ReactNode, RefObject } from 'react'
import { cn } from '@/lib/utils'
import { IS_MAC } from '@/utils/platform'

interface PaneTopRowProps {
  /** From the caller's own `usePaneTopRowEdges()` call — kept OUT of this
   *  component so a caller that already measures edges for another reason
   *  (TabBar's `useTabBarScroll`, for its reopen-sidebar-toggle placement)
   *  doesn't end up with two ResizeObservers on the same element. */
  rowRef: RefObject<HTMLDivElement | null>
  isBottomPane: boolean
  isAtLeftEdge: boolean
  isAtTopEdge: boolean
  className?: string
  children: ReactNode
  /** Hit-test marker for the sidebar/file-explorer drag arms
   *  (internal-tab-drag.ts, use-file-explorer-drag-drop.ts) to find which
   *  pane a drop lands on — set whenever the row belongs to a real pane. */
  paneId?: string
}

/**
 * The window-chrome shell every "pane top row" shares: drag region, macOS
 * traffic-light inset, the shrink-0 height. TabBar (the IDE sector's own
 * header), ChatOnlyPaneHeader (chatFillsPane), and the chat's own header in
 * side-by-side/stacked presentation all render through this ONE component so
 * the contract can never drift between them — only the CONTENT and the
 * background token differ per caller.
 */
export function PaneTopRow({
  rowRef,
  isBottomPane,
  isAtLeftEdge,
  isAtTopEdge,
  className,
  children,
  paneId,
}: PaneTopRowProps) {
  return (
    <div
      ref={rowRef}
      data-testid="pane-top-row"
      data-tab-bar-pane-id={paneId}
      className={cn(
        'relative flex shrink-0 items-center gap-1.5 overflow-hidden px-2 py-1',
        IS_MAC ? 'h-[44px]' : 'h-[34px]',
        // Traffic-light inset: only the row that actually sits under the
        // macOS window controls (window top-left) reserves the space — a
        // row elsewhere at the left edge (a vertical split's lower pane, or
        // the IDE sector's own column beside the chat) is nowhere near them.
        IS_MAC && !isBottomPane && isAtLeftEdge && isAtTopEdge && 'pl-[88px]',
        className,
      )}
      data-tauri-drag-region
    >
      {children}
    </div>
  )
}
