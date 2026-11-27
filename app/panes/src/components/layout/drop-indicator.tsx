import { forwardRef } from 'react'
import { createPortal } from 'react-dom'
import { cn } from '@/lib/utils'
import type { DropMode, RowRect } from '@/components/tree-dnd/drop-core'

export const DropIndicator = forwardRef<HTMLDivElement>(function DropIndicator(_props, ref) {
  return createPortal(
    <div
      ref={ref}
      aria-hidden="true"
      data-drop-indicator=""
      className={cn(
        'pointer-events-none fixed z-[60] hidden h-0.5 rounded-full bg-sidebar-drop-line',
        'transition-[top,left,width] duration-[60ms] ease-out motion-reduce:transition-none',
        "before:absolute before:top-1/2 before:-left-1.5 before:size-1 before:-translate-x-0.5 before:-translate-y-1/2 before:rounded-full before:border-2 before:border-sidebar-drop-line before:content-['']",
      )}
    />,
    document.body,
  )
})

const LINE_INSET_PX = 8

const LINE_TRIM_PX = 10

export interface DropLineBox {
  left: number
  top: number
  width: number
}

export type DropLineRect = RowRect

export function dropLineBox(rect: DropLineRect, position: Exclude<DropMode, 'into'>): DropLineBox {
  return {
    left: rect.left + LINE_INSET_PX,
    width: Math.max(0, rect.width - LINE_TRIM_PX),
    top: (position === 'before' ? rect.top : rect.bottom) - 1,
  }
}

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
