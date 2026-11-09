import { forwardRef, useCallback, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { cn } from '@/lib/utils'
import { ROW_HAS_VIEW_IDLE, ROW_INACTIVE, ROW_NEST_TARGET, TAB_IDLE } from './workspace-row-base'

/** Where the pointer sits inside the grabbed row; subtracted from the pointer so the ghost
 *  stays under the spot you took hold of rather than snapping its corner to the cursor. */
export interface GrabOffset {
  dx: number
  dy: number
}

/** Clamped to the row: a press on a child that has since re-laid-out (hover controls appearing)
 *  must not place the ghost somewhere the row never was. */
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
  /** Top-left at drag start (pointer less grab offset), FIRST paint only: later moves write
   *  .style.transform through the ref, so dragging never re-renders the list it left. */
  origin: { x: number; y: number } | null
  children: ReactNode
  className?: string
}

/** The ghost's per-frame position, as a compositor-only transform. */
export function ghostTransform(x: number, y: number): string {
  return `translate3d(${x}px, ${y}px, 0)`
}

// Follows the cursor during a row drag. Portalled to body because it uses viewport coordinates:
// inline, a transformed ancestor (the peeked sidebar card) would offset and clip it.
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
        // TRANSFORM, never left/top: it moves every frame, and a composited translate skips layout
        // and repaint of the (possibly huge) pane underneath. No `will-change`: a moving transform
        // already gets the layer promotion.
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

// The dragged rows themselves, cloned, so what you carry looks like what you grabbed. Up to three
// stack with the back ones faded; a badge carries the full count.
export function DragGhostRows({ rows }: { rows: GhostRows }) {
  // A callback ref, not an effect: attaching in this commit avoids an empty first frame.
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

// The raised white control surface, pinned in every mode: --background goes charcoal in dark,
// which vanishes over the dark chrome the ghost is dragged across.
export const LIFTED =
  'control-surface outline outline-1 outline-border! shadow-md shadow-black/15 ' +
  'inset-shadow-[0_1px_var(--elevated-highlight)]'

const AMBIENT = `${ROW_INACTIVE} ${ROW_HAS_VIEW_IDLE} ${ROW_NEST_TARGET} ${TAB_IDLE}`.split(/\s+/)

// A clone drops what it wore at rest, open or as a drop target (keyed to the sidebar's surface,
// near invisible over the editor) and takes the lifted surface. The source row is unchanged.
function liftClone(clone: HTMLElement): void {
  for (const cls of AMBIENT) clone.classList.remove(cls)
  for (const el of [
    clone,
    ...clone.querySelectorAll<HTMLElement>('[class*="foreground-inverse"]'),
  ]) {
    for (const cls of [...el.classList]) {
      if (cls.includes('foreground-inverse')) el.classList.remove(cls)
    }
  }
  for (const cls of LIFTED.split(/\s+/)) clone.classList.add(cls)
}

// Clones the live rows into the detached nodes {@link DragGhostRows} mounts. Every rect is read
// before any style is written, so the drag's busiest frame forces one layout, not one per clone.
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

/**
 * A whole view tab, cloned: every member of a group, its separators and its close
 * button, not a stack of member rows. Only the lifted surface is added, so what is
 * carried is exactly what was grabbed, in or out of the strip.
 */
export function cloneGhostTab(shell: HTMLElement): GhostRows {
  const rect = shell.getBoundingClientRect()
  const clone = shell.cloneNode(true) as HTMLElement
  clone.removeAttribute('id')
  // The clone sits in the document beside the real tab: a duplicate test id would shadow it.
  for (const el of [clone, ...clone.querySelectorAll<HTMLElement>('[data-testid]')]) {
    el.removeAttribute('data-testid')
  }
  liftClone(clone)
  clone.style.position = 'absolute'
  clone.style.left = '0'
  clone.style.top = '0'
  clone.style.margin = '0'
  clone.style.width = `${rect.width}px`
  return { nodes: [clone], width: rect.width, height: rect.height, count: 1 }
}
