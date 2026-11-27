import { forwardRef, useCallback, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { cn } from '@/lib/utils'
import { ROW_HAS_VIEW_IDLE, ROW_INACTIVE, ROW_NEST_TARGET, TAB_IDLE } from './workspace-row-base'

export interface GrabOffset {
  dx: number
  dy: number
}

export function grabOffsetFrom(rect: DOMRect, pointerX: number, pointerY: number): GrabOffset {
  return {
    dx: Math.min(Math.max(pointerX - rect.left, 0), rect.width),
    dy: Math.min(Math.max(pointerY - rect.top, 0), rect.height),
  }
}

const DRAG_GHOST_MAX_ROWS = 3

const STACK_STEP_PX = 4

const STACK_BACK_OPACITY = '0.25'

interface DragGhostProps {
  origin: { x: number; y: number } | null
  children: ReactNode
  className?: string
}

export function ghostTransform(x: number, y: number): string {
  return `translate3d(${x}px, ${y}px, 0)`
}

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
        transform: ghostTransform(origin?.x ?? 0, origin?.y ?? 0),
      }}
    >
      {children}
    </div>,
    document.body,
  )
})

export interface GhostRows {
  nodes: HTMLElement[]
  width: number
  height: number
  count: number
}

export function DragGhostRows({ rows }: { rows: GhostRows }) {
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

export const LIFTED =
  'control-surface outline outline-1 outline-border! shadow-md shadow-black/15 ' +
  'inset-shadow-[0_1px_var(--elevated-highlight)]'

const AMBIENT = `${ROW_INACTIVE} ${ROW_HAS_VIEW_IDLE} ${ROW_NEST_TARGET} ${TAB_IDLE}`.split(/\s+/)

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

export function cloneGhostTab(shell: HTMLElement): GhostRows {
  const rect = shell.getBoundingClientRect()
  const clone = shell.cloneNode(true) as HTMLElement
  clone.removeAttribute('id')
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
