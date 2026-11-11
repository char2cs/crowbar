import type { ReactNode, RefObject } from 'react'
import { cn } from '@/lib/utils'
import { EdgeDissolve } from '@/components/ui/edge-dissolve'
import { IS_MAC } from '@/utils/platform'
import { CHROME_ROW_HEIGHT_PX } from '@/lib/chrome-metrics'

const ROW_HEIGHT_PX = CHROME_ROW_HEIGHT_PX
// Glass extends this far below the row so scrolled text ramps from sharp to blurred over enough
// pixels to read as a gradient (the composer's dissolve uses a similar proportion).
const CHAT_BLUR_EXTRA_PX = 56

interface PaneTopRowProps {
  /** From the caller's own `usePaneTopRowEdges()`, so a caller already measuring edges (TabBar)
   *  doesn't put two ResizeObservers on the element. */
  rowRef: RefObject<HTMLDivElement | null>
  isBottomPane: boolean
  isAtLeftEdge: boolean
  isAtTopEdge: boolean
  /** 'opaque' (default): flat pane-background fill in normal flow. 'chat-blur': no fill; the
   *  composer's progressive-blur dissolve so scrolled-behind text fades instead of clipping.
   *  Independent of `overlay`. */
  variant?: 'opaque' | 'chat-blur'
  /** Float absolutely over the container's top (which needs `position: relative`) so content fills
   *  the box and shows through the chat-blur glass. TabBar never sets it: it sits outside the
   *  chat/editor split, and floating it would resize that split's container. */
  overlay?: boolean
  className?: string
  children: ReactNode
  /** Hit-test marker the tab and file-explorer drags use to find which pane a drop lands on. */
  paneId?: string
}

// The window-chrome shell (drag region, traffic-light inset, height) shared by TabBar and the chat
// headers, so the contract cannot drift between them; only content and `variant` differ.
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
        // Only the row under the macOS window controls (window top-left) reserves their inset.
        IS_MAC && !isBottomPane && isAtLeftEdge && isAtTopEdge && 'pl-[88px]',
        className,
      )}
      style={{ height: CHROME_ROW_HEIGHT_PX }}
      data-tauri-drag-region
    >
      {isChatBlur && (
        <EdgeDissolve edge="top" height={ROW_HEIGHT_PX + CHAT_BLUR_EXTRA_PX} className="-z-10" />
      )}
      {children}
    </div>
  )
}
