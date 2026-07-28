import { forwardRef } from 'react'
import { createPortal } from 'react-dom'
import { cn } from '@/utils/cn'

/**
 * Offset from the cursor. Keeps the chip clear of the pointer glyph itself, so
 * the row under the pointer (the drop target) stays readable mid-drag.
 */
export const DRAG_GHOST_OFFSET_X = 12
export const DRAG_GHOST_OFFSET_Y = -10

interface DragGhostProps {
  label: string
  /**
   * Where the drag started. Only positions the FIRST paint — every move after
   * that writes .style.left/.top through the forwarded ref, so dragging never
   * re-renders the list it was torn from.
   */
  origin: { x: number; y: number } | null
  className?: string
}

/** The chip that follows the cursor while a sidebar row is being dragged. */
export const DragGhost = forwardRef<HTMLDivElement, DragGhostProps>(function DragGhost(
  { label, origin, className },
  ref,
) {
  // Portalled to the body because it is positioned in raw viewport coordinates.
  // Rendered inline it would be laid out against the nearest transformed
  // ancestor instead — which the peeked sidebar card is, while it is on screen —
  // so the chip drifted off the cursor and was clipped away entirely at the
  // card's edge. See sidebar-peek.tsx.
  return createPortal(
    <div
      ref={ref}
      data-drag-ghost=""
      className={cn(
        'pointer-events-none fixed z-50 max-w-56 truncate rounded-md border border-border bg-secondary px-2 py-1 text-[13px] text-secondary-foreground opacity-90 shadow-md',
        className,
      )}
      style={{
        left: origin ? origin.x + DRAG_GHOST_OFFSET_X : 0,
        top: origin ? origin.y + DRAG_GHOST_OFFSET_Y : 0,
      }}
    >
      {label}
    </div>,
    document.body,
  )
})
