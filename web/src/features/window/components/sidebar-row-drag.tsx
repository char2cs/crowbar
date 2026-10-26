import { createContext, use, useMemo, useRef, useState, type ReactNode } from 'react'
import {
  DndContext,
  DragOverlay,
  TouchSensor,
  useDraggable,
  useSensor,
  useSensors,
  type DragMoveEvent,
  type DragStartEvent,
} from '@dnd-kit/core'
import { dropModeAt, type DropMode } from '@/components/tree-dnd/drop-core'
import type { NativeRowDrop } from '@/lib/native-chrome'
import { postToShell } from '@/lib/shell-page-bridge'
import { cn } from '@/lib/utils'

interface Hit {
  id: string
  mode: DropMode
}

const DragContext = createContext<{ hit: Hit | null; claimed: boolean }>({
  hit: null,
  claimed: false,
})

// The sensor activates after a hold even with a still finger, which is the native long-press
// menu's gesture; the drag is only claimed (and the menu held back) once the row has moved.
const HOLD = { delay: 250, tolerance: 6 }
const CLAIM_PX = 8

function pointerY(event: DragMoveEvent): number | null {
  const start = event.activatorEvent
  const y = 'touches' in start ? (start as TouchEvent).touches[0]?.clientY : null
  return y === null || y === undefined ? null : y + event.delta.y
}

function hitAt(x: number, y: number, drop: NativeRowDrop | null): Hit | null {
  const el = document.elementFromPoint(x, y)?.closest<HTMLElement>('[data-sidebar-row-id]')
  const id = el?.getAttribute('data-sidebar-row-id')
  const target = id ? drop?.targets[id] : undefined
  if (!el || !id || !target) return null
  const { top, height } = el.getBoundingClientRect()
  const mode = dropModeAt(height ? (y - top) / height : 0.5, target.modes, target.band)
  return mode ? { id, mode } : null
}

/**
 * Press-and-hold reordering for the phone's chat list: the page draws the lift and the
 * indicator, and the main webview, which knows the desktop's drop rules, answers where the
 * row may land (`drop`) and carries out the move.
 */
export function RowDragScope({
  drop,
  overlay,
  children,
}: {
  drop: NativeRowDrop | null
  overlay: (rowId: string) => ReactNode
  children: ReactNode
}) {
  const sensors = useSensors(useSensor(TouchSensor, { activationConstraint: HOLD }))
  const [dragged, setDragged] = useState<string | null>(null)
  const [claimed, setClaimed] = useState(false)
  const [hit, setHit] = useState<Hit | null>(null)
  const startX = useRef(0)
  const state = useMemo(() => ({ hit, claimed }), [hit, claimed])

  const end = (move: boolean) => {
    setDragged(null)
    setClaimed(false)
    setHit(null)
    if (!claimed) return
    if (move && dragged && hit) {
      postToShell('sidebar', {
        type: 'moveRow',
        rowId: dragged,
        targetId: hit.id,
        position: hit.mode,
      })
    } else postToShell('sidebar', { type: 'dragRow', rowId: null })
    postToShell('sidebar', { type: 'rowDrag', active: false })
  }

  return (
    <DndContext
      sensors={sensors}
      onDragStart={(event: DragStartEvent) => {
        const id = String(event.active.id)
        const start = event.activatorEvent
        startX.current = 'touches' in start ? ((start as TouchEvent).touches[0]?.clientX ?? 0) : 0
        setDragged(id)
      }}
      onDragMove={(event) => {
        if (!claimed) {
          if (Math.hypot(event.delta.x, event.delta.y) < CLAIM_PX) return
          setClaimed(true)
          postToShell('sidebar', { type: 'rowDrag', active: true })
          postToShell('sidebar', { type: 'dragRow', rowId: String(event.active.id) })
        }
        const y = pointerY(event)
        setHit(y === null ? null : hitAt(startX.current + event.delta.x, y, drop))
      }}
      onDragEnd={() => end(true)}
      onDragCancel={() => end(false)}
    >
      <DragContext value={state}>{children}</DragContext>
      <DragOverlay dropAnimation={null}>
        {dragged && claimed && (
          <div className="rounded-lg bg-chrome-bg shadow-lg ring-1 ring-border">
            {overlay(dragged)}
          </div>
        )}
      </DragOverlay>
    </DndContext>
  )
}

/** A row the user can hold and move, with the drop indicator drawn on it while it is the target. */
export function DraggableRow({ id, children }: { id: string; children: ReactNode }) {
  const { setNodeRef, listeners, attributes, isDragging } = useDraggable({ id })
  const { hit, claimed } = use(DragContext)
  const mode = hit?.id === id ? hit.mode : null
  // dnd-kit's role and tab stop would shadow the treeitem the row already is.
  const { role: _role, tabIndex: _tabIndex, ...aria } = attributes
  return (
    <div
      ref={setNodeRef}
      {...listeners}
      {...aria}
      className={cn('relative', isDragging && claimed && 'opacity-40')}
    >
      {children}
      {mode === 'into' && (
        <div className="pointer-events-none absolute inset-0 rounded-lg ring-2 ring-primary" />
      )}
      {(mode === 'before' || mode === 'after') && (
        <div
          className={cn(
            'pointer-events-none absolute inset-x-2 h-0.5 rounded-full bg-primary',
            mode === 'before' ? 'top-0' : 'bottom-0',
          )}
        />
      )}
    </div>
  )
}
