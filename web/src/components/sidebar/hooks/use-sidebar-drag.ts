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
import { dispatchDrop, type DropHandlers } from '@/components/sidebar/lib/sidebar-drop-dispatch'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

/**
 * One drag arm for the unified sidebar: `SidebarTree` and `RecentsBand` both bind
 * through it. This owns the pointer (press, threshold, move, release, recovery);
 * `sidebar-drag-hover` resolves and paints what a position would drop on, and
 * `sidebar-drop-dispatch` commits a release. Refs only, window listeners subscribed
 * once, and React state written only when the drop would draw something different.
 */

/**
 * 8, not the predecessors' 5 — live-reported and reproduced: an ordinary
 * click routinely carries 5-7px of incidental pointer movement between down
 * and up (hand tremor, a trackpad click's own slight drag), which crossed 5
 * and armed a real drag on a plain click. That drag's own faded source row
 * (opacity-40, still in its list slot) and its full-opacity ghost (floating
 * via `position: fixed`, nearly on top of that same slot for a small
 * movement) briefly composited into what looked like one taller, doubled-up
 * row — reported as "active rows grow in height" — self-corrected the
 * instant `endDrag` ran on pointerup, which is why it only ever showed up
 * for the length of a click and never in a settled `getBoundingClientRect`
 * read. 8px is comfortably past ordinary click jitter while still short
 * enough that an intentional drag arms immediately.
 */
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

/**
 * Swallow the click a release fires on whatever row it lands on — a drop is
 * never also a click that opens the row it happened to land on.
 */
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
  /** The attributes a droppable row spreads onto its own `SidebarRow`
   *  (`SidebarRowProps.dragProps`) — `extra` carries the ancestor facts the
   *  caller's own tree-walk already knows and `SidebarRow` itself does not
   *  (path/expanded/hasChildren); a depth-0 leaf renderer may omit it. */
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
  // The scroller a wheel-lock was attached to, so `endDrag` removes the SAME
  // listener from the SAME element rather than re-reading `scrollRef.current`
  // (which could theoretically have moved on since drag start).
  const wheelLockScrollerRef = useRef<HTMLElement | null>(null)
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
    // Replayed rather than left to the next move: the line mounts one render
    // AFTER the drag begins, so the slot resolved at drag start would
    // otherwise go unmarked until the pointer moved again.
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
    // No `row.working` refusal here any more (live-reported: it blocked
    // picking a working chat up AT ALL, split included). §8.3's "may not be
    // dragged" is about REORDERING — moving a row re-points the ground
    // under it, mirroring the backend's own `guardNotWorking` — and that is
    // still enforced where a reorder is actually decided:
    // `SIDEBAR_DROP_POLICY.allowedModes` refuses every mode for a working
    // subject over a ROW target. A drop onto a PANE never touches tree
    // placement (`performSidebarPaneDrop`/`openChatIntoPane` just open a
    // split view), so a working chat has no less claim to that than an idle one.
    // Block the text selection from the PRESS: `selectstart` fires before the
    // threshold promotes the press into a drag, so arming this at drag start
    // is arming it after the only event it could have cancelled.
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
    /**
     * Publish a resolved drop, but only when it would draw something
     * different. This is the whole re-render budget of a drag.
     */
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
        // The public state carries only the logical value — never `el`,
        // which is internal plumbing for `paintPaneHit` alone (see
        // `ResolvedPaneHit`).
        setPaneHit(pane && { kind: pane.kind, paneId: pane.paneId, zone: pane.zone })
      }
      // The line is DOM, not state: it moves on every publish, including the
      // ones that changed nothing for React (an edge scroll re-running the
      // hit test under a held-still pointer resolves the same drop at a new
      // rect).
      lastHitRef.current = hit
      paintDropLine(dropLineRef.current, hit)
    }

    function beginDrag(e: MouseEvent): void {
      const pending = pendingRef.current!
      pendingRef.current = null
      tryCapturePointer(pending.target, pending.pointerId)

      const subjects = optionsRef.current.subjectsFor(pending.row.id)
      const drag: ActiveDrag = { subjects, pointer: { x: e.clientX, y: e.clientY } }

      // Every measurement this drag needs, taken together and before the
      // first style write: interleaving reads and writes forces a layout per
      // clone on the one frame of a drag that is already doing the most work.
      //
      // The pressed row (`pending.row`) resolves to `pending.target` directly
      // rather than through `rowDom.elementFor` — a chat with a live view
      // renders through TWO `SidebarRow` instances sharing one id (its own
      // tree row, and Recents' mirror of it — inlineRenameDisabled's own doc
      // in sidebar-row.tsx), and `elementFor`'s `[attr="id"] `querySelector
      // has no way to prefer one over the other: it always returns whichever
      // comes first in DOM order, which is the TREE's copy (it renders above
      // Recents). Dragging the Recents row then cloned and measured the
      // TREE's instance instead — same id, different styling (`ROW_HAS_VIEW_IDLE`'s
      // muted ground, not Recents' own ROW_ACTIVE) — live-verified as the
      // cause of "when dragging that active row, the background is not
      // active." `pending.target` is `e.currentTarget` from the exact
      // `pointerdown` that started this drag (armed in `onPointerDownDrag`
      // below), so it is never ambiguous. Every OTHER co-dragged subject in a
      // multi-select still resolves through `elementFor` — this exact
      // ambiguity is specific to the one row the pointer is actually on.
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

      const rows = cloneGhostRows(
        elements.length > 0 ? elements : [pending.target],
        subjects.length,
      )

      if (scroller && scrollerBox) {
        scrollerXRef.current = { left: scrollerBox.left, right: scrollerBox.right }
        edgeScrollerRef.current = createEdgeScroller(
          scroller,
          { top: scrollerBox.top, height: scrollerBox.height },
          {
            onScrolled: () => {
              // The tree moved under the pointer, so the row beneath it did
              // too. Re-resolving inside the scroller's own frame is what
              // keeps a viewport-positioned line honest with no scroll
              // listener firing through every ordinary drag.
              publish(hitTest(drag.pointer.x, drag.pointer.y, drag.subjects))
            },
            onRunningChange: (running) => setDropLineTracking(dropLineRef.current, running),
          },
        )
        // The list stays put under the wheel for as long as this drag is
        // live — only a pointer held near the top/bottom edge (the edge
        // scroller above, which writes scrollTop directly) may move it.
        // Reuses the same stable `preventDefault` identity `selectstart`
        // already does, so add/removeEventListener pair up the same way.
        scroller.addEventListener('wheel', preventDefault, { passive: false })
        wheelLockScrollerRef.current = scroller
      }

      draggingRef.current = drag
      ghostOriginRef.current = {
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
      // A release delivered outside the webview (the pointer left the window
      // before crossing SIDEBAR_DRAG_THRESHOLD_PX / before setPointerCapture)
      // never reaches window as pointerup — this is the next event window
      // DOES get, and `buttons` already reads 0, so treat it as the missed
      // release rather than leave `selectstart` blocked document-wide until
      // reload (reported live: "text selection doesn't work anywhere").
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
      const hit = hitTest(e.clientX, e.clientY, drag.subjects)

      // Edge scroll only while the pointer is still over the tree/band's own
      // column. The band is a function of Y alone, so a row carried sideways
      // onto the editor pane would otherwise keep the list running out from
      // under a drag that is trying to hold still over a pane zone.
      const span = scrollerXRef.current
      if (span !== null && e.clientX >= span.left && e.clientX <= span.right) {
        edgeScrollerRef.current?.update(e.clientY)
      } else edgeScrollerRef.current?.stop()

      if (ghostRef.current) {
        // One composited transform, not two layout-triggering offsets.
        ghostRef.current.style.transform = ghostTransform(
          e.clientX - grabRef.current.dx,
          e.clientY - grabRef.current.dy,
        )
      }

      publish(hit)
    }

    function endDrag(): void {
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
