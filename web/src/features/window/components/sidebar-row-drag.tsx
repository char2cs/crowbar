import {
  createContext,
  use,
  useEffect,
  useEffectEvent,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
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
import { ROW_NEST_TARGET } from '@/components/layout/workspace-row-base'
import { dropModeAt, type DropMode } from '@/components/tree-dnd/drop-core'
import type { NativeRowDrop } from '@/lib/native-chrome'
import { MENU_DWELL_MS, rowTouch, type RowTouch } from '@/features/window/lib/row-touch'
import { postToShell } from '@/lib/shell-page-bridge'
import { cn } from '@/lib/utils'

interface Held {
  /** The draggable's id: a row's own, or the Active section's view of it. */
  id: string
  rowId: string
  /** Only the menu is wanted; the row is not something to move. */
  menuOnly: boolean
  node: HTMLElement
}

interface Hit {
  id: string
  mode: DropMode
}

const DragContext = createContext<{ hit: Hit | null; claimed: boolean }>({
  hit: null,
  claimed: false,
})

// The sensor activates after a hold even with a still finger, which is the menu's gesture;
// rowTouch decides from there whether the touch becomes a drag or the row's menu.
const HOLD = { delay: 250, tolerance: 6 }

function pointerY(event: DragMoveEvent): number | null {
  const start = event.activatorEvent
  const y = 'touches' in start ? (start as TouchEvent).touches[0]?.clientY : null
  return y === null || y === undefined ? null : y + event.delta.y
}

// The lifted copy and dnd-kit's wrapper around it are under the finger: the row beneath is the target.
function hitAt(x: number, y: number, drop: NativeRowDrop | null): Hit | null {
  let el: HTMLElement | null = null
  for (const at of document.elementsFromPoint(x, y)) {
    if (at.closest('[data-drag-overlay], [data-menu-only]')) continue
    el = at.closest<HTMLElement>('[data-sidebar-row-id]')
    if (el) break
  }
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
  onMenu,
  children,
}: {
  drop: NativeRowDrop | null
  /** The hold stayed put: the row's menu is wanted, anchored on the held element. */
  onMenu: (rowId: string, node: HTMLElement) => void
  overlay: (rowId: string) => ReactNode
  children: ReactNode
}) {
  const sensors = useSensors(useSensor(TouchSensor, { activationConstraint: HOLD }))
  const [held, setHeld] = useState<Held | null>(null)
  const dragged = held?.rowId ?? null
  const [touch, setTouch] = useState<RowTouch>('idle')
  const claimed = touch === 'drag'
  const [hit, setHit] = useState<Hit | null>(null)
  const startX = useRef(0)
  // The shell's answer lands after the finger has moved on, or while it rests: the target is
  // read again from where the finger is when the answer arrives.
  const finger = useRef<{ x: number; y: number } | null>(null)
  const state = useMemo(() => ({ hit, claimed }), [hit, claimed])

  useEffect(() => {
    if (claimed && finger.current) setHit(hitAt(finger.current.x, finger.current.y, drop))
  }, [drop, claimed])

  const openMenu = useEffectEvent(() => {
    if (held) onMenu(held.rowId, held.node)
  })
  useEffect(() => {
    if (touch !== 'armed') return
    const dwell = setTimeout(() => {
      setTouch(rowTouch('armed', { type: 'dwell' }))
      openMenu()
    }, MENU_DWELL_MS)
    return () => clearTimeout(dwell)
  }, [touch])

  // The shell hears of the lift from here, for its haptic.
  useEffect(() => {
    if (!claimed || !dragged) return
    postToShell('sidebar', { type: 'rowDrag', active: true })
    postToShell('sidebar', { type: 'dragRow', rowId: dragged })
  }, [claimed, dragged])

  const end = (move: boolean) => {
    setHeld(null)
    setTouch(rowTouch(touch, { type: 'release' }))
    setHit(null)
    finger.current = null
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
        const start = event.activatorEvent
        const node = (start.target as Element | null)?.closest<HTMLElement>('[data-held-row]')
        if (!node) return
        const data = event.active.data.current as { rowId: string; menuOnly: boolean }
        startX.current = 'touches' in start ? ((start as TouchEvent).touches[0]?.clientX ?? 0) : 0
        setHeld({ id: String(event.active.id), rowId: data.rowId, menuOnly: data.menuOnly, node })
        setTouch(rowTouch('idle', { type: 'hold' }))
      }}
      onDragMove={(event) => {
        const moved = rowTouch(touch, { type: 'move', dx: event.delta.x, dy: event.delta.y })
        // A view row never lifts: a moving finger ends its hold.
        const owner = held?.menuOnly && moved === 'drag' ? 'idle' : moved
        setTouch(owner)
        if (owner !== 'drag') return
        const y = pointerY(event)
        finger.current = y === null ? null : { x: startX.current + event.delta.x, y }
        setHit(finger.current && hitAt(finger.current.x, finger.current.y, drop))
      }}
      onDragEnd={() => end(true)}
      onDragCancel={() => end(false)}
    >
      <DragContext value={state}>{children}</DragContext>
      <DragOverlay dropAnimation={null}>
        {dragged && claimed && (
          <div
            data-drag-overlay=""
            className="rounded-lg bg-chrome-bg shadow-lg ring-1 ring-border"
          >
            {overlay(dragged)}
          </div>
        )}
      </DragOverlay>
    </DndContext>
  )
}

/** A row the user can hold and move, with the drop indicator drawn on it while it is the target. */
export function DraggableRow({
  id,
  menuOnly,
  disabled,
  children,
}: {
  id: string
  /** A view over a row elsewhere: held for its menu, never lifted or dropped on. */
  menuOnly?: boolean
  /** A row on its way out is held still: it is not something to move. */
  disabled?: boolean
  children: ReactNode
}) {
  const { setNodeRef, listeners, attributes, isDragging } = useDraggable({
    id: menuOnly ? `menu:${id}` : id,
    disabled,
    data: { rowId: id, menuOnly: !!menuOnly },
  })
  const { hit, claimed } = use(DragContext)
  const mode = hit?.id === id ? hit.mode : null
  // dnd-kit's role and tab stop would shadow the treeitem the row already is.
  const { role: _role, tabIndex: _tabIndex, ...aria } = attributes
  return (
    <div
      ref={setNodeRef}
      {...listeners}
      {...aria}
      data-held-row=""
      data-menu-only={menuOnly ? '' : undefined}
      className={cn('relative', isDragging && claimed && 'opacity-40')}
    >
      {children}
      {mode === 'into' && (
        <div
          className={cn(
            'pointer-events-none absolute inset-0 rounded-lg border opacity-100',
            ROW_NEST_TARGET,
          )}
        />
      )}
      {(mode === 'before' || mode === 'after') && (
        <div
          data-drop-indicator={mode}
          className={cn(
            // The desktop's hairline and end-cap (components/layout/drop-indicator.tsx).
            'pointer-events-none absolute right-0.5 left-2 h-0.5 rounded-full bg-sidebar-drop-line',
            "before:absolute before:top-1/2 before:-left-1.5 before:size-1 before:-translate-x-0.5 before:-translate-y-1/2 before:rounded-full before:border-2 before:border-sidebar-drop-line before:content-['']",
            mode === 'before' ? 'top-0' : 'bottom-0',
          )}
        />
      )}
    </div>
  )
}
