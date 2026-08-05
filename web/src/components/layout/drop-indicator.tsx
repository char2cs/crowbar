import { forwardRef } from 'react'
import { createPortal } from 'react-dom'
import { cn } from '@/lib/utils'
import type { DropMode } from './drop-rules'

/**
 * The reorder signal: a 2px hairline with a circle end-cap, marking the slot a
 * drop would land in.
 *
 * Two signals, never both. This one says "between these rows"; a nest says
 * "inside this one" by filling the row (ROW_NEST_TARGET). Showing a line and a
 * fill at once leaves the user to guess which of two different moves they are
 * about to make.
 *
 * ONE element for the whole drag, positioned in viewport coordinates and MOVED
 * as the pointer crosses boundaries. Drawn inside each target row instead it
 * would mount and unmount per crossing — a mark that blinks from slot to slot
 * where it should travel between them, and a transition that can never run
 * because the element it is on is new every time.
 *
 * Portalled to the body for the same reason the ghost is: rendered inline it
 * would be laid out against the nearest transformed ancestor (the peeked
 * sidebar card is one while it is on screen) rather than the viewport.
 */
export const DropIndicator = forwardRef<HTMLDivElement>(function DropIndicator(_props, ref) {
  return createPortal(
    <div
      ref={ref}
      aria-hidden="true"
      data-drop-indicator=""
      className={cn(
        // Above the ghost (z-50), not below it. The ghost is anchored to the
        // point the row was grabbed by, so it now sits under the cursor rather
        // than beside it — and the line marking where the drop lands is the one
        // thing in a drag that must never be the thing covered up.
        'pointer-events-none fixed z-[60] hidden h-0.5 rounded-full bg-sidebar-drop-line',
        // It slides between slots rather than jumping: the line is the same
        // object throughout a drag, so the move reads as one mark tracking the
        // pointer instead of several appearing in turn.
        'transition-[top,left,width] duration-[60ms] ease-out motion-reduce:transition-none',
        // The end-cap. `size-1` with a `border-2` is a 4px dot under the app's
        // border-box sizing, sitting just clear of the line's left end.
        "before:absolute before:top-1/2 before:-left-1.5 before:size-1 before:-translate-x-0.5 before:-translate-y-1/2 before:rounded-full before:border-2 before:border-sidebar-drop-line before:content-['']",
      )}
    />,
    document.body,
  )
})

/** How far the line starts inside the target row's own left border edge. */
const LINE_INSET_PX = 8

/** Taken off the row's width: the inset above, less the 2px it keeps clear of
 *  the row's right edge. */
const LINE_TRIM_PX = 10

/** Where the line sits, in viewport coordinates. */
export interface DropLineBox {
  left: number
  top: number
  width: number
}

/** The rect of a row, as the line needs to read it. */
export interface DropLineRect {
  top: number
  bottom: number
  left: number
  width: number
}

/**
 * Place the line against the row a drop would land beside.
 *
 * Rows carry their indent on the row element itself, so `rect.left` already
 * encodes depth: insetting from it puts the line at the depth the dropped row
 * will land at, which is the one thing a full-width line could never say.
 *
 * The 1px lift centres the 2px line on the boundary between the two rows rather
 * than hanging it below the edge it marks.
 */
export function dropLineBox(rect: DropLineRect, position: Exclude<DropMode, 'into'>): DropLineBox {
  return {
    left: rect.left + LINE_INSET_PX,
    width: Math.max(0, rect.width - LINE_TRIM_PX),
    top: (position === 'before' ? rect.top : rect.bottom) - 1,
  }
}
