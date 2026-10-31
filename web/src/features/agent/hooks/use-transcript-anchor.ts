import { useCallback, useLayoutEffect, useRef } from 'react'
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
}

/**
 * Keeps a transcript on its newest line while the reader is there, and out of
 * their way once they scroll up. Growth eases (follow-scroll); anything that is
 * not content arriving lands in one write. A scroll of under a row (STICK_SLACK)
 * while a reply streams is pulled back on purpose: a burst outruns a frame.
 */
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
  const boxed = useRef(true)
  const landNext = useRef(true)
  const wasMeasuring = useRef(false)
  const shown = useRef(options.visible ?? true)
  const optionsRef = useRef(options)
  optionsRef.current = options

  const write = useCallback((el: HTMLElement, top: number) => {
    el.scrollTop = top
    written.current = el.scrollTop
  }, [])

  const resync = useCallback(
    (correcting = false) => {
      const el = scrollRef.current
      if (!el) return
      if (el.clientHeight === 0) {
        boxed.current = false
        return
      }
      if (!boxed.current) {
        boxed.current = true
        landNext.current = true
      }
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
    const build = () =>
      createFollowScroll(el, (top) => {
        written.current = top
      })
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
  }, [resync])

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
  }, [])

  const stick = useCallback(() => {
    stuck.current = true
  }, [])

  const onScroll = useCallback(() => {
    const el = scrollRef.current
    if (!el) return
    if (written.current !== null && Math.abs(el.scrollTop - written.current) <= 1) return
    written.current = null
    stuck.current = el.scrollHeight - el.scrollTop - el.clientHeight <= STICK_SLACK
    fromBottom.current = el.scrollHeight - el.scrollTop
  }, [])

  return { scrollRef, onScroll, afterCommit, notifyReflow, noteScrollWrite, stick }
}
