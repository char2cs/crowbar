import { forwardRef, useCallback, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { cn } from '@/lib/utils'
import { ROW_ACTIVE, ROW_INACTIVE } from './workspace-row-base'

/**
 * Where the pointer sits INSIDE the row it grabbed, in that row's own box.
 *
 * The ghost is positioned by subtracting this from the pointer, which is what
 * makes a dragged row stay under the part of it you actually took hold of. The
 * ghost used to pin its top-left to the cursor plus a fixed offset, so whatever
 * you grabbed — the middle of a long branch name, the right end of the row — the
 * row snapped its corner to your hand the instant the drag began.
 *
 * Restored verbatim (Task 21) from before the workspace tree/chats panel
 * retirement (commit f119a402) — generic, tree-agnostic, and exactly what one
 * unified drag arm needs; only its two former consumers were tree-specific.
 */
export interface GrabOffset {
  dx: number
  dy: number
}

/**
 * Read the grab point from the row's box and the pointer that pressed it.
 *
 * Clamped to the row: a pointerdown that lands on a child which has since
 * re-laid-out (the hover controls appearing, say) must not push the ghost
 * somewhere the row never was.
 */
export function grabOffsetFrom(rect: DOMRect, pointerX: number, pointerY: number): GrabOffset {
  return {
    dx: Math.min(Math.max(pointerX - rect.left, 0), rect.width),
    dy: Math.min(Math.max(pointerY - rect.top, 0), rect.height),
  }
}

/** How many rows the stack draws before the count badge takes over. */
export const DRAG_GHOST_MAX_ROWS = 3

/** Step between stacked clones, both axes. */
const STACK_STEP_PX = 4

/** The ones behind the front row. */
const STACK_BACK_OPACITY = '0.25'

interface DragGhostProps {
  /**
   * The ghost's top-left at drag start — the pointer already less its grab
   * offset. Only positions the FIRST paint — every move after that writes
   * .style.transform through the forwarded ref, so dragging never re-renders the
   * list it was torn from.
   */
  origin: { x: number; y: number } | null
  children: ReactNode
  className?: string
}

/** The ghost's per-frame position, as a compositor-only transform. */
export function ghostTransform(x: number, y: number): string {
  return `translate3d(${x}px, ${y}px, 0)`
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
      className={cn('pointer-events-none fixed top-0 left-0 z-50', className)}
      style={{
        // TRANSFORM, never left/top.
        //
        // This moves every frame of a drag. Writing `left`/`top` puts the ghost
        // back through layout and repaints whatever it is over — and what it is
        // usually over is the editor pane, which on the New Tab surface holds a
        // 51,281-character <pre>. Measured on the live app: one pointermove per
        // frame cost 60ms with left/top against 8ms for the same pointer sweep
        // with no drag at all. A translate is composited: no layout, no repaint
        // of the content underneath.
        //
        // No `will-change`: it was here to get the layer up front, but the
        // measurement does not support the cost. Transform vs left/top was worth
        // ~3ms of a 60ms frame; the 42ms was a full-window overlay element,
        // since deleted (see index.css). A permanent hint on an element that
        // exists only during the drag buys a promotion this already gets from
        // being a moving transform.
        transform: ghostTransform(origin?.x ?? 0, origin?.y ?? 0),
      }}
    >
      {children}
    </div>,
    document.body,
  )
})

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
 * Give a cloned row the ACTIVE treatment, whatever it wore in the list.
 *
 * A row in the air is lifted, and lifted is what ROW_ACTIVE already draws — a
 * raised, inset-lit surface with a real background. An inactive row carries
 * ROW_INACTIVE instead, which is a transparent border and no fill: perfectly
 * legible in the sidebar, where it sits on the sidebar's own surface, and
 * close to invisible the moment it is picked up and flown over the editor pane.
 *
 * This says nothing about which workspace is OPEN. The source row keeps its own
 * styling (faded to `opacity-40` while the drag is in flight); only the clone —
 * which exists solely to be carried — is promoted.
 */
function liftClone(clone: HTMLElement): void {
  for (const cls of ROW_INACTIVE.split(/\s+/)) clone.classList.remove(cls)
  for (const cls of ROW_ACTIVE.split(/\s+/)) clone.classList.add(cls)
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
    liftClone(clone)
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
