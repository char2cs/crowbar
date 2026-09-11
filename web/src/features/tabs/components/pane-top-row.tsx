import type { ReactNode, RefObject } from 'react'
import { cn } from '@/lib/utils'
import { EdgeDissolve } from '@/components/ui/edge-dissolve'
import { IS_MAC } from '@/utils/platform'

const ROW_HEIGHT_PX = IS_MAC ? 44 : 34
// How far past its own height the glass extends downward, giving scrolled
// text room to ramp from sharp to fully blurred rather than switching over
// too few pixels to read as a gradient. The composer's own dissolve zone
// (composer.css) runs roughly 2-4x its dock's height; this is the same
// proportion applied to a much shorter header.
const CHAT_BLUR_EXTRA_PX = 56

interface PaneTopRowProps {
  /** From the caller's own `usePaneTopRowEdges()` call — kept OUT of this
   *  component so a caller that already measures edges for another reason
   *  (TabBar's `useTabBarScroll`, for its reopen-sidebar-toggle placement)
   *  doesn't end up with two ResizeObservers on the same element. */
  rowRef: RefObject<HTMLDivElement | null>
  isBottomPane: boolean
  isAtLeftEdge: boolean
  isAtTopEdge: boolean
  /**
   * 'opaque' (default): the IDE sector's own look — a flat pane-background
   * fill, sitting in normal flex flow above whatever it heads. Nothing
   * needs to render "behind" it.
   *
   * 'chat-blur': the chat's own glass — no fill of its own, steals the chat
   * composer's own progressive-blur "dissolve" (composer.css) so
   * scrolled-behind text blurs and fades rather than being clipped by a
   * hard edge. See `overlay` for whether it ALSO floats free of flex flow —
   * the two are independent: TabBar's own collapsed-presentation row uses
   * chat-blur's look without overlay's positioning (see that prop's own
   * doc for why).
   */
  variant?: 'opaque' | 'chat-blur'
  /**
   * Float as an absolute overlay pinned to the container's top edge instead
   * of taking flex-flow space, so whatever renders below can fill the FULL
   * box and show real content behind this row's own chat-blur glass (the
   * container needs `position: relative`, and its real content painted
   * before this in DOM order). Only meaningful paired with
   * `variant="chat-blur"`.
   *
   * TabBar's own row deliberately does NOT set this even when it switches
   * to chat-blur (collapsed presentation, chat selected): TabBar sits
   * OUTSIDE the chat/editor split entirely, so floating it would resize
   * that split's own container out from under it, not just change this
   * row's look. ChatColumnHeader and ChatOnlyPaneHeader DO set it — both
   * live INSIDE the box whose content should show through.
   */
  overlay?: boolean
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
 * `variant` differ per caller.
 */
export function PaneTopRow({
  rowRef,
  isBottomPane,
  isAtLeftEdge,
  isAtTopEdge,
  variant = 'opaque',
  overlay = false,
  className,
  children,
  paneId,
}: PaneTopRowProps) {
  const isChatBlur = variant === 'chat-blur'
  return (
    <div
      ref={rowRef}
      data-testid="pane-top-row"
      data-tab-bar-pane-id={paneId}
      className={cn(
        'flex shrink-0 items-center gap-1.5 px-2 py-1',
        overlay ? 'absolute inset-x-0 top-0 z-10' : 'relative overflow-hidden',
        !isChatBlur && 'bg-pane-background',
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
      {isChatBlur && (
        <EdgeDissolve edge="top" height={ROW_HEIGHT_PX + CHAT_BLUR_EXTRA_PX} className="-z-10" />
      )}
      {children}
    </div>
  )
}
