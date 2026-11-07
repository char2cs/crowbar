import { useCallback, useLayoutEffect, useRef, useState } from 'react'
import { createFollowScroll, type FollowScroll } from '@/features/agent/hooks/lib/follow-scroll'

/** How close to the bottom still counts as reading the newest end: about one
 *  row, so a burst that outruns one scroll frame is not read as scrolling away. */
const STICK_SLACK = 96

/** Where the reader left a chat, for its next mount this session. */
export interface TranscriptScrollPosition {
  stuck: boolean
  /** From the bottom: turns that land while the chat is away keep the reader's row. */
  distanceFromBottom: number
}

interface UseTranscriptAnchorOptions {
  /** The feed is replaying history: every change is a reposition. */
  loadingHistory?: boolean
  /** Read once, at mount. */
  initialPosition?: TranscriptScrollPosition | null
  /** Called once, from unmount. */
  onPositionChange?: (position: TranscriptScrollPosition) => void
  /** A background tab stays mounted behind visibility:hidden, which changes no geometry. */
  visible?: boolean
  /** A turn runs or a sent prompt waits for one: only then does growth ease.
   *  With nothing arriving, a change (a late row layout, the dock) lands at once. */
  arriving?: boolean
  /** Each write the anchor makes, as it makes it: the virtualizer otherwise
   *  hears of it from the scroll event, a frame after painting the old window. */
  onWrite?: (top: number) => void
}

export interface TranscriptAnchor {
  scrollRef: React.RefObject<HTMLDivElement | null>
  onScroll: () => void
  /** Call from a layout effect after every commit of the list: any of them may
   *  have moved its bottom. `measuring` is true while a row on screen still has
   *  its estimated height, so the next change corrects an estimate. */
  afterCommit: (measuring: boolean) => void
  /** The viewport's box changed: a resize, or a display:none workspace coming back. */
  notifyReflow: () => void
  /** Something else (the virtualizer) wrote scrollTop: its scroll event is not the reader's. */
  noteScrollWrite: () => void
  /** The reader's own prompt landed: rejoin the live end. */
  stick: () => void
  /** Rows land above the reader: the next commit keeps their distance from the bottom. */
  keepPlace: () => void
  /** The reader's hand on the scroller, passive: a wheel, drag or key up lets go
   *  of the bottom before the view has moved, which position alone cannot see. */
  intent: ReaderIntent
  /** The view is held to the bottom: a row resizing above it is the anchor's to
   *  answer, not the virtualizer's (its shift paints a frame before the total). */
  holdsBottom: () => boolean
  /** Hears `holdsBottom` change, for a render that depends on it. */
  subscribe: (listener: () => void) => () => void
}

interface ReaderIntent {
  onWheel: (event: React.WheelEvent) => void
  onKeyDown: (event: React.KeyboardEvent) => void
  onTouchStart: (event: React.TouchEvent) => void
  onTouchMove: (event: React.TouchEvent) => void
}

const READ_UP_KEYS = new Set(['ArrowUp', 'PageUp', 'Home'])

/** A trackpad at rest still reports a pixel or two: not a reader going back. */
const WHEEL_JITTER_PX = 4

/** A box between the event's target and the transcript that still scrolls up
 *  takes the gesture itself (a tool's payload); the transcript does not move. */
function takenInside(target: EventTarget, scroller: HTMLElement | null): boolean {
  for (let el = target instanceof Element ? target : null; el && el !== scroller;) {
    const scrolls =
      el.scrollHeight > el.clientHeight && /auto|scroll/.test(getComputedStyle(el).overflowY)
    if (scrolls && el.scrollTop > 0) return true
    el = el.parentElement
  }
  return false
}

const typing = (target: EventTarget) =>
  target instanceof HTMLElement &&
  (target.isContentEditable || target.tagName === 'INPUT' || target.tagName === 'TEXTAREA')

const readsUp = (event: React.KeyboardEvent) =>
  READ_UP_KEYS.has(event.key) || (event.key === ' ' && event.shiftKey)

// Keeps a transcript on its newest line while the reader is there, out of their way once they
// scroll up. Growth eases (follow-scroll), anything else lands in one write; a sub-row scroll
// (STICK_SLACK) mid-stream is pulled back on purpose, since a burst outruns a frame.
export function useTranscriptAnchor(options: UseTranscriptAnchorOptions = {}): TranscriptAnchor {
  const scrollRef = useRef<HTMLDivElement | null>(null)
  const stuck = useRef(options.initialPosition?.stuck ?? true)
  // Read on a reveal and on unmount, when a display:none box measures 0.
  const fromBottom = useRef<number | null>(options.initialPosition?.distanceFromBottom ?? null)
  const follow = useRef<FollowScroll | null>(null)
  // Where the last write we know of left scrollTop: its scroll event is not the reader.
  const written = useRef<number | null>(null)
  // Where the current catch-up began, so a staircase of corrections is one gap.
  const easeFrom = useRef<number | null>(null)
  const landNext = useRef(true)
  // The reader let go on purpose: their own climb back within STICK_SLACK
  // (a smooth wheel scroll's first steps) is reading, not rejoining.
  const reading = useRef(false)
  const seen = useRef<number | null>(null)
  const touchY = useRef<number | null>(null)
  const wasMeasuring = useRef(false)
  const shown = useRef(options.visible ?? true)
  const [listeners] = useState(() => new Set<() => void>())
  const setStuck = useCallback(
    (next: boolean) => {
      if (stuck.current === next) return
      stuck.current = next
      for (const listener of listeners) listener()
    },
    [listeners],
  )
  const optionsRef = useRef(options)
  optionsRef.current = options

  const wrote = useCallback((top: number) => {
    written.current = top
    seen.current = top
    optionsRef.current.onWrite?.(top)
  }, [])

  const write = useCallback(
    (el: HTMLElement, top: number) => {
      el.scrollTop = top
      wrote(el.scrollTop)
    },
    [wrote],
  )

  const resync = useCallback(
    (correcting = false) => {
      const el = scrollRef.current
      if (!el) return
      // A display:none box measures 0; the report that hid it asked for a landing
      // (notifyReflow), which waits here until the box is back.
      if (el.clientHeight === 0) return
      const land = landNext.current
      landNext.current = false
      if (!stuck.current) {
        if (land && fromBottom.current !== null) {
          write(el, Math.max(0, el.scrollHeight - fromBottom.current))
        }
        fromBottom.current = el.scrollHeight - el.scrollTop
        return
      }
      // The ceiling, not scrollHeight: an unreachable target overshoots on the first frame.
      const target = el.scrollHeight - el.clientHeight
      const gap = target - el.scrollTop
      if (gap <= 1) easeFrom.current = null
      // From the lower of where the catch-up began and where the view is now: a
      // view moved up since (a clamp, a virtualizer write) widens the gap.
      const from = Math.min(easeFrom.current ?? el.scrollTop, el.scrollTop)
      // A gap over a viewport is a reposition, and easing it sweeps the whole transcript.
      const reposition =
        land ||
        optionsRef.current.loadingHistory === true ||
        optionsRef.current.arriving !== true ||
        target - from > el.clientHeight ||
        (correcting && easeFrom.current === null)
      if (reposition) {
        if (el.scrollTop !== target) write(el, target)
        easeFrom.current = null
      } else if (gap > 1) {
        easeFrom.current = from
        follow.current?.setTarget(target)
      }
      fromBottom.current = el.scrollHeight - el.scrollTop
    },
    [write],
  )

  useLayoutEffect(() => {
    const el = scrollRef.current
    if (!el) return
    const build = () => createFollowScroll(el, wrote)
    follow.current = build()
    // An unfocused webview freezes rAF and never runs a request made meanwhile,
    // so the follow is rebuilt rather than re-targeted when focus returns.
    const onFocus = () => {
      follow.current?.stop()
      follow.current = build()
      resync()
    }
    window.addEventListener('focus', onFocus)
    return () => {
      window.removeEventListener('focus', onFocus)
      follow.current?.stop()
      follow.current = null
      optionsRef.current.onPositionChange?.({
        stuck: stuck.current,
        distanceFromBottom:
          el.clientHeight > 0
            ? el.scrollHeight - el.scrollTop
            : (fromBottom.current ?? el.scrollHeight - el.scrollTop),
      })
    }
  }, [resync, wrote])

  const afterCommit = useCallback(
    (measuring: boolean) => {
      const correcting = wasMeasuring.current
      wasMeasuring.current = measuring
      resync(correcting)
    },
    [resync],
  )

  // Coming to the front changes no geometry, so nothing else would land the view.
  useLayoutEffect(() => {
    const visible = options.visible ?? true
    if (visible && !shown.current) {
      landNext.current = true
      resync()
    }
    shown.current = visible
  }, [options.visible, resync])

  // A new viewport is a reposition, not content arriving.
  const notifyReflow = useCallback(() => {
    landNext.current = true
    resync()
  }, [resync])

  const noteScrollWrite = useCallback(() => {
    written.current = scrollRef.current?.scrollTop ?? null
    seen.current = written.current
  }, [])

  const stick = useCallback(() => {
    setStuck(true)
    reading.current = false
  }, [setStuck])

  const keepPlace = useCallback(() => {
    landNext.current = true
  }, [])

  const holdsBottom = useCallback(() => stuck.current, [])
  const subscribe = useCallback(
    (listener: () => void) => {
      listeners.add(listener)
      return () => void listeners.delete(listener)
    },
    [listeners],
  )

  const onScroll = useCallback(() => {
    const el = scrollRef.current
    if (!el) return
    if (written.current !== null && Math.abs(el.scrollTop - written.current) <= 1) return
    written.current = null
    const rising = seen.current !== null && el.scrollTop < seen.current - 1
    seen.current = el.scrollTop
    fromBottom.current = el.scrollHeight - el.scrollTop
    if (reading.current && rising) return
    setStuck(el.scrollHeight - el.scrollTop - el.clientHeight <= STICK_SLACK)
    if (stuck.current) reading.current = false
  }, [setStuck])

  const letGo = useCallback(() => {
    if (!stuck.current) return
    setStuck(false)
    reading.current = true
    easeFrom.current = null
    follow.current?.pause()
    const el = scrollRef.current
    if (el) fromBottom.current = el.scrollHeight - el.scrollTop
  }, [setStuck])

  const [intent] = useState<ReaderIntent>(() => ({
    onWheel: (event) => {
      const jitter = event.deltaMode === 0 && event.deltaY > -WHEEL_JITTER_PX
      const sideways = Math.abs(event.deltaX) >= Math.abs(event.deltaY)
      if (event.deltaY >= 0 || jitter || sideways || event.ctrlKey) return
      if (!takenInside(event.target, scrollRef.current)) letGo()
    },
    onKeyDown: (event) => {
      if (!readsUp(event) || typing(event.target)) return
      if (!takenInside(event.target, scrollRef.current)) letGo()
    },
    onTouchStart: (event) => {
      touchY.current = event.touches[0]?.clientY ?? null
    },
    onTouchMove: (event) => {
      const y = event.touches[0]?.clientY
      // A finger moving down drags the text down: the reader is going back.
      if (y !== undefined && touchY.current !== null && y > touchY.current) letGo()
    },
  }))

  return {
    scrollRef,
    onScroll,
    intent,
    afterCommit,
    notifyReflow,
    noteScrollWrite,
    stick,
    keepPlace,
    holdsBottom,
    subscribe,
  }
}
