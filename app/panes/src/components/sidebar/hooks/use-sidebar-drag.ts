import { useCallback, useEffect, useRef, useState } from 'react'
import {
  cloneGhostRows,
  ghostTransform,
  grabOffsetFrom,
  type GhostRows,
  type GrabOffset,
} from '@/components/layout/drag-ghost'
import { createEdgeScroller, type EdgeScroller } from '@/components/layout/edge-scroll'
import { sameDrop, type DropMode } from '@/components/tree-dnd/drop-core'
import { clearDropHover } from '@/features/panes/stores/drag-store'
import {
  hitTest,
  paintDropLine,
  paintPaneHit,
  publishPaneZonePreview,
  rowDom,
  samePaneHit,
  setDropLineTracking,
  type DragRow,
  type Hit,
  type ResolvedPaneHit,
  type RowDragExtra,
  type SidebarPaneHit,
} from '@/components/sidebar/lib/sidebar-drag-hover'
import {
  commitRowDrop,
  dispatchDrop,
  type DropHandlers,
} from '@/components/sidebar/lib/sidebar-drop-dispatch'
import { createStripDrag, type StripDrag } from '@/components/sidebar/lib/tab-strip-drag'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

// One drag arm for SidebarTree and the Recents tabs. This owns the pointer (press, threshold,
// move, release, recovery); sidebar-drag-hover paints the drop, sidebar-drop-dispatch commits it.
// Refs and once-subscribed window listeners; React state changes only when the drop would.

// 8px, not 5: an ordinary click carries 5-7px of jitter, which armed a drag whose faded source
// and fixed-position ghost briefly composited into one doubled row. 8px clears jitter while an
// intentional drag still arms immediately.
export const SIDEBAR_DRAG_THRESHOLD_PX = 8

/** Stable empty, so a render that is not dragging hands out one identity. */
const NO_IDS: ReadonlySet<string> = new Set<string>()

/** One drag in flight — mutated in place, never replaced, so an edge scroll
 *  re-resolving the drop under a held-still pointer reads the live position. */
interface ActiveDrag {
  subjects: readonly SidebarRow[]
  pointer: { x: number; y: number }
}

/** Stable identity, so add/removeEventListener pair up across a drag. */
const preventDefault = (e: Event) => e.preventDefault()

/** Swallow the click a release fires, so a drop never also opens the row it lands on. */
function suppressNextClick(): void {
  const swallow = (e: MouseEvent) => {
    e.stopPropagation()
    e.preventDefault()
  }
  window.addEventListener('click', swallow, { capture: true, once: true })
  setTimeout(() => window.removeEventListener('click', swallow, { capture: true }), 0)
}

/** Whether the pointer was captured. A row deleted mid-press, or a pointerId the
 *  browser no longer has down, throws; the drag still starts, and the stale-buttons
 *  and blur recovery cover a release that leaves the webview. */
function tryCapturePointer(target: Element, pointerId: number): boolean {
  if (!target.isConnected) return false
  try {
    target.setPointerCapture(pointerId)
    return true
  } catch {
    return false
  }
}

export interface UseSidebarDragOptions extends DropHandlers {
  /** The tree/band's own scroll container — what an edge-held drag scrolls. */
  scrollRef: React.RefObject<HTMLElement | null>
  /** The strip of view tabs this drag may reorder within, when the pointer is in its band. */
  strip?: () => HTMLElement | null
}

export interface SidebarDrag {
  dragging: boolean
  /** The rows in flight — drawn at `opacity-40` where they still sit. */
  draggingIds: ReadonlySet<string>
  /** The row a drop would land INSIDE, which fills (ROW_NEST_TARGET). */
  nestTargetId: string | null
  /** The pane a drop is currently over, and which zone — null off any pane. */
  paneHit: SidebarPaneHit | null
  ghostRows: GhostRows | null
  ghostRef: React.RefObject<HTMLDivElement | null>
  ghostOrigin: { x: number; y: number } | null
  /** Callback ref for the hairline, which mounts one render after the drag begins. */
  attachDropLine: (el: HTMLDivElement | null) => void
  onPointerDownDrag: (row: SidebarRow, e: React.PointerEvent) => void
  /** The attributes a droppable row spreads as `dragProps`; `extra` carries tree-walk facts the
   *  row lacks (path/expanded/hasChildren) and a depth-0 leaf renderer may omit it. */
  dragProps: (row: SidebarRow, extra?: RowDragExtra) => Record<string, string | undefined>
}

export function useSidebarDrag(options: UseSidebarDragOptions): SidebarDrag {
  const [dragging, setDragging] = useState(false)
  const [draggingIds, setDraggingIds] = useState<ReadonlySet<string>>(NO_IDS)
  const [nestTargetId, setNestTargetId] = useState<string | null>(null)
  const [paneHit, setPaneHit] = useState<SidebarPaneHit | null>(null)
  const [ghostRows, setGhostRows] = useState<GhostRows | null>(null)

  // Everything below is a ref because it is read inside window listeners that
  // subscribe ONCE: a dependency here would re-subscribe them mid-drag, and a
  // closure over a render's props would go stale the first time the tree moves.
  const optionsRef = useRef(options)
  optionsRef.current = options

  const ghostRef = useRef<HTMLDivElement | null>(null)
  const ghostOriginRef = useRef<{ x: number; y: number } | null>(null)
  const dropLineRef = useRef<HTMLDivElement | null>(null)
  const lastHitRef = useRef<Hit>(null)
  const dropTargetRef = useRef<(DragRow & { mode: DropMode }) | null>(null)
  const paneHitRef = useRef<ResolvedPaneHit | null>(null)
  const edgeScrollerRef = useRef<EdgeScroller | null>(null)
  const scrollerXRef = useRef<{ left: number; right: number } | null>(null)
  // The scroller the wheel-lock was attached to, so endDrag removes it from the same element.
  const wheelLockScrollerRef = useRef<HTMLElement | null>(null)
  const stripRef = useRef<StripDrag | null>(null)
  const grabRef = useRef<GrabOffset>({ dx: 0, dy: 0 })
  const draggingRef = useRef<ActiveDrag | null>(null)
  const pendingRef = useRef<{
    row: SidebarRow
    startX: number
    startY: number
    target: HTMLElement
    pointerId: number
  } | null>(null)

  const attachDropLine = useCallback((el: HTMLDivElement | null) => {
    dropLineRef.current = el
    // Replayed: the line mounts one render after the drag begins, so the slot resolved at drag
    // start would otherwise go unmarked until the pointer moved again.
    paintDropLine(el, lastHitRef.current)
  }, [])

  const dragProps = useCallback(
    (row: SidebarRow, extra?: RowDragExtra): Record<string, string | undefined> =>
      rowDom.props({ ...row, ...extra }),
    [],
  )

  const onPointerDownDrag = useCallback((row: SidebarRow, e: React.PointerEvent) => {
    if (e.button !== 0) return
    if (draggingRef.current) return // ignore a second pointer mid-drag
    // No `row.working` refusal: §8.3 forbids REORDERING a working row, which the drop policy
    // enforces on row targets, while a pane drop only opens a view. Block text selection from
    // the PRESS — `selectstart` fires before the threshold promotes it into a drag.
    document.addEventListener('selectstart', preventDefault)
    // No pointer capture yet — capturing here swallows the dblclick that opens
    // the rename editor.
    pendingRef.current = {
      row,
      startX: e.clientX,
      startY: e.clientY,
      target: e.currentTarget as HTMLElement,
      pointerId: e.pointerId,
    }
  }, [])

  useEffect(() => {
    /** Publish a resolved drop only when it would draw something different — a drag's whole
     *  re-render budget. */
    function publish(hit: Hit): void {
      const row = hit?.kind === 'row' ? hit.row : null
      if (!sameDrop(row, dropTargetRef.current)) {
        dropTargetRef.current = row
        setNestTargetId(row && row.mode === 'into' ? row.id : null)
      }
      const pane = hit?.kind === 'pane' ? hit : null
      if (!samePaneHit(pane, paneHitRef.current)) {
        paintPaneHit(paneHitRef.current, pane)
        publishPaneZonePreview(pane)
        paneHitRef.current = pane
        // Public state carries only the logical value, never the internal `el`.
        setPaneHit(pane && { kind: pane.kind, paneId: pane.paneId, zone: pane.zone })
      }
      // The line is DOM, not state: it moves on every publish, including ones that change nothing
      // for React (an edge scroll re-resolving the same drop at a new rect).
      lastHitRef.current = hit
      paintDropLine(dropLineRef.current, hit)
    }

    function beginDrag(e: MouseEvent): void {
      const pending = pendingRef.current!
      pendingRef.current = null
      tryCapturePointer(pending.target, pending.pointerId)

      const subjects = optionsRef.current.subjectsFor(pending.row.id)
      const drag: ActiveDrag = { subjects, pointer: { x: e.clientX, y: e.clientY } }

      // Measure everything before the first style write; interleaving forces a layout per clone.
      // The pressed row uses `pending.target`, not `elementFor`: a live chat renders in the tree
      // and in Recents under one id, and the selector would always pick the tree's copy.
      const elements = subjects
        .map((s) => (s.id === pending.row.id ? pending.target : rowDom.elementFor(s)))
        .filter((el): el is HTMLElement => el !== null)
      const scroller = optionsRef.current.scrollRef.current
      const scrollerBox = scroller?.getBoundingClientRect()
      const grabbed = pending.target
      grabRef.current = grabOffsetFrom(
        grabbed.getBoundingClientRect(),
        pending.startX,
        pending.startY,
      )

      const stripEl = optionsRef.current.strip?.()
      stripRef.current = stripEl ? createStripDrag(stripEl, pending.target, grabRef.current) : null
      if (stripRef.current) grabRef.current = stripRef.current.grab
      const rows =
        stripRef.current?.ghostRows() ??
        cloneGhostRows(elements.length > 0 ? elements : [pending.target], subjects.length)

      // The strip scrolls itself sideways and takes the wheel; the vertical scroller is for the tree.
      if (scroller && scrollerBox && !stripRef.current) {
        scrollerXRef.current = { left: scrollerBox.left, right: scrollerBox.right }
        edgeScrollerRef.current = createEdgeScroller(
          scroller,
          { top: scrollerBox.top, height: scrollerBox.height },
          {
            onScrolled: () => {
              // The tree moved under a still pointer, so re-resolve within the scroller's own
              // frame — keeps the line honest without a scroll listener on every drag.
              publish(hitTest(drag.pointer.x, drag.pointer.y, drag.subjects))
            },
            onRunningChange: (running) => setDropLineTracking(dropLineRef.current, running),
          },
        )
        // The wheel can't move the list during a drag; only the edge scroller (which writes
        // scrollTop directly) may. Same stable `preventDefault` so add/remove pair up.
        scroller.addEventListener('wheel', preventDefault, { passive: false })
        wheelLockScrollerRef.current = scroller
      }

      draggingRef.current = drag
      ghostOriginRef.current = stripRef.current?.position(e.clientX, e.clientY) ?? {
        x: e.clientX - grabRef.current.dx,
        y: e.clientY - grabRef.current.dy,
      }
      document.documentElement.setAttribute('data-row-dragging', '')
      // The listener above stops a selection STARTING; this drops one the
      // press already made (a press landing on a text node can carry a caret
      // with it).
      window.getSelection()?.removeAllRanges()
      setGhostRows(rows)
      setDraggingIds(new Set(subjects.map((s) => s.id)))
      setDragging(true)
      publish(hitTest(e.clientX, e.clientY, subjects))
    }

    function onPointerMove(e: MouseEvent): void {
      // A release outside the webview never reaches window as pointerup; this is the next event
      // window gets, with `buttons` already 0. Treat it as the release, or `selectstart` stays
      // blocked document-wide until reload.
      if ((pendingRef.current || draggingRef.current) && e.buttons === 0) {
        pendingRef.current = null
        if (draggingRef.current) endDrag()
        else document.removeEventListener('selectstart', preventDefault)
        return
      }
      if (pendingRef.current) {
        const { startX, startY } = pendingRef.current
        if (Math.hypot(e.clientX - startX, e.clientY - startY) > SIDEBAR_DRAG_THRESHOLD_PX)
          beginDrag(e)
        return
      }
      const drag = draggingRef.current
      if (!drag) return
      drag.pointer.x = e.clientX
      drag.pointer.y = e.clientY

      // READS before WRITES. The hit test forces layout; writing the ghost's
      // position first would make every move a read-after-write reflow.
      const strip = stripRef.current
      const hit = strip?.inBand(e.clientX, e.clientY)
        ? null
        : hitTest(e.clientX, e.clientY, drag.subjects)

      // Edge scroll only while the pointer is over the tree/band's own column: the band depends
      // on Y alone, so a row carried sideways onto a pane would keep the list scrolling.
      const span = scrollerXRef.current
      if (span !== null && e.clientX >= span.left && e.clientX <= span.right) {
        edgeScrollerRef.current?.update(e.clientY)
      } else edgeScrollerRef.current?.stop()

      if (strip) strip.move(e.clientX, e.clientY, ghostRef.current)
      else if (ghostRef.current) {
        // One composited transform, not two layout-triggering offsets.
        ghostRef.current.style.transform = ghostTransform(
          e.clientX - grabRef.current.dx,
          e.clientY - grabRef.current.dy,
        )
      }

      publish(hit)
    }

    function endDrag(): void {
      stripRef.current?.dispose()
      stripRef.current = null
      edgeScrollerRef.current?.stop()
      edgeScrollerRef.current = null
      scrollerXRef.current = null
      wheelLockScrollerRef.current?.removeEventListener('wheel', preventDefault)
      wheelLockScrollerRef.current = null
      grabRef.current = { dx: 0, dy: 0 }
      document.documentElement.removeAttribute('data-row-dragging')
      document.removeEventListener('selectstart', preventDefault)
      pendingRef.current = null
      draggingRef.current = null
      dropTargetRef.current = null
      paintPaneHit(paneHitRef.current, null)
      // Takes the zone preview down with it.
      clearDropHover()
      paneHitRef.current = null
      lastHitRef.current = null
      ghostOriginRef.current = null
      setGhostRows(null)
      setDraggingIds(NO_IDS)
      setNestTargetId(null)
      setPaneHit(null)
      setDragging(false)
    }

    function onPointerUp(e: MouseEvent): void {
      pendingRef.current = null
      // Unconditionally: the listener is armed on every press, including the
      // ones that turn out to be plain clicks and never reach beginDrag.
      document.removeEventListener('selectstart', preventDefault)
      const drag = draggingRef.current
      if (!drag) return

      suppressNextClick()
      const strip = stripRef.current
      if (strip?.inBand(e.clientX, e.clientY)) {
        // The strip already decided where the tab goes; the pointer's own row is not the answer.
        const beside = strip.drop(e.clientX)
        const row =
          beside && rowDom.read(beside.el.querySelector('[data-sidebar-recents-row]') ?? beside.el)
        if (row) commitRowDrop(drag.subjects, row.id, beside.mode, true, optionsRef.current)
        strip.settle(e.clientX)
        endDrag()
        return
      }
      dispatchDrop(drag.subjects, e.clientX, e.clientY, optionsRef.current)
      endDrag()
    }

    // The window losing focus mid-press/mid-drag (a native dialog, Cmd+Tab)
    // with no pointermove to follow: nothing else would ever clear the
    // pending/dragging state or the document-wide `selectstart` block.
    function onWindowBlur(): void {
      if (draggingRef.current) endDrag()
      else if (pendingRef.current) {
        pendingRef.current = null
        document.removeEventListener('selectstart', preventDefault)
      }
    }

    window.addEventListener('pointermove', onPointerMove)
    window.addEventListener('pointerup', onPointerUp)
    window.addEventListener('pointercancel', endDrag)
    window.addEventListener('blur', onWindowBlur)
    return () => {
      edgeScrollerRef.current?.stop()
      stripRef.current?.dispose()
      stripRef.current = null
      // A teardown mid-drag would otherwise leave the document marked and the
      // selection blocked, with no drag left to end and clear them.
      document.documentElement.removeAttribute('data-row-dragging')
      document.removeEventListener('selectstart', preventDefault)
      wheelLockScrollerRef.current?.removeEventListener('wheel', preventDefault)
      wheelLockScrollerRef.current = null
      paintPaneHit(paneHitRef.current, null)
      clearDropHover()
      window.removeEventListener('pointermove', onPointerMove)
      window.removeEventListener('pointerup', onPointerUp)
      window.removeEventListener('pointercancel', endDrag)
      window.removeEventListener('blur', onWindowBlur)
    }
  }, [])

  return {
    dragging,
    draggingIds,
    nestTargetId,
    paneHit,
    ghostRows,
    ghostRef,
    ghostOrigin: ghostOriginRef.current,
    attachDropLine,
    onPointerDownDrag,
    dragProps,
  }
}
