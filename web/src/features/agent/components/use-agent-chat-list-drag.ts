import { useCallback, useEffect, useEffectEvent, useRef, useState } from 'react'
import {
  AGENT_CHAT_ROW_HEIGHT,
  autoScrollDelta,
  resolveDropSlotIndex,
} from './agent-chat-drop-geometry'
import type { AgentChat } from '@/features/agent/api/agent-api'

/** Where a drop right now would land: the trash, or a slot in the list. Slots
 *  are gaps between rows (0 … count), so `null` only ever means "not dragging" —
 *  a live drag always has a slot, and the panel always draws a line in it. */
type DropTarget = { kind: 'trash' } | { kind: 'slot'; index: number }

const DRAG_THRESHOLD_PX = 5

/**
 * Where the chip rides relative to the cursor.
 *
 * Its own constants rather than the workspace tree's grab-offset anchoring: this
 * list is windowed, so what follows the pointer is a DragGhostChip — a small
 * label, not a clone of the row. Anchoring a chip by the point a full-width row
 * was grabbed at would leave the cursor off the end of it.
 */
const CHIP_OFFSET_X = 12
const CHIP_OFFSET_Y = -10
// Edge auto-scroll while dragging: within this many px of the scroll container's
// top/bottom, scroll toward it by this many px per animation frame, so a drag can
// reach rows the windowed list hasn't painted.
const AUTO_SCROLL_EDGE_PX = 48
const AUTO_SCROLL_STEP_PX = 12

/** Is (x, y) inside a DOMRect? (Used for the always-painted trash zone.) */
function pointInRect(x: number, y: number, r: DOMRect): boolean {
  return r.height > 0 && x >= r.left && x <= r.right && y >= r.top && y <= r.bottom
}

// A completed drag still produces a click on the row it started from, which
// would select (and open) the dragged chat. Swallow that one click — same trick
// the workspace tree uses.
function suppressNextClick(): void {
  const swallow = (e: MouseEvent) => {
    e.stopPropagation()
    e.preventDefault()
  }
  window.addEventListener('click', swallow, { capture: true, once: true })
  setTimeout(() => window.removeEventListener('click', swallow, { capture: true }), 0)
}

interface UseAgentChatListDragParams {
  /** The virtualizer's scroll container — the drag reads its rect + scrollTop. */
  scrollRef: React.RefObject<HTMLDivElement | null>
  /** The current ordered chats — for the row count the slots are resolved against. */
  ordered: AgentChat[]
  /** Drop `dragId` into slot `slot` (the caller owns the order + persistence). */
  onReorder: (dragId: string, slot: number) => void
  /** Drop `dragId` on the trash. */
  onDelete: (chatId: string) => void
}

/**
 * The chat list's drag-to-reorder / drag-to-trash-delete interaction.
 *
 * The list is windowed, so off-screen rows aren't painted and DOM hit-testing
 * can't find them. The drop target is resolved from SCROLL GEOMETRY instead
 * (agent-chat-drop-geometry.ts): the SLOT between rows the pointer sits in, the
 * trash zone by its own rect, plus edge auto-scroll so a drag can reach far
 * rows. All pointer listeners subscribe once for the hook's life and read live
 * values through refs, so nothing re-subscribes on a render.
 */
export function useAgentChatListDrag({
  scrollRef,
  ordered,
  onReorder,
  onDelete,
}: UseAgentChatListDragParams) {
  const [draggingId, setDraggingId] = useState<string | null>(null)
  // Where a drop would land right now — null only while nothing is being dragged.
  const [hoverTarget, setHoverTarget] = useState<DropTarget | null>(null)

  // Arm on pointer-down; promote to a real drag past the movement threshold.
  const dragRef = useRef<{ id: string; startX: number; startY: number; active: boolean } | null>(
    null,
  )

  // The always-painted trash zone lives outside the scroll container, so geometry
  // doesn't cover it — resolve it by its own rect instead.
  const trashRef = useRef<HTMLDivElement | null>(null)

  // The cursor-following chip. Its position is written straight to the DOM on
  // every move — routing that through React would re-render the whole list at
  // pointer rate. State only says WHETHER it exists; the ref says where.
  const ghostRef = useRef<HTMLDivElement | null>(null)
  const ghostOriginRef = useRef<{ x: number; y: number } | null>(null)

  // Edge auto-scroll bookkeeping: the latest pointer position and the live rAF
  // frame id, both refs so the once-registered listeners never go stale.
  const autoScrollPointerRef = useRef<{ x: number; y: number } | null>(null)
  const autoScrollFrameRef = useRef<number | null>(null)

  // Read by the window-level pointer handlers, which are registered once — a ref
  // keeps them off the render-identity treadmill.
  const orderedRef = useRef(ordered)
  orderedRef.current = ordered

  // Resolve reorder/delete against the LATEST callbacks via an Effect Event, so
  // the pointer listeners subscribe once instead of re-subscribing per render.
  const onDrop = useEffectEvent((target: DropTarget | null, dragId: string) => {
    if (target?.kind === 'trash') onDelete(dragId)
    else if (target?.kind === 'slot') onReorder(dragId, target.index)
  })

  useEffect(() => {
    // What a pointer at (x, y) would drop onto: the trash zone (its own rect),
    // else the slot between rows it sits in (scroll geometry). A slot always
    // resolves — reorderIds is what decides that the two slots touching the
    // dragged row leave the list alone.
    const resolveTarget = (x: number, y: number): DropTarget | null => {
      const trashEl = trashRef.current
      if (trashEl && pointInRect(x, y, trashEl.getBoundingClientRect())) return { kind: 'trash' }

      const scrollEl = scrollRef.current
      if (!scrollEl) return null
      const rect = scrollEl.getBoundingClientRect()
      return {
        kind: 'slot',
        index: resolveDropSlotIndex({
          pointerY: y,
          containerTop: rect.top,
          scrollTop: scrollEl.scrollTop,
          rowHeight: AGENT_CHAT_ROW_HEIGHT,
          count: orderedRef.current.length,
        }),
      }
    }

    const stopAutoScroll = () => {
      if (autoScrollFrameRef.current !== null) {
        cancelAnimationFrame(autoScrollFrameRef.current)
        autoScrollFrameRef.current = null
      }
      autoScrollPointerRef.current = null
    }

    // Near a top/bottom edge, scroll the list toward it and keep the hover target
    // in sync with the new offset (the pointer can be still while the list moves).
    const tickAutoScroll = () => {
      const scrollEl = scrollRef.current
      const pointer = autoScrollPointerRef.current
      const drag = dragRef.current
      if (!scrollEl || !pointer || !drag?.active) {
        autoScrollFrameRef.current = null
        return
      }
      const rect = scrollEl.getBoundingClientRect()
      const delta = autoScrollDelta({
        pointerY: pointer.y,
        containerTop: rect.top,
        containerHeight: rect.height,
        edge: AUTO_SCROLL_EDGE_PX,
        step: AUTO_SCROLL_STEP_PX,
      })
      if (delta !== 0) {
        scrollEl.scrollTop += delta
        setHoverTarget(resolveTarget(pointer.x, pointer.y))
      }
      autoScrollFrameRef.current = requestAnimationFrame(tickAutoScroll)
    }

    const startAutoScroll = () => {
      if (autoScrollFrameRef.current === null) {
        autoScrollFrameRef.current = requestAnimationFrame(tickAutoScroll)
      }
    }

    const endDrag = () => {
      stopAutoScroll()
      dragRef.current = null
      ghostOriginRef.current = null
      document.documentElement.removeAttribute('data-row-dragging')
      setDraggingId(null)
      setHoverTarget(null)
    }

    const onPointerMove = (e: PointerEvent) => {
      const drag = dragRef.current
      if (!drag) return
      if (!drag.active) {
        if (Math.hypot(e.clientX - drag.startX, e.clientY - drag.startY) <= DRAG_THRESHOLD_PX)
          return
        drag.active = true
        // Seed the ghost's first paint before the render that mounts it.
        ghostOriginRef.current = { x: e.clientX, y: e.clientY }
        // The same grabbing cursor the workspace tree raises, from the same
        // attribute — a chat row in the air is a row in the air. See index.css.
        document.documentElement.setAttribute('data-row-dragging', '')
        setDraggingId(drag.id)
        startAutoScroll()
      }
      if (ghostRef.current) {
        ghostRef.current.style.left = `${e.clientX + CHIP_OFFSET_X}px`
        ghostRef.current.style.top = `${e.clientY + CHIP_OFFSET_Y}px`
      }
      autoScrollPointerRef.current = { x: e.clientX, y: e.clientY }
      setHoverTarget(resolveTarget(e.clientX, e.clientY))
    }

    const onPointerUp = (e: PointerEvent) => {
      const drag = dragRef.current
      if (!drag?.active) {
        endDrag()
        return
      }
      // The post-drop click must never double as a row selection.
      suppressNextClick()

      const target = resolveTarget(e.clientX, e.clientY)
      onDrop(target, drag.id)
      endDrag()
    }

    window.addEventListener('pointermove', onPointerMove)
    window.addEventListener('pointerup', onPointerUp)
    window.addEventListener('pointercancel', endDrag)
    return () => {
      stopAutoScroll()
      // Never leave the window stuck on the grabbing cursor with no drag left
      // to end and clear it.
      document.documentElement.removeAttribute('data-row-dragging')
      window.removeEventListener('pointermove', onPointerMove)
      window.removeEventListener('pointerup', onPointerUp)
      window.removeEventListener('pointercancel', endDrag)
    }
    // scrollRef is a stable ref (from useAgentChatListVirtualizer); listed only to
    // satisfy exhaustive-deps, it never causes the listeners to re-subscribe.
  }, [scrollRef])

  // Stable so AgentChatRow's memo holds: passed as a prop to every row, it must
  // not change identity on a parent render (adding a chat, a drag) or every row
  // would reconcile. It only writes a ref, so it never needs to change.
  const onPointerDownDrag = useCallback((chatId: string, e: React.PointerEvent) => {
    if (e.button !== 0) return
    dragRef.current = { id: chatId, startX: e.clientX, startY: e.clientY, active: false }
  }, [])

  return {
    draggingId,
    /** The slot a drop would land in right now — where the panel draws the
     *  insertion line. null while nothing is dragged, or while over the trash. */
    hoverSlot: hoverTarget?.kind === 'slot' ? hoverTarget.index : null,
    isOverTrash: hoverTarget?.kind === 'trash',
    ghostRef,
    ghostOriginRef,
    trashRef,
    onPointerDownDrag,
  }
}
