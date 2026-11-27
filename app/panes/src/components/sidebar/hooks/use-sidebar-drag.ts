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

const SIDEBAR_DRAG_THRESHOLD_PX = 8

const NO_IDS: ReadonlySet<string> = new Set<string>()

interface ActiveDrag {
  subjects: readonly SidebarRow[]
  pointer: { x: number; y: number }
}

const preventDefault = (e: Event) => e.preventDefault()

function suppressNextClick(): void {
  const swallow = (e: MouseEvent) => {
    e.stopPropagation()
    e.preventDefault()
  }
  window.addEventListener('click', swallow, { capture: true, once: true })
  setTimeout(() => window.removeEventListener('click', swallow, { capture: true }), 0)
}

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
  scrollRef: React.RefObject<HTMLElement | null>
  strip?: () => HTMLElement | null
}

export interface SidebarDrag {
  dragging: boolean
  draggingIds: ReadonlySet<string>
  nestTargetId: string | null
  paneHit: SidebarPaneHit | null
  ghostRows: GhostRows | null
  ghostRef: React.RefObject<HTMLDivElement | null>
  ghostOrigin: { x: number; y: number } | null
  attachDropLine: (el: HTMLDivElement | null) => void
  onPointerDownDrag: (row: SidebarRow, e: React.PointerEvent) => void
  dragProps: (row: SidebarRow, extra?: RowDragExtra) => Record<string, string | undefined>
}

export function useSidebarDrag(options: UseSidebarDragOptions): SidebarDrag {
  const [dragging, setDragging] = useState(false)
  const [draggingIds, setDraggingIds] = useState<ReadonlySet<string>>(NO_IDS)
  const [nestTargetId, setNestTargetId] = useState<string | null>(null)
  const [paneHit, setPaneHit] = useState<SidebarPaneHit | null>(null)
  const [ghostRows, setGhostRows] = useState<GhostRows | null>(null)

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
    paintDropLine(el, lastHitRef.current)
  }, [])

  const dragProps = useCallback(
    (row: SidebarRow, extra?: RowDragExtra): Record<string, string | undefined> =>
      rowDom.props({ ...row, ...extra }),
    [],
  )

  const onPointerDownDrag = useCallback((row: SidebarRow, e: React.PointerEvent) => {
    if (e.button !== 0) return
    if (draggingRef.current) return
    document.addEventListener('selectstart', preventDefault)
    pendingRef.current = {
      row,
      startX: e.clientX,
      startY: e.clientY,
      target: e.currentTarget as HTMLElement,
      pointerId: e.pointerId,
    }
  }, [])

  useEffect(() => {
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
        setPaneHit(pane && { kind: pane.kind, paneId: pane.paneId, zone: pane.zone })
      }
      lastHitRef.current = hit
      paintDropLine(dropLineRef.current, hit)
    }

    function beginDrag(e: MouseEvent): void {
      const pending = pendingRef.current!
      pendingRef.current = null
      tryCapturePointer(pending.target, pending.pointerId)

      const subjects = optionsRef.current.subjectsFor(pending.row.id)
      const drag: ActiveDrag = { subjects, pointer: { x: e.clientX, y: e.clientY } }

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

      if (scroller && scrollerBox && !stripRef.current) {
        scrollerXRef.current = { left: scrollerBox.left, right: scrollerBox.right }
        edgeScrollerRef.current = createEdgeScroller(
          scroller,
          { top: scrollerBox.top, height: scrollerBox.height },
          {
            onScrolled: () => {
              publish(hitTest(drag.pointer.x, drag.pointer.y, drag.subjects))
            },
            onRunningChange: (running) => setDropLineTracking(dropLineRef.current, running),
          },
        )
        scroller.addEventListener('wheel', preventDefault, { passive: false })
        wheelLockScrollerRef.current = scroller
      }

      draggingRef.current = drag
      ghostOriginRef.current = stripRef.current?.position(e.clientX, e.clientY) ?? {
        x: e.clientX - grabRef.current.dx,
        y: e.clientY - grabRef.current.dy,
      }
      document.documentElement.setAttribute('data-row-dragging', '')
      window.getSelection()?.removeAllRanges()
      setGhostRows(rows)
      setDraggingIds(new Set(subjects.map((s) => s.id)))
      setDragging(true)
      publish(hitTest(e.clientX, e.clientY, subjects))
    }

    function onPointerMove(e: MouseEvent): void {
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

      const strip = stripRef.current
      const hit = strip?.inBand(e.clientX, e.clientY)
        ? null
        : hitTest(e.clientX, e.clientY, drag.subjects)

      const span = scrollerXRef.current
      if (span !== null && e.clientX >= span.left && e.clientX <= span.right) {
        edgeScrollerRef.current?.update(e.clientY)
      } else edgeScrollerRef.current?.stop()

      if (strip) strip.move(e.clientX, e.clientY, ghostRef.current)
      else if (ghostRef.current) {
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
      document.removeEventListener('selectstart', preventDefault)
      const drag = draggingRef.current
      if (!drag) return

      suppressNextClick()
      const strip = stripRef.current
      if (strip?.inBand(e.clientX, e.clientY)) {
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
