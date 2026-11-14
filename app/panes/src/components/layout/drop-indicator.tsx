import { forwardRef } from 'react'
import { createPortal } from 'react-dom'
import { cn } from '@/lib/utils'
import type { DropMode, RowRect } from '@/components/tree-dnd/drop-core'

// The reorder hairline ("between these rows"; a nest fills the row instead, never both). ONE
// element MOVED across slots so it travels rather than blinks and can transition; portalled to
// body so a transformed ancestor (the peek card) cannot offset it.
export const DropIndicator = forwardRef<HTMLDivElement>(function DropIndicator(_props, ref) {
  return createPortal(
    <div
      ref={ref}
      aria-hidden="true"
      data-drop-indicator=""
      className={cn(
        // Above the ghost (z-50): the ghost sits under the cursor, and the drop line must
        // never be the thing covered.
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

/** The hit test's own measured rect, so the line is drawn from the numbers the decision used. */
export type DropLineRect = RowRect

// Place the line beside the target row. `rect.left` carries the row's indent, so the inset puts
// the line at the dropped row's depth; the 1px lift centres it on the boundary.
export function dropLineBox(rect: DropLineRect, position: Exclude<DropMode, 'into'>): DropLineBox {
  return {
    left: rect.left + LINE_INSET_PX,
    width: Math.max(0, rect.width - LINE_TRIM_PX),
    top: (position === 'before' ? rect.top : rect.bottom) - 1,
  }
}

/** The line for a side-by-side strip: a vertical hairline on the tab's near edge. */
export function dropLineBoxX(
  rect: DropLineRect,
  position: Exclude<DropMode, 'into'>,
): DropLineBox & { height: number } {
  return {
    left: (position === 'before' ? rect.left : rect.left + rect.width) - 1,
    top: rect.top + 4,
    width: 2,
    height: Math.max(0, rect.bottom - rect.top - 8),
  }
}
