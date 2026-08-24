import { useCallback, useEffect, useRef, useState } from 'react'
import {
  cloneGhostRows,
  ghostTransform,
  grabOffsetFrom,
  type GhostRows,
  type GrabOffset,
} from '@/components/layout/drag-ghost'
import { dropLineBox } from '@/components/layout/drop-indicator'
import { createEdgeScroller, type EdgeScroller } from '@/components/layout/edge-scroll'
import {
  PANE_ARM_MS,
  paintRemovalOverlay,
  type RemovalOverlayText,
} from '@/components/layout/editor-removal-overlay'
import { dragSubjectsFor, sameDrop } from '@/components/tree-dnd/drop-core'
import {
  chatRowElementFor,
  findChatDrop,
  type ChatDragSubject,
  type ChatDropHit,
  type ResolvedChatDrop,
} from '@/features/agent/tree/lib/chat-drop'

/**
 * Dragging rows in the Chats tree.
 *
 * The same gesture as the sidebar's, wired from the same parts — the shared hit
 * test, the shared ghost, the shared hairline, the shared edge scroller, and now
 * the shared REMOVAL TARGET: the editor pane, with its veil and its arming
 * dwell. What is here is the wiring only: what a pointer press arms, when a
 * press becomes a drag, and which of those parts a given frame has to touch.
 *
 * The budget is the whole design. A pointermove does ONE hit test (one
 * `elementsFromPoint`, one attribute sweep, one rect) and then writes a
 * transform and, at most, a few pixel coordinates on the hairline. React state
 * is written only when the resolved drop would DRAW something different, so
 * moving inside one row's band re-renders nothing at all — and the ghost's
 * position never goes through React, because a list re-render per pointer pixel
 * is the thing that made the old drag cost 60ms a frame. The pane's veil is on
 * the same budget: it is painted straight into the DOM, so a drag over the
 * editor never re-renders the panel either.
 */

/** How far the pointer must travel before a press becomes a drag. */
export const CHAT_DRAG_THRESHOLD_PX = 5

/**
 * How long a drag rests over a folded row's middle before it opens.
 *
 * Long enough that crossing one on the way somewhere else does not disturb it,
 * short enough that stopping over it reads as asking. The row opens; it does not
 * stay open by itself, because the drop that follows is what says whether the
 * user meant to go in.
 */
export const CHAT_SPRING_OPEN_MS = 450

/** Stable empty, so a render that is not dragging hands out one identity. */
const NO_IDS: ReadonlySet<string> = new Set<string>()

/**
 * One drag in flight.
 *
 * The pointer lives HERE, in an object the drag owns and mutates, rather than in
 * a ref of its own. An edge scroll has to re-resolve the drop under a hand that
 * is holding still, and reading a separate ref meant a null check on a value
 * that cannot be null while a drag exists — a branch no test could ever reach,
 * standing in for the invariant this type states outright.
 */
interface ActiveDrag {
  subjects: readonly ChatDragSubject[]
  pointer: { x: number; y: number }
}

/**
 * Where the pane's removal zone is in its dwell.
 *
 * `waiting` is the beat between arriving over the pane and the removal becoming
 * real; only `armed` lets a release delete anything.
 */
interface PaneState {
  state: 'off' | 'waiting' | 'armed'
  timer: number | null
}

/** Stable identity, so add/removeEventListener pair up across a drag. */
const preventDefault = (e: Event) => e.preventDefault()

/**
 * Swallow the click the browser fires after a drop.
 *
 * A release over a row is a drop, never also a click on the row it happened to
 * land on — without this, dropping a chat onto another chat also OPENS it.
 */
function suppressNextClick(): void {
  const swallow = (e: MouseEvent) => {
    e.stopPropagation()
    e.preventDefault()
  }
  window.addEventListener('click', swallow, { capture: true, once: true })
  setTimeout(() => window.removeEventListener('click', swallow, { capture: true }), 0)
}

/**
 * Move the hairline onto the slot `hit` names, or take it away.
 *
 * Positioned from the rect the hit test already measured — the line never
 * measures anything itself — and written only where a coordinate actually
 * differs, so holding the pointer still inside one band costs nothing.
 *
 * A nest is the OTHER mark: an `into` hit hides the line, and the row itself
 * fills instead. Never both — a line and a fill at once leave the user guessing
 * which of two genuinely different moves is about to happen, and one of them
 * rewrites what a conversation reads.
 */
function paintDropLine(el: HTMLDivElement | null, hit: ChatDropHit | null): void {
  if (!el) return
  if (hit?.kind !== 'row' || hit.row.mode === 'into') {
    el.style.display = 'none'
    return
  }
  const box = dropLineBox(hit.rect, hit.row.mode)
  el.style.display = 'block'
  if (el.dataset.dropIndicator !== hit.row.mode) el.dataset.dropIndicator = hit.row.mode
  const left = `${box.left}px`
  const top = `${box.top}px`
  const width = `${box.width}px`
  if (el.style.left !== left) el.style.left = left
  if (el.style.top !== top) el.style.top = top
  if (el.style.width !== width) el.style.width = width
}

/** An edge scroll invalidates the line's slot every frame; a 60ms ease between
 *  those slots is 60ms of the line trailing the rows it marks. */
function setDropLineTracking(el: HTMLDivElement | null, tracking: boolean): void {
  if (!el) return
  el.style.transition = tracking ? 'none' : ''
}

export interface AgentChatsDragOptions {
  /** The panel's scroll container — what an edge-held drag scrolls. */
  scrollRef: React.RefObject<HTMLDivElement | null>
  /** The rows a drag starting on `grabbed` carries, in tree order. */
  subjectsFor: (grabbed: ChatDragSubject) => ChatDragSubject[]
  /** Commit a drop onto a row. */
  onDrop: (subjects: readonly ChatDragSubject[], target: ResolvedChatDrop) => void
  /** Hold these rows for removal — a release over an ARMED editor pane. */
  onPaneRemove: (subjects: readonly ChatDragSubject[]) => void
  /**
   * What the pane's veil should say about `subjects`, or null when this drag has
   * nothing to remove — in which case the veil stays down and the pane never
   * arms, because offering a target that refuses on release is worse than
   * offering none.
   */
  removalText: (subjects: readonly ChatDragSubject[], armed: boolean) => RemovalOverlayText | null
  /** Open a folded row the drag has rested inside. */
  onSpringOpen: (id: string) => void
}

export interface AgentChatsDrag {
  /** The rows in flight — drawn at `opacity-40` where they still sit. */
  draggingIds: ReadonlySet<string>
  /** The row a drop would land INSIDE, which fills. */
  nestTargetId: string | null
  dragging: boolean
  ghostRows: GhostRows | null
  ghostRef: React.RefObject<HTMLDivElement | null>
  /** The ghost's top-left for its FIRST paint; every frame after is a transform. */
  ghostOrigin: { x: number; y: number } | null
  /** Callback ref for the hairline, which mounts one render after the drag begins. */
  attachDropLine: (el: HTMLDivElement | null) => void
  onPointerDownDrag: (subject: ChatDragSubject, e: React.PointerEvent) => void
}

export function useAgentChatsDrag(options: AgentChatsDragOptions): AgentChatsDrag {
  const [dragging, setDragging] = useState(false)
  const [draggingIds, setDraggingIds] = useState<ReadonlySet<string>>(NO_IDS)
  const [nestTargetId, setNestTargetId] = useState<string | null>(null)
  const [ghostRows, setGhostRows] = useState<GhostRows | null>(null)

  // Everything below is a ref because it is read inside window listeners that
  // subscribe ONCE: a dependency here would re-subscribe them mid-drag, and a
  // closure over a render's props would go stale the first time the list moves.
  const optionsRef = useRef(options)
  optionsRef.current = options

  const ghostRef = useRef<HTMLDivElement | null>(null)
  const ghostOriginRef = useRef<{ x: number; y: number } | null>(null)
  const dropLineRef = useRef<HTMLDivElement | null>(null)
  const lastHitRef = useRef<ChatDropHit | null>(null)
  const dropTargetRef = useRef<ResolvedChatDrop | null>(null)
  const edgeScrollerRef = useRef<EdgeScroller | null>(null)
  const scrollerXRef = useRef<{ left: number; right: number } | null>(null)
  const grabRef = useRef<GrabOffset>({ dx: 0, dy: 0 })
  const springRef = useRef<{ id: string; timer: number } | null>(null)
  const paneRef = useRef<PaneState>({ state: 'off', timer: null })
  const draggingRef = useRef<ActiveDrag | null>(null)
  const pendingRef = useRef<{
    subject: ChatDragSubject
    startX: number
    startY: number
    target: HTMLElement
    pointerId: number
  } | null>(null)

  const attachDropLine = useCallback((el: HTMLDivElement | null) => {
    dropLineRef.current = el
    // Replayed rather than left to the next move: the line mounts one render
    // AFTER the drag begins, so the slot resolved at drag start would otherwise
    // go unmarked until the pointer moved again.
    paintDropLine(el, lastHitRef.current)
  }, [])

  const onPointerDownDrag = useCallback((subject: ChatDragSubject, e: React.PointerEvent) => {
    if (e.button !== 0) return
    if (draggingRef.current) return // ignore a second pointer mid-drag
    // Block the text selection from the PRESS: `selectstart` fires before the
    // threshold promotes the press into a drag, so arming this at drag start is
    // arming it after the only event it could have cancelled.
    document.addEventListener('selectstart', preventDefault)
    // No pointer capture yet — capturing here swallows the dblclick that opens
    // the rename editor.
    pendingRef.current = {
      subject,
      startX: e.clientX,
      startY: e.clientY,
      target: e.currentTarget as HTMLElement,
      pointerId: e.pointerId,
    }
  }, [])

  useEffect(() => {
    /** Let go of a pending spring-open, whether or not one is running. */
    function cancelSpring(): void {
      if (springRef.current === null) return
      clearTimeout(springRef.current.timer)
      springRef.current = null
    }

    /**
     * Arm, keep or drop the spring-open for the row under the pointer.
     *
     * Only a folded row with something inside it can spring, and only while the
     * pointer is asking to go INSIDE it — resting on a row's edge is asking to
     * land beside it, and opening the row there would move the target out from
     * under the hand mid-gesture.
     */
    function trackSpring(hit: ChatDropHit | null): void {
      const row = hit?.kind === 'row' ? hit.row : null
      if (!row || row.mode !== 'into' || row.expanded || !row.hasChildren) {
        cancelSpring()
        return
      }
      if (springRef.current?.id === row.id) return
      cancelSpring()
      const id = row.id
      springRef.current = {
        id,
        // No "still dragging?" guard: endDrag cancels this timer, so it fires
        // only while a drag is in flight — and a guard that can never be false
        // is a guard nobody can test.
        timer: window.setTimeout(() => {
          springRef.current = null
          optionsRef.current.onSpringOpen(id)
        }, CHAT_SPRING_OPEN_MS),
      }
    }

    /**
     * Draw the pane's removal zone in one of its two states, and say whether
     * there was anything to draw.
     *
     * Returns false when this drag can remove nothing — every row it is carrying
     * has left the store while it was in the air — in which case the veil stays
     * down and the pane never arms. Better to offer no target than one that
     * refuses on release.
     */
    function paintRemovalTarget(subjects: readonly ChatDragSubject[], armed: boolean): boolean {
      // The SAME plan the drop itself commits, asked first. The veil is a
      // promise about what a release will do, so deriving it from a second rule
      // is how it ends up offering a removal the drop then refuses.
      const text = optionsRef.current.removalText(subjects, armed)
      if (!text) {
        paintRemovalOverlay(null)
        return false
      }
      paintRemovalOverlay(text)
      return true
    }

    /**
     * Follow the pointer in and out of the editor pane, arming the removal once
     * it has stayed a beat.
     *
     * The dwell is the whole point: a long reorder crosses the pane on its way
     * between two ends of the list, and a pane that removed on release would
     * make that transit a loaded gun. Nothing can be DROPPED until it elapses.
     *
     * What the dwell does NOT gate is whether the zone is drawn at all. The veil
     * goes up when the drag starts and stays up for its whole length; this only
     * steps it between `available` and `armed`.
     */
    function trackPane(over: boolean, subjects: readonly ChatDragSubject[]): void {
      const pane = paneRef.current
      if (!over) {
        if (pane.state === 'off') return
        if (pane.timer !== null) clearTimeout(pane.timer)
        paneRef.current = { state: 'off', timer: null }
        // Back to available, NOT away: the row is still in the air and the pane
        // is still where it would go.
        paintRemovalTarget(subjects, false)
        return
      }
      if (pane.state !== 'off') return
      const timer = window.setTimeout(() => {
        // No "still dragging?" guard: endDrag clears this timer, so it fires
        // only while a drag is in flight — and a guard that can never be false
        // is a guard nobody can test.
        if (!paintRemovalTarget(subjects, true)) {
          paneRef.current = { state: 'off', timer: null }
          return
        }
        paneRef.current = { state: 'armed', timer: null }
      }, PANE_ARM_MS)
      paneRef.current = { state: 'waiting', timer }
    }

    /**
     * Publish a resolved drop, but only when it would draw something different.
     *
     * This is the whole re-render budget of a drag.
     */
    function publish(hit: ChatDropHit | null, subjects: readonly ChatDragSubject[]): void {
      trackPane(hit?.kind === 'pane', subjects)
      const row = hit?.kind === 'row' ? hit.row : null
      if (!sameDrop(row, dropTargetRef.current)) {
        dropTargetRef.current = row
        setNestTargetId(row && row.mode === 'into' ? row.id : null)
      }
      trackSpring(hit)
      // The line is DOM, not state: it moves on every publish, including the
      // ones that changed nothing for React (an edge scroll re-running the hit
      // test under a held-still pointer resolves the same drop at a new rect).
      lastHitRef.current = hit
      paintDropLine(dropLineRef.current, hit)
    }

    function beginDrag(e: MouseEvent): void {
      const pending = pendingRef.current!
      pendingRef.current = null
      // Only if the row is still in the tree: a chat deleted on the wire between
      // the press and the threshold takes its element with it, and capturing a
      // pointer on a detached node throws.
      if (pending.target.isConnected) pending.target.setPointerCapture(pending.pointerId)

      const subjects = dragSubjectsFor(
        pending.subject,
        optionsRef.current.subjectsFor(pending.subject),
      )
      const drag: ActiveDrag = { subjects, pointer: { x: e.clientX, y: e.clientY } }

      // Every measurement this drag needs, taken together and before the first
      // style write: interleaving reads and writes forces a layout per clone on
      // the one frame of a drag that is already doing the most work.
      const elements = subjects
        .map((s) => chatRowElementFor(s))
        .filter((el): el is HTMLElement => el !== null)
      const scroller = optionsRef.current.scrollRef.current
      const scrollerBox = scroller?.getBoundingClientRect()
      const grabbed = chatRowElementFor(pending.subject) ?? pending.target
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
              // The list moved under the pointer, so the row beneath it did too.
              // Re-resolving inside the scroller's own frame is what keeps a
              // viewport-positioned line honest with no scroll listener firing
              // through every ordinary drag.
              //
              // The drag is CAPTURED, not read back off the ref: this closure
              // and the drag it belongs to begin and end together, so there is
              // no state here to check for.
              publish(findChatDrop(drag.pointer.x, drag.pointer.y, drag.subjects), drag.subjects)
            },
            onRunningChange: (running) => setDropLineTracking(dropLineRef.current, running),
          },
        )
      }

      draggingRef.current = drag
      ghostOriginRef.current = {
        x: e.clientX - grabRef.current.dx,
        y: e.clientY - grabRef.current.dy,
      }
      document.documentElement.setAttribute('data-row-dragging', '')
      // The listener above stops a selection STARTING; this drops one the press
      // already made (a press landing on a text node can carry a caret with it).
      window.getSelection()?.removeAllRanges()
      setGhostRows(rows)
      setDraggingIds(new Set(subjects.map((s) => s.id)))
      setDragging(true)
      // The removal zone, up front — an affordance you have to guess at is not
      // one. `publish` below may immediately arm it if the drag started with the
      // pointer already over the pane.
      paintRemovalTarget(subjects, false)
      publish(findChatDrop(e.clientX, e.clientY, subjects), subjects)
    }

    function onPointerMove(e: MouseEvent): void {
      if (pendingRef.current) {
        const { startX, startY } = pendingRef.current
        if (Math.hypot(e.clientX - startX, e.clientY - startY) > CHAT_DRAG_THRESHOLD_PX)
          beginDrag(e)
        return
      }
      const drag = draggingRef.current
      if (!drag) return
      drag.pointer.x = e.clientX
      drag.pointer.y = e.clientY

      // READS before WRITES. The hit test forces layout; writing the ghost's
      // position first would make every move a read-after-write reflow.
      const hit = findChatDrop(e.clientX, e.clientY, drag.subjects)

      // Edge scroll only while the pointer is still over the panel. The band is
      // a function of Y alone, so a row carried sideways onto the editor pane —
      // which is how it is removed, and which asks the hand to hold still for
      // the arming dwell — would otherwise keep the list running out from under
      // the drag while the user is trying to hold still over something else.
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

      publish(hit, drag.subjects)
    }

    function endDrag(): void {
      edgeScrollerRef.current?.stop()
      edgeScrollerRef.current = null
      scrollerXRef.current = null
      cancelSpring()
      grabRef.current = { dx: 0, dy: 0 }
      document.documentElement.removeAttribute('data-row-dragging')
      document.removeEventListener('selectstart', preventDefault)
      draggingRef.current = null
      dropTargetRef.current = null
      lastHitRef.current = null
      ghostOriginRef.current = null
      if (paneRef.current.timer !== null) clearTimeout(paneRef.current.timer)
      paneRef.current = { state: 'off', timer: null }
      // Explicitly, and after the drag has been cleared: leaving the pane
      // normally drops the veil back to `available` rather than taking it away,
      // so relying on trackPane would leave the zone drawn over the editor after
      // every drag that ended anywhere but the pane.
      paintRemovalOverlay(null)
      setGhostRows(null)
      setDraggingIds(NO_IDS)
      setNestTargetId(null)
      setDragging(false)
    }

    function onPointerUp(e: MouseEvent): void {
      pendingRef.current = null
      // Unconditionally: the listener is armed on every press, including the
      // ones that turn out to be plain clicks and never reach endDrag.
      document.removeEventListener('selectstart', preventDefault)
      const drag = draggingRef.current
      if (!drag) return

      suppressNextClick()
      const hit = findChatDrop(e.clientX, e.clientY, drag.subjects)
      // Only an ARMED pane removes. A release over one that is still counting
      // out its dwell is a drag that ended in the wrong place, not a delete.
      if (hit?.kind === 'pane') {
        if (paneRef.current.state === 'armed') optionsRef.current.onPaneRemove(drag.subjects)
      } else if (hit?.kind === 'row') optionsRef.current.onDrop(drag.subjects, hit.row)
      endDrag()
    }

    window.addEventListener('pointermove', onPointerMove)
    window.addEventListener('pointerup', onPointerUp)
    window.addEventListener('pointercancel', endDrag)
    return () => {
      edgeScrollerRef.current?.stop()
      cancelSpring()
      if (paneRef.current.timer !== null) clearTimeout(paneRef.current.timer)
      paneRef.current = { state: 'off', timer: null }
      paintRemovalOverlay(null)
      // A teardown mid-drag would otherwise leave the document marked and the
      // selection blocked, with no drag left to end and clear them.
      document.documentElement.removeAttribute('data-row-dragging')
      document.removeEventListener('selectstart', preventDefault)
      window.removeEventListener('pointermove', onPointerMove)
      window.removeEventListener('pointerup', onPointerUp)
      window.removeEventListener('pointercancel', endDrag)
    }
  }, [])

  return {
    draggingIds,
    nestTargetId,
    dragging,
    ghostRows,
    ghostRef,
    ghostOrigin: ghostOriginRef.current,
    attachDropLine,
    onPointerDownDrag,
  }
}
