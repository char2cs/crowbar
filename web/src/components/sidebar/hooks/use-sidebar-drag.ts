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
  anyModeAllowed,
  sameDrop,
  NO_MODES,
  type DropMode,
  type DropPolicy,
} from '@/components/tree-dnd/drop-core'
import {
  createDropHitTest,
  createDropRowDom,
  type DropRowSpec,
  type DropZone,
} from '@/components/tree-dnd/drop-dom'
import { getPaneDropZoneFromRect, type PaneDropZone } from '@/features/panes/utils/pane-drop-zones'
import { SIDEBAR_DROP_POLICY } from '@/components/sidebar/lib/sidebar-drop-policy'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

/**
 * One drag arm for the whole unified sidebar — `SidebarTree` and `RecentsBand`
 * both bind through this instead of each carrying its own copy (the two files
 * this replaces: `workspace-tree-context.tsx`'s drag half and
 * `use-agent-chats-drag.ts`). `SIDEBAR_DROP_POLICY` (Task 20) is the one
 * matrix; `tree-dnd/drop-dom.ts`'s `createDropRowDom`/`createDropHitTest`
 * (confirmed generic, both old trees already bound through this factory) are
 * bound to `SidebarRow` once, here, instead of twice.
 *
 * The budget is the same one both predecessors were built to: refs-only
 * inside one `useEffect`, window-level `pointermove`/`pointerup`/
 * `pointercancel` subscribed ONCE, direct `style.transform` writes for the
 * ghost and the hairline, and React state written only when the resolved
 * drop would draw something DIFFERENT — moving inside one row's band
 * re-renders nothing.
 */

/** Matches both predecessor hooks' confirmed value. */
export const SIDEBAR_DRAG_THRESHOLD_PX = 5

/** Facts a row's own tree-walk knows that `SidebarRow` itself does not carry —
 *  handed to {@link SidebarDrag.dragProps} alongside the row so the hit test
 *  can resolve first-child re-parents and the subtree cycle guard below. */
export interface RowDragExtra {
  /** This row's ancestor chain, delimited on both sides: `/root/a/`. Read back
   *  at hit-test time for the cycle guard — never trust `undefined` as "root",
   *  since `chain.includes` on an empty string is simply always false, which
   *  is the correct (permissive) answer for a row with no known ancestry
   *  (Recents entries, which render at depth 0 with no tree context). */
  path?: string
  expanded?: boolean
  hasChildren?: boolean
}

type DragRow = SidebarRow & RowDragExtra

/** One attribute per row kind, so a hit test can tell a row apart from
 *  anything else under the pointer with one `getAttribute` sweep. */
const SIDEBAR_DRAG_ROW_SPEC: DropRowSpec = {
  kinds: {
    branch: 'data-sidebar-branch-drop',
    folder: 'data-sidebar-folder-drop',
    chat: 'data-sidebar-chat-drop',
    workflow: 'data-sidebar-workflow-drop',
  },
  parentAttr: 'data-sidebar-drop-parent',
  strings: { path: 'data-sidebar-path' },
  flags: { expanded: 'data-sidebar-expanded', hasChildren: 'data-sidebar-children' },
}

const rowDom = createDropRowDom<DragRow>(SIDEBAR_DRAG_ROW_SPEC)

/**
 * The chat-subtree cycle guard (Task 20's own disclosed deferral): a drag may
 * not drop a row into its own descendant, which the pure matrix cannot see
 * since `SIDEBAR_DROP_POLICY` never reads a row's ancestry. Ported from the
 * old `chat-drop.ts`'s `chatAllowedModes` — same mechanism (a delimited
 * ancestor-chain STRING published on the row, substring-tested against each
 * subject's id so `/ab/` can never false-match subject `a`), run here at the
 * hit-test layer instead of inside the shared matrix, since `SidebarRow`
 * itself carries no path.
 */
function cycleSafeAllowedModes(subjects: readonly DragRow[], target: DragRow) {
  const allowed = SIDEBAR_DROP_POLICY.allowedModes(subjects, target)
  if (!anyModeAllowed(allowed)) return allowed
  const path = target.path ?? ''
  return subjects.some((s) => path.includes(`/${s.id}/`)) ? NO_MODES : allowed
}

const SIDEBAR_DRAG_POLICY: DropPolicy<DragRow, DragRow> = {
  allowedModes: cycleSafeAllowedModes,
  edgeBandFor: SIDEBAR_DROP_POLICY.edgeBandFor,
}

/** The four zones a pane resolves to — spec §8.1's table (center → into this
 *  view, you choose where; an edge → into this view, on that side). */
export type SidebarPaneZone = 'center' | 'left' | 'right' | 'top' | 'bottom'

export interface SidebarPaneHit {
  kind: 'pane'
  paneId: string
  zone: SidebarPaneZone
}

/**
 * The pane as a drop target, geometry-aware rather than the old binary
 * "remove" zone (`editor-removal-overlay.tsx`/`drop-target-dom.ts`'s
 * `PANE_DROP_ATTR`, deleted with it — Task 22).
 *
 * Contract for whoever mounts a droppable pane element (`PaneContainer`):
 * spread `{[PANE_DROP_ATTR]: paneId}` onto it — the ATTRIBUTE'S VALUE is the
 * pane's id, not a bare presence flag, since `onPaneDrop` needs to say which
 * pane.
 */
export const PANE_DROP_ATTR = 'data-pane-drop'

const paneZone: DropZone<DragRow, SidebarPaneHit> = {
  attr: PANE_DROP_ATTR,
  hit: (_subjects, el, point) => {
    const paneId = el.getAttribute(PANE_DROP_ATTR)
    if (!paneId) return null
    const rect = el.getBoundingClientRect()
    if (rect.width <= 0 || rect.height <= 0) return null
    const zone: PaneDropZone = getPaneDropZoneFromRect(point, rect)
    return { kind: 'pane', paneId, zone: zone ?? 'center' }
  },
}

const hitTest = createDropHitTest<DragRow, DragRow, SidebarPaneHit>(
  rowDom,
  SIDEBAR_DRAG_POLICY,
  paneZone,
)

type Hit = ReturnType<typeof hitTest>

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

function paintDropLine(el: HTMLDivElement | null, hit: Hit): void {
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

function setDropLineTracking(el: HTMLDivElement | null, tracking: boolean): void {
  if (!el) return
  el.style.transition = tracking ? 'none' : ''
}

function samePaneHit(a: SidebarPaneHit | null, b: SidebarPaneHit | null): boolean {
  if (a === null || b === null) return a === b
  return a.paneId === b.paneId && a.zone === b.zone
}

export interface UseSidebarDragOptions {
  /** The tree/band's own scroll container — what an edge-held drag scrolls. */
  scrollRef: React.RefObject<HTMLElement | null>
  /** The rows a drag starting on `rowId` carries, in tree order — the caller's
   *  own selection resolution; this hook does not maintain one of its own. */
  subjectsFor: (rowId: string) => SidebarRow[]
  /** Commit a drop onto another row. */
  onDrop: (subjects: SidebarRow[], target: SidebarRow, mode: DropMode) => void
  /** Commit a drop onto a pane — replaces the old `onPaneRemove` shape. Every
   *  drop ADDS now (spec §8.1); nothing here arms a removal. */
  onPaneDrop: (subjects: SidebarRow[], paneId: string, zone: SidebarPaneZone) => void
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
  const paneHitRef = useRef<SidebarPaneHit | null>(null)
  const edgeScrollerRef = useRef<EdgeScroller | null>(null)
  const scrollerXRef = useRef<{ left: number; right: number } | null>(null)
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
        paneHitRef.current = pane
        setPaneHit(pane)
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
      // Only if the row is still in the tree: a row deleted on the wire
      // between the press and the threshold takes its element with it, and
      // capturing a pointer on a detached node throws.
      if (pending.target.isConnected) pending.target.setPointerCapture(pending.pointerId)

      const subjects = optionsRef.current.subjectsFor(pending.row.id)
      const drag: ActiveDrag = { subjects, pointer: { x: e.clientX, y: e.clientY } }

      // Every measurement this drag needs, taken together and before the
      // first style write: interleaving reads and writes forces a layout per
      // clone on the one frame of a drag that is already doing the most work.
      const elements = subjects
        .map((s) => rowDom.elementFor(s))
        .filter((el): el is HTMLElement => el !== null)
      const scroller = optionsRef.current.scrollRef.current
      const scrollerBox = scroller?.getBoundingClientRect()
      const grabbed = rowDom.elementFor(pending.row) ?? pending.target
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
      grabRef.current = { dx: 0, dy: 0 }
      document.documentElement.removeAttribute('data-row-dragging')
      document.removeEventListener('selectstart', preventDefault)
      draggingRef.current = null
      dropTargetRef.current = null
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
      const hit = hitTest(e.clientX, e.clientY, drag.subjects)
      if (hit?.kind === 'pane') {
        optionsRef.current.onPaneDrop([...drag.subjects], hit.paneId, hit.zone)
      } else if (hit?.kind === 'row') {
        // `hit.row` is what `rowDom.read()` reconstructed off the DOM —
        // {kind, id, parentId, path, expanded, hasChildren} and NOT a real
        // `SidebarRow` (order/label/ownsWorktree/workspaceId/working/hasView
        // are all absent, and `parentId` comes back '' for a root row, never
        // `null` the way every real row uses). Good enough for the matrix
        // (it only ever needs id/kind/parentId/expanded/hasChildren), wrong
        // to hand a caller that expects the real thing — resolved back to
        // the genuine row here, once, at commit time rather than every
        // pointermove. `subjectsFor` already resolves "what live row does
        // this id name" (that's exactly what it does for the row a press
        // started on), so it doubles as the target lookup without a new
        // option on this hook's contract.
        const target = optionsRef.current.subjectsFor(hit.row.id).find((r) => r.id === hit.row.id)
        // A race — the row left the tree between the hit test and the
        // release — refuses rather than handing a caller a target it can no
        // longer resolve.
        if (target) optionsRef.current.onDrop([...drag.subjects], target, hit.row.mode)
      }
      endDrag()
    }

    window.addEventListener('pointermove', onPointerMove)
    window.addEventListener('pointerup', onPointerUp)
    window.addEventListener('pointercancel', endDrag)
    return () => {
      edgeScrollerRef.current?.stop()
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
