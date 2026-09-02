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
import { handleTrash } from '@/components/layout/space-content-actions'
import { toast } from '@/features/window/stores/toast-store'
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

/** How long the working-chat drag refusal (spec §8.3) stays up before it
 *  clears itself, if nothing sooner does (a fresh pointerdown anywhere). */
const WORKING_DRAG_REFUSAL_MS = 1600

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
  /** Published only by `RecentsBand` (see `recents-band.tsx`'s own
   *  `drag.dragProps(row, { inRecents: true })`) — the one fact a hit test
   *  needs that a tree row and a Recents row disagree on: spec §8.1's
   *  table gives "middle of a Recents entry" and "above/below a Recents
   *  entry" different outcomes than the same geometry over a tree row, and
   *  `SidebarRow` itself carries nothing that tells the two apart (a
   *  root-level tree bubble and a Recents row both draw `parentId: null`).
   *  Read back at commit time in `onPointerUp` and handed to `onDrop` as
   *  its own argument — never smuggled onto the resolved `SidebarRow`
   *  itself, which stays exactly what every other caller already expects. */
  inRecents?: boolean
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
  flags: {
    expanded: 'data-sidebar-expanded',
    hasChildren: 'data-sidebar-children',
    inRecents: 'data-sidebar-recents-row',
  },
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

/**
 * Painted onto the SAME element as {@link PANE_DROP_ATTR} for as long as this
 * pane is the one a release would land in — spec §8.2's "the entry about to
 * take a drop wears the same ring a pane wears" is a promise about a PANE
 * too, and this is the attribute a pane's own CSS reads to draw it (see
 * `paintPaneHit` below). A bare presence flag, like the old veil's
 * `data-armed` — there is only one question here ("is this the one"), not a
 * value to carry.
 */
export const PANE_HIT_ATTR = 'data-pane-hit'

/**
 * `SidebarPaneHit` plus the ACTUAL element `elementsFromPoint` resolved —
 * carried only through this hook's own internal ref/paint plumbing, never
 * exposed on the public `paneHit` state (which stays the plain logical
 * value `{kind, paneId, zone}`). `WorkspaceHost` keeps every retained
 * workspace mounted at once (hidden, not unmounted), and `PANE_DROP_ATTR`'s
 * VALUES — `ROOT_PANE_ID`/`BOTTOM_PANE_ID` — are literal constants every
 * workspace store shares, so a bare paneId string cannot uniquely name a DOM
 * node: `document.querySelector('[data-pane-drop="root-pane"]')` can just as
 * easily return a hidden, off-screen workspace's node as the one actually
 * under the pointer, depending on DOM order — the exact hazard
 * `performSidebarPaneDrop`'s own active-workspace guard exists to close for
 * the commit path (Fix round 1). The hit test ALREADY resolved the one true
 * element via `elementsFromPoint` (which, unlike `querySelector`, only ever
 * returns what is actually painted at that point); carrying it forward here
 * is what lets `paintPaneHit` below never have to re-derive it by attribute.
 */
interface ResolvedPaneHit extends SidebarPaneHit {
  el: Element
}

const paneZone: DropZone<DragRow, ResolvedPaneHit> = {
  attr: PANE_DROP_ATTR,
  hit: (_subjects, el, point) => {
    const paneId = el.getAttribute(PANE_DROP_ATTR)
    if (!paneId) return null
    const rect = el.getBoundingClientRect()
    if (rect.width <= 0 || rect.height <= 0) return null
    const zone: PaneDropZone = getPaneDropZoneFromRect(point, rect)
    return { kind: 'pane', paneId, zone: zone ?? 'center', el }
  },
}

/**
 * Addendum §2: the file explorer card's own surface, once folded for a live
 * drag, standing in for removal — the same shape `drop-dom.ts`'s own doc
 * comment on `DropZone` names as its original reason to exist ("the editor
 * pane, standing for removal" — deleted with the old Chats panel in Task
 * 22, and reused here rather than a second whole-region hit-test built from
 * scratch). `SidebarCarousel` spreads this attribute onto its trash-target
 * surface only while that surface is actually showing one (see its own
 * `data-sidebar-trash-drop` usage) — the hit test itself does no further
 * validation, since REFUSING a specific subject (a locked branch, a working
 * chat already refused at pickup) is `planRemoval`/`handleTrash`'s job, not
 * a drop zone's; a card that IS the trash target accepts anything dragged
 * to it and lets the removal plan decide what actually happens.
 */
export const CARD_TRASH_DROP_ATTR = 'data-sidebar-trash-drop'

interface CardTrashHit {
  kind: 'card-trash'
}

const cardTrashZone: DropZone<DragRow, CardTrashHit> = {
  attr: CARD_TRASH_DROP_ATTR,
  hit: (subjects) => (subjects.length > 0 ? { kind: 'card-trash' } : null),
}

const hitTest = createDropHitTest<DragRow, DragRow, ResolvedPaneHit | CardTrashHit>(
  rowDom,
  SIDEBAR_DRAG_POLICY,
  [paneZone, cardTrashZone],
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

/**
 * §8.2: "the entry about to take a drop wears the same ring a pane wears" —
 * so a pane being hovered during a sidebar-row drag needs the OTHER half of
 * that same answer, a neutral indicator marking which pane a release would
 * land in. Painted straight onto the DOM, like the hairline above and the
 * ghost — a pane's own element lives in a completely different part of the
 * tree (inside `WorkspaceHost`, not the sidebar), so there is no ref to hand
 * this a mounted node the way `attachDropLine` gets one.
 *
 * Takes the RESOLVED elements, never re-derives one by attribute (Fix round
 * 2): `PANE_DROP_ATTR`'s value is not unique across the document — every
 * retained workspace's hidden pane tree carries the same
 * `ROOT_PANE_ID`/`BOTTOM_PANE_ID` strings — so a `document.querySelector`
 * lookup here could paint a different, off-screen workspace's node instead
 * of the one the pointer is actually over, depending on DOM order. `el` is
 * whatever `elementsFromPoint` itself resolved (see `ResolvedPaneHit`), which
 * cannot make that mistake — it only ever names what is really painted
 * there.
 *
 * Gated on the ELEMENT changing, not the paneId string — for the same
 * reason: two elements can share a paneId, and only element identity says
 * "this is genuinely a different target" (a center→edge move on the SAME
 * pane repaints the drop line but not this).
 */
function paintPaneHit(prev: ResolvedPaneHit | null, next: ResolvedPaneHit | null): void {
  if ((prev?.el ?? null) === (next?.el ?? null)) return
  prev?.el.removeAttribute(PANE_HIT_ATTR)
  next?.el.setAttribute(PANE_HIT_ATTR, '')
}

/**
 * Spec §8.3's refusal, painted straight onto the DOM rather than through a
 * new store field or CSS rule — the same imperative-write budget every other
 * per-frame effect in this file already spends (the ghost, the drop line,
 * `paintPaneHit`). "The rest of the sidebar dims, the dragged row goes red,
 * and a short line says why": the scroller is the caller's own scroll
 * region (the only "rest of the sidebar" this hook has a handle on), the row
 * is found the same way the ghost clone finds it (`rowDom.elementFor`), and
 * the line is a small fixed note positioned off the row's own rect. Returns
 * the cleanup that undoes all three; the caller owns when that fires (a
 * timeout, or the next pointerdown anywhere).
 */
function paintWorkingDragRefusal(scroller: HTMLElement | null, rowEl: HTMLElement): () => void {
  const prevOpacity = scroller?.style.opacity ?? ''
  const prevTransition = scroller?.style.transition ?? ''
  if (scroller) {
    scroller.style.transition = 'opacity 120ms ease-out'
    scroller.style.opacity = '0.4'
  }

  const prevOutline = rowEl.style.outline
  const prevBg = rowEl.style.backgroundColor
  rowEl.style.outline = '1.5px solid var(--destructive)'
  rowEl.style.outlineOffset = '-1.5px'
  rowEl.style.backgroundColor = 'color-mix(in srgb, var(--destructive) 14%, transparent)'

  const rect = rowEl.getBoundingClientRect()
  const note = document.createElement('div')
  note.setAttribute('data-row-drag-refusal', '')
  note.textContent = "Can't drag — still working"
  note.style.cssText = [
    'position:fixed',
    'z-index:80',
    'pointer-events:none',
    'font-size:11px',
    'font-weight:500',
    'padding:3px 6px',
    'border-radius:6px',
    'background:var(--destructive)',
    'color:var(--destructive-foreground)',
    `left:${rect.left}px`,
    `top:${rect.bottom + 4}px`,
  ].join(';')
  document.body.appendChild(note)

  return () => {
    if (scroller) {
      scroller.style.opacity = prevOpacity
      scroller.style.transition = prevTransition
    }
    rowEl.style.outline = prevOutline
    rowEl.style.backgroundColor = prevBg
    note.remove()
  }
}

export interface UseSidebarDragOptions {
  /** The tree/band's own scroll container — what an edge-held drag scrolls. */
  scrollRef: React.RefObject<HTMLElement | null>
  /** The rows a drag starting on `rowId` carries, in tree order — the caller's
   *  own selection resolution; this hook does not maintain one of its own. */
  subjectsFor: (rowId: string) => SidebarRow[]
  /** Commit a drop onto another row. `targetInRecents` is true when the row
   *  the pointer released over lives in a `RecentsBand`, not a tree — spec
   *  §8.1's table gives that geometry a different meaning ("into that view,
   *  opened" / "it moves to that slot") than the same drop over a tree row
   *  ("make it a thread of this chat"), and `target: SidebarRow` alone
   *  cannot say which (a root-level tree bubble and a Recents row both
   *  carry `parentId: null`). A caller that only ever drops onto a tree —
   *  `SidebarTree` (not in this package) — can ignore the fourth argument
   *  entirely; it is always `false` there. */
  onDrop: (
    subjects: SidebarRow[],
    target: SidebarRow,
    mode: DropMode,
    targetInRecents?: boolean,
  ) => void
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
  const paneHitRef = useRef<ResolvedPaneHit | null>(null)
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
  // §8.3's refusal — cleanup (style/note reversal) plus the timer/listener
  // that clear it, so a second refusal (or a real drag starting) can tear
  // down whatever the last one left painted before starting fresh.
  const refusalRef = useRef<(() => void) | null>(null)

  const clearWorkingDragRefusal = useCallback(() => {
    refusalRef.current?.()
    refusalRef.current = null
  }, [])

  const showWorkingDragRefusal = useCallback(
    (row: SidebarRow) => {
      clearWorkingDragRefusal()
      const rowEl = rowDom.elementFor(row)
      if (!rowEl) return
      const undoPaint = paintWorkingDragRefusal(optionsRef.current.scrollRef.current, rowEl)
      const timer = window.setTimeout(() => clearWorkingDragRefusal(), WORKING_DRAG_REFUSAL_MS)
      // Any next interaction clears it early too — a refusal that outlives
      // the click that dismissed it would read as stuck rather than timed.
      const onNextPointerDown = () => clearWorkingDragRefusal()
      window.addEventListener('pointerdown', onNextPointerDown, { capture: true, once: true })
      refusalRef.current = () => {
        window.clearTimeout(timer)
        window.removeEventListener('pointerdown', onNextPointerDown, { capture: true })
        undoPaint()
      }
    },
    [clearWorkingDragRefusal],
  )

  useEffect(() => () => clearWorkingDragRefusal(), [clearWorkingDragRefusal])

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

  const onPointerDownDrag = useCallback(
    (row: SidebarRow, e: React.PointerEvent) => {
      if (e.button !== 0) return
      if (draggingRef.current) return // ignore a second pointer mid-drag
      // §8.3: "a working chat may not be dragged... the rest of the sidebar
      // dims, the dragged row goes red, and a short line says why." Refused
      // at the pickup itself, before a drag is ever armed — the drop policy
      // (`SIDEBAR_DROP_POLICY.allowedModes`) already refuses a working
      // subject too, but that only ever fires once a drag is already in the
      // air, which is one gesture too late for "may not be dragged".
      if (row.working) {
        showWorkingDragRefusal(row)
        return
      }
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
    },
    [showWorkingDragRefusal],
  )

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
      // A real drag starting supersedes any refusal flash still up from a
      // moment ago — read/cleared straight off the ref, like everything
      // else this once-subscribed effect touches.
      refusalRef.current?.()
      refusalRef.current = null
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
      paintPaneHit(paneHitRef.current, null)
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
      } else if (hit?.kind === 'card-trash') {
        // Addendum §2 — the same removal tray every trash-button delete
        // already used, one hold per dragged row (in practice always
        // exactly one: neither tree nor Recents drags carry more than the
        // grabbed row today). `handleTrash` re-resolves each id against the
        // live store itself; a row that turns out not to be deletable (a
        // locked branch, a repo home) gets the same toast the old row-level
        // trash confirm gave it, rather than a drop that visibly landed and
        // did nothing. A working chat never reaches this at all: `1a`'s
        // pickup-time refusal (`onPointerDownDrag`) already stops the drag
        // before it starts.
        for (const subject of drag.subjects) {
          if (!handleTrash(subject.id)) {
            toast.error(`Can't delete ${subject.label || 'this row'} — it may be locked`)
          }
        }
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
        if (target) {
          optionsRef.current.onDrop(
            [...drag.subjects],
            target,
            hit.row.mode,
            Boolean(hit.row.inRecents),
          )
        }
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
      paintPaneHit(paneHitRef.current, null)
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
