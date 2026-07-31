import { forwardRef, useCallback, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { cn } from '@/utils/cn'

/**
 * Offset from the cursor. Keeps the ghost clear of the pointer glyph itself, so
 * the row under the pointer (the drop target) stays readable mid-drag.
 */
export const DRAG_GHOST_OFFSET_X = 12
export const DRAG_GHOST_OFFSET_Y = -10

/** How many rows the stack draws before the count badge takes over. */
export const DRAG_GHOST_MAX_ROWS = 3

/** Step between stacked clones, both axes. */
const STACK_STEP_PX = 4

/** The ones behind the front row. */
const STACK_BACK_OPACITY = '0.25'

interface DragGhostProps {
  /**
   * Where the drag started. Only positions the FIRST paint — every move after
   * that writes .style.left/.top through the forwarded ref, so dragging never
   * re-renders the list it was torn from.
   */
  origin: { x: number; y: number } | null
  children: ReactNode
  className?: string
}

/**
 * The thing that follows the cursor while a row is being dragged.
 *
 * Portalled to the body because it is positioned in raw viewport coordinates.
 * Rendered inline it would be laid out against the nearest transformed
 * ancestor instead — which the peeked sidebar card is, while it is on screen —
 * so it drifted off the cursor and was clipped away entirely at the card's
 * edge. See sidebar-peek.tsx.
 */
export const DragGhost = forwardRef<HTMLDivElement, DragGhostProps>(function DragGhost(
  { origin, children, className },
  ref,
) {
  return createPortal(
    <div
      ref={ref}
      data-drag-ghost=""
      className={cn('pointer-events-none fixed z-50', className)}
      style={{
        left: origin ? origin.x + DRAG_GHOST_OFFSET_X : 0,
        top: origin ? origin.y + DRAG_GHOST_OFFSET_Y : 0,
      }}
    >
      {children}
    </div>,
    document.body,
  )
})

/** A plain text chip, for a list whose rows are windowed and cannot be cloned. */
export function DragGhostChip({ label, className }: { label: string; className?: string }) {
  return (
    <div
      className={cn(
        'max-w-56 truncate rounded-md border border-border bg-secondary px-2 py-1 text-[13px] text-secondary-foreground opacity-90 shadow-md',
        className,
      )}
    >
      {label}
    </div>
  )
}

/** The grabbed rows, cloned. See {@link DragGhostRows}. */
export interface GhostRows {
  /** Detached clones, front-most first. Already styled for the stack. */
  nodes: HTMLElement[]
  /** The grabbed row's own box, so the stack reserves the space it needs. */
  width: number
  height: number
  /** How many rows the drag carries, including the ones past the stack. */
  count: number
}

/**
 * The dragged rows themselves, cloned — not a differently-styled chip.
 *
 * What you are carrying should look like what you grabbed; a chip made you
 * translate between two representations of the same row mid-gesture. Up to
 * three stack, the ones behind faded, and a badge carries the count so a
 * multi-row drag says how much it is moving without drawing all of it.
 */
export function DragGhostRows({ rows }: { rows: GhostRows }) {
  // A callback ref rather than an effect: the clones are detached DOM and this
  // is the commit that has to attach them. As an effect the ghost would paint
  // empty for a frame at the start of every drag.
  const mount = useCallback(
    (el: HTMLDivElement | null) => {
      if (!el) return
      for (const node of rows.nodes) el.appendChild(node)
    },
    [rows.nodes],
  )

  const depth = Math.max(0, rows.nodes.length - 1) * STACK_STEP_PX
  return (
    <div
      className="relative"
      style={{ width: rows.width + depth, height: rows.height + depth }}
      data-drag-ghost-rows={rows.count}
    >
      <div ref={mount} className="absolute inset-0" />
      {rows.count > 1 && (
        <span className="absolute -right-1.5 -top-1.5 inline-flex h-4 min-w-4 items-center justify-center rounded-full bg-primary px-1 text-[10px] font-medium text-primary-foreground tabular-nums shadow-sm">
          {rows.count}
        </span>
      )}
    </div>
  )
}

/**
 * Clone the live rows into the detached, pre-styled nodes {@link DragGhostRows}
 * mounts.
 *
 * Every rect is read before the first style is written: interleaving them would
 * force a layout per clone, on the one frame of a drag that is already doing
 * the most work.
 */
export function cloneGhostRows(elements: readonly HTMLElement[], count: number): GhostRows {
  const shown = elements.slice(0, DRAG_GHOST_MAX_ROWS)
  const rects = shown.map((el) => el.getBoundingClientRect())

  const nodes = shown.map((el, i) => {
    const clone = el.cloneNode(true) as HTMLElement
    clone.removeAttribute('id')
    clone.style.position = 'absolute'
    clone.style.left = '0'
    clone.style.top = '0'
    clone.style.margin = '0'
    clone.style.width = `${rects[i].width}px`
    clone.style.transform = `translate(${i * STACK_STEP_PX}px, ${i * STACK_STEP_PX}px)`
    clone.style.opacity = i === 0 ? '1' : STACK_BACK_OPACITY
    clone.style.zIndex = String(DRAG_GHOST_MAX_ROWS - i)
    return clone
  })

  return { nodes, width: rects[0]?.width ?? 0, height: rects[0]?.height ?? 0, count }
}
