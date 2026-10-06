import { useCallback, useEffect, useLayoutEffect, useRef, type PointerEvent } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import type { ConsoleEntry } from '@/features/console/lib/entries'
import { EntryRow } from './entry-row'

/** A rough row height; each row is measured once it has rendered. */
const ESTIMATED_ROW = 20

/** Within this many pixels of the bottom, the view follows new lines. */
const FOLLOW_SLACK = 32

/** How long after the user touches the pane a scroll still counts as theirs. */
const USER_SCROLL_WINDOW_MS = 250

interface ConsoleLogProps {
  entries: readonly ConsoleEntry[]
  expandedId: number | null
  onToggle: (id: number) => void
  /** While true the console is shown: the view returns to the newest line and takes focus. */
  open: boolean
}

/**
 * The scrolling log. Virtualized, because the buffer holds thousands of lines
 * and a line's height depends on how its fields wrap.
 *
 * It sticks to the newest line until the user scrolls up, and resumes when they
 * come back down. Only a scroll that follows the user's own input (wheel, touch,
 * the scrollbar, keys) can stop it: rows are measured after layout, the buffer
 * trims its oldest lines, and the browser clamps the position, so scroll events
 * also arrive from layout nobody asked for, at distances from the bottom that
 * mean nothing.
 */
export function ConsoleLog({ entries, expandedId, onToggle, open }: ConsoleLogProps) {
  const scroller = useRef<HTMLDivElement>(null)
  const following = useRef(true)
  const userActive = useRef(false)
  const userTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  const virtualizer = useVirtualizer({
    count: entries.length,
    getScrollElement: () => scroller.current,
    estimateSize: () => ESTIMATED_ROW,
    overscan: 15,
    getItemKey: (index) => entries[index].id,
  })
  // The pane pins itself; the virtualizer shifting the position as rows are measured would fight it.
  virtualizer.shouldAdjustScrollPositionOnItemSizeChange = () => false
  const total = virtualizer.getTotalSize()

  const markUser = useCallback(() => {
    userActive.current = true
    if (userTimer.current !== null) clearTimeout(userTimer.current)
    userTimer.current = setTimeout(() => {
      userActive.current = false
    }, USER_SCROLL_WINDOW_MS)
  }, [])

  // Only the scrollbar and the gutter press the pane itself: a click on a row is not a scroll.
  const onPointerDown = useCallback(
    (event: PointerEvent<HTMLDivElement>) => {
      if (event.target === event.currentTarget) markUser()
    },
    [markUser],
  )

  useEffect(
    () => () => {
      if (userTimer.current !== null) clearTimeout(userTimer.current)
    },
    [],
  )

  const onScroll = useCallback(() => {
    const el = scroller.current
    if (!el || !userActive.current) return
    following.current = el.scrollHeight - el.scrollTop - el.clientHeight < FOLLOW_SLACK
  }, [])

  useLayoutEffect(() => {
    if (!open) return
    following.current = true
    scroller.current?.focus({ preventScroll: true })
  }, [open])

  useLayoutEffect(() => {
    const el = scroller.current
    if (!el || !following.current || entries.length === 0) return
    const pin = () => {
      if (following.current) el.scrollTop = el.scrollHeight
    }
    pin()
    const frame = requestAnimationFrame(pin)
    return () => cancelAnimationFrame(frame)
  }, [entries.length, total, expandedId, open])

  // Resizing or re-docking changes the viewport without touching the entries.
  useEffect(() => {
    const el = scroller.current
    if (!el || typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(() => {
      if (following.current) el.scrollTop = el.scrollHeight
    })
    observer.observe(el)
    return () => observer.disconnect()
  }, [])

  return (
    <div
      ref={scroller}
      onScroll={onScroll}
      // The user's own input: evidence that a scroll is theirs.
      onWheel={markUser}
      onTouchMove={markUser}
      onKeyDown={markUser}
      onPointerDown={onPointerDown}
      // A scrollable region the keyboard can reach, so the arrow and page keys scroll it.
      tabIndex={0}
      data-slot="console-log"
      role="log"
      aria-live="off"
      aria-label="Daemon log"
      className="focus-visible:ring-ring min-h-0 flex-1 overflow-y-auto py-2 outline-none focus-visible:ring-2 focus-visible:ring-inset"
    >
      <div className="relative w-full" style={{ height: total }}>
        {virtualizer.getVirtualItems().map((item) => (
          <div
            key={item.key}
            data-index={item.index}
            ref={virtualizer.measureElement}
            className="absolute top-0 left-0 w-full"
            style={{ transform: `translateY(${item.start}px)` }}
          >
            <EntryRow
              entry={entries[item.index]}
              expanded={entries[item.index].id === expandedId}
              onToggle={onToggle}
            />
          </div>
        ))}
      </div>
    </div>
  )
}
