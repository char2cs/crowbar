import type { ReactNode, RefObject } from 'react'
import { cn } from '@/lib/utils'
import { EdgeDissolve } from '@/components/ui/edge-dissolve'
import { IS_MAC } from '@/utils/platform'
import { CHROME_ROW_HEIGHT_PX } from '@/lib/chrome-metrics'

const ROW_HEIGHT_PX = CHROME_ROW_HEIGHT_PX
const CHAT_BLUR_EXTRA_PX = 56

interface PaneTopRowProps {
  rowRef: RefObject<HTMLDivElement | null>
  isBottomPane: boolean
  isAtLeftEdge: boolean
  isAtTopEdge: boolean
  variant?: 'opaque' | 'chat-blur'
  overlay?: boolean
  className?: string
  children: ReactNode
  paneId?: string
}

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
